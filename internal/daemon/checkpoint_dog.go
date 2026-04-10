package daemon

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/steveyegge/gastown/internal/constants"
	"github.com/steveyegge/gastown/internal/session"
	"github.com/steveyegge/gastown/internal/util"
)

const (
	defaultCheckpointDogInterval = 10 * time.Minute
)

// CheckpointDogConfig holds configuration for the checkpoint_dog patrol.
type CheckpointDogConfig struct {
	// Enabled controls whether the checkpoint dog runs.
	Enabled bool `json:"enabled"`

	// IntervalStr is how often to run, as a string (e.g., "10m").
	IntervalStr string `json:"interval,omitempty"`
}

// checkpointDogInterval returns the configured interval, or the default (10m).
func checkpointDogInterval(config *DaemonPatrolConfig) time.Duration {
	if config != nil && config.Patrols != nil && config.Patrols.CheckpointDog != nil {
		if config.Patrols.CheckpointDog.IntervalStr != "" {
			if d, err := time.ParseDuration(config.Patrols.CheckpointDog.IntervalStr); err == nil && d > 0 {
				return d
			}
		}
	}
	return defaultCheckpointDogInterval
}

// runtimeExcludeDirs are directories to unstage after git add -A.
// These contain runtime/ephemeral data that should not be checkpointed.
var runtimeExcludeDirs = []string{
	".claude/",
	".beads/",
	".runtime/",
	"__pycache__/",
}

// runCheckpointDog auto-commits WIP changes in active polecat worktrees.
// This protects against data loss when sessions crash or hit context limits.
//
// ## ZFC Exemption
// The checkpoint dog executes git operations directly (same pattern as
// compactor_dog's SQL operations). The daemon pours a molecule for
// observability, then runs git commands via exec.Command.
func (d *Daemon) runCheckpointDog() {
	if !d.isPatrolActive("checkpoint_dog") {
		return
	}

	d.logger.Printf("checkpoint_dog: starting cycle")

	mol := d.pourDogMolecule(constants.MolDogCheckpoint, nil)
	defer mol.close()

	rigs := d.getKnownRigs()
	totalScanned := 0
	totalCheckpointed := 0

	for _, rigName := range rigs {
		scanned, checkpointed := d.checkpointRigPolecats(rigName)
		totalScanned += scanned
		totalCheckpointed += checkpointed
	}

	mol.closeStep("scan")
	mol.closeStep("checkpoint")

	d.logger.Printf("checkpoint_dog: cycle complete — scanned %d worktrees, checkpointed %d",
		totalScanned, totalCheckpointed)
	mol.closeStep("report")
}

// checkpointRigPolecats checkpoints dirty polecat worktrees in a single rig.
// Returns (scanned, checkpointed) counts.
func (d *Daemon) checkpointRigPolecats(rigName string) (int, int) {
	polecatsDir := filepath.Join(d.config.TownRoot, rigName, "polecats")
	polecats, err := listPolecatWorktrees(polecatsDir)
	if err != nil {
		return 0, 0
	}

	scanned := 0
	checkpointed := 0

	for _, polecatName := range polecats {
		scanned++

		// Check if tmux session is alive — only checkpoint active sessions.
		// Dead sessions can't benefit from checkpoints.
		sessionName := session.PolecatSessionName(session.PrefixFor(rigName), polecatName)
		alive, err := d.tmux.HasSession(sessionName)
		if err != nil {
			d.logger.Printf("checkpoint_dog: error checking session %s: %v", sessionName, err)
			continue
		}
		if !alive {
			continue
		}

		workDir := filepath.Join(polecatsDir, polecatName)
		if d.checkpointWorktree(workDir, rigName, polecatName) {
			checkpointed++
		}
	}

	return scanned, checkpointed
}

// findPolecatGitRoot finds the git worktree root within a polecat directory.
// New structure: polecats/<name>/<rigname>/ — git root is at the rig subdirectory.
// Old structure: polecats/<name>/ — git root is the polecat directory itself.
// Returns "" if no git root is found.
func findPolecatGitRoot(polecatDir, rigName string) string {
	// New structure: polecats/<name>/<rigname>/.git
	newPath := filepath.Join(polecatDir, rigName)
	if _, err := os.Stat(filepath.Join(newPath, ".git")); err == nil {
		return newPath
	}

	// Old structure: polecats/<name>/.git
	if _, err := os.Stat(filepath.Join(polecatDir, ".git")); err == nil {
		return polecatDir
	}

	// Fallback: scan immediate subdirectories for a .git entry.
	// Handles rigs where the repo name differs from the rig name.
	entries, err := os.ReadDir(polecatDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		candidate := filepath.Join(polecatDir, entry.Name())
		if _, err := os.Stat(filepath.Join(candidate, ".git")); err == nil {
			return candidate
		}
	}

	return ""
}

