package polecat

import (
	"fmt"
	"strings"

	"github.com/steveyegge/gastown/internal/git"
)

// RuntimeExcludeDirs are directories to unstage after git add -A.
// These contain runtime/ephemeral data that should not be checkpointed.
var RuntimeExcludeDirs = []string{
	".claude/",
	".beads/",
	".runtime/",
	"__pycache__/",
}

// IsDirty reports whether the worktree at workDir has uncommitted changes,
// excluding runtime directories that are not checkpointable.
func IsDirty(workDir string) (bool, error) {
	g := git.NewGit(workDir)
	status, err := g.Status()
	if err != nil {
		return false, fmt.Errorf("git status: %w", err)
	}
	if status.Clean {
		return false, nil
	}

	// Filter out runtime-only changes: if every dirty file is under an
	// excluded directory prefix, the worktree is effectively clean.
	for _, fileList := range [][]string{
		status.Modified, status.Added, status.Deleted, status.Untracked,
	} {
		for _, f := range fileList {
			if !isRuntimePath(f) {
				return true, nil
			}
		}
	}
	return false, nil
}

// CheckpointWorktree creates a WIP checkpoint commit for a single worktree.
// It stages all changes, unstages runtime/ephemeral directories, and commits
// whatever remains. Returns true if a checkpoint commit was created.
func CheckpointWorktree(workDir string) (bool, error) {
	g := git.NewGit(workDir)

	// Check if there's anything to checkpoint.
	dirty, err := IsDirty(workDir)
	if err != nil {
		return false, err
	}
	if !dirty {
		return false, nil
	}

	// Stage everything.
	if err := g.Add("-A"); err != nil {
		return false, fmt.Errorf("git add -A: %w", err)
	}

	// Unstage runtime/ephemeral directories.
	for _, dir := range RuntimeExcludeDirs {
		// ResetFiles is safe even if dir doesn't exist (exits 0).
		_ = g.ResetFiles(dir)
	}

	// Check if anything is staged after exclusions.
	hasDiff, err := hasStagedChanges(g)
	if err != nil {
		return false, fmt.Errorf("checking staged changes: %w", err)
	}
	if !hasDiff {
		return false, nil
	}

	// Commit the checkpoint.
	if err := g.Commit("WIP: checkpoint (auto)"); err != nil {
		return false, fmt.Errorf("git commit: %w", err)
	}

	return true, nil
}

// hasStagedChanges returns true if the index has staged changes relative to HEAD.
func hasStagedChanges(g *git.Git) (bool, error) {
	hasChanges, err := g.HasStagedChanges()
	if err != nil {
		return false, err
	}
	return hasChanges, nil
}

// CheckpointAndPush checkpoints any dirty changes in the worktree and pushes
// the current branch to the remote. Returns (committed, pushed, error).
// If the worktree is clean but has unpushed commits, it pushes those.
func CheckpointAndPush(workDir, remote string) (committed bool, pushed bool, err error) {
	g := git.NewGit(workDir)

	// Checkpoint dirty changes first.
	committed, err = CheckpointWorktree(workDir)
	if err != nil {
		return false, false, fmt.Errorf("checkpoint: %w", err)
	}

	// Get current branch.
	branch, err := g.CurrentBranch()
	if err != nil {
		return committed, false, fmt.Errorf("current branch: %w", err)
	}

	// Don't push if on main/master — polecat branches only.
	if branch == "main" || branch == "master" || branch == "HEAD" {
		return committed, false, nil
	}

	// Check for unpushed commits (including any we just created).
	// Polecat branches typically lack upstream tracking, so UnpushedCommits
	// returns 0. Use CommitsAhead(origin/main, HEAD) as the reliable check.
	unpushed, _ := g.CommitsAhead("origin/main", "HEAD")
	if unpushed == 0 {
		return committed, false, nil
	}

	// Push the branch.
	if err := g.Push(remote, branch, false); err != nil {
		return committed, false, fmt.Errorf("push %s to %s: %w", branch, remote, err)
	}

	return committed, true, nil
}

// isRuntimePath returns true if the file path falls under one of the
// runtime exclude directories.
func isRuntimePath(path string) bool {
	for _, dir := range RuntimeExcludeDirs {
		if strings.HasPrefix(path, dir) {
			return true
		}
	}
	return false
}