// checkpointWorktree creates a WIP checkpoint commit for a single worktree.
// Returns true if a checkpoint was created.
func (d *Daemon) checkpointWorktree(workDir, rigName, polecatName string) bool {
	gitRoot := findPolecatGitRoot(workDir, rigName)
	if gitRoot == "" {
		d.logger.Printf("checkpoint_dog: no git root found in %s/%s", rigName, polecatName)
		return false
	}

	return d.commitWIPCheckpoint(gitRoot, rigName, polecatName)
}

// commitWIPCheckpoint creates a WIP commit in the given git directory.
// Returns true if a checkpoint commit was created.
func (d *Daemon) commitWIPCheckpoint(gitRoot, rigName, polecatName string) bool {
	// Check git status (exclude runtime dirs from consideration)
	statusOut, err := runGitCmd(gitRoot, "status", "--porcelain")
	if err != nil {
		d.logger.Printf("checkpoint_dog: git status failed in %s/%s: %v", rigName, polecatName, err)
		return false
	}
	if strings.TrimSpace(statusOut) == "" {
		return false // Clean worktree
	}

	// Stage everything
	if _, err := runGitCmd(gitRoot, "add", "-A"); err != nil {
		d.logger.Printf("checkpoint_dog: git add -A failed in %s/%s: %v", rigName, polecatName, err)
		return false
	}

	// Unstage runtime/ephemeral directories
	for _, dir := range runtimeExcludeDirs {
		// git reset HEAD -- <dir> is safe even if dir doesn't exist (exits 0)
		_, _ = runGitCmd(gitRoot, "reset", "HEAD", "--", dir)
	}

	// Check if anything is staged after exclusions
	diffOut, err := runGitCmd(gitRoot, "diff", "--cached", "--quiet")
	if err == nil && strings.TrimSpace(diffOut) == "" {
		// --quiet exits 0 if no diff → nothing staged
		return false
	}

	// Commit the checkpoint
	if _, err := runGitCmd(gitRoot, "commit", "-m", "WIP: checkpoint (auto)"); err != nil {
		d.logger.Printf("checkpoint_dog: git commit failed in %s/%s: %v", rigName, polecatName, err)
		return false
	}

	d.logger.Printf("checkpoint_dog: created WIP checkpoint in %s/%s", rigName, polecatName)
	return true
}

// CheckpointAndPushWorktree commits any dirty WIP in a polecat worktree and
// pushes the branch to origin. This preserves work from dead sessions before
// the worktree is cleaned up (crash recovery or idle reap).
//
// Returns true if work was preserved (committed and/or pushed).
func (d *Daemon) CheckpointAndPushWorktree(polecatDir, rigName, polecatName string) bool {
	gitRoot := findPolecatGitRoot(polecatDir, rigName)
	if gitRoot == "" {
		d.logger.Printf("checkpoint_dog: preserve: no git root found in %s/%s", rigName, polecatName)
		return false
	}

	// Step 1: commit any dirty work
	committed := d.commitWIPCheckpoint(gitRoot, rigName, polecatName)

	// Step 2: determine the current branch
	branch, err := runGitCmd(gitRoot, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		d.logger.Printf("checkpoint_dog: preserve: cannot determine branch in %s/%s: %v", rigName, polecatName, err)
		return committed
	}

	// Don't push if on main/master — that would be dangerous
	if branch == "main" || branch == "master" || branch == "HEAD" {
		d.logger.Printf("checkpoint_dog: preserve: skipping push for %s/%s (on branch %s)", rigName, polecatName, branch)
		return committed
	}

	// Step 3: check if there are commits to push (any commits ahead of origin/main)
	logOut, err := runGitCmd(gitRoot, "log", "--oneline", "origin/main..HEAD")
	if err != nil || strings.TrimSpace(logOut) == "" {
		// No commits ahead of main — nothing to push
		return committed
	}

	// Step 4: push the branch to origin
	if _, err := runGitCmd(gitRoot, "push", "origin", branch, "--force-with-lease"); err != nil {
		d.logger.Printf("checkpoint_dog: preserve: git push failed in %s/%s: %v", rigName, polecatName, err)
		return committed
	}

	d.logger.Printf("checkpoint_dog: preserved WIP for dead session %s/%s — pushed branch %s to origin", rigName, polecatName, branch)
	return true
}

// runGitCmd executes a git command in the given directory and returns stdout.
func runGitCmd(workDir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = workDir
	util.SetDetachedProcessGroup(cmd)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return "", fmt.Errorf("%s: %s", err, errMsg)
		}
		return "", err
	}

	return strings.TrimSpace(stdout.String()), nil
}
