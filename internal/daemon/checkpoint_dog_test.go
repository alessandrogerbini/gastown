package daemon

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// newTestLoggerWithBuf returns a logger and the underlying buffer for inspection.
func newTestLoggerWithBuf() (*log.Logger, *strings.Builder) {
	var buf strings.Builder
	return log.New(&buf, "", 0), &buf
}

func TestCheckpointDogInterval_Default(t *testing.T) {
	interval := checkpointDogInterval(nil)
	if interval != defaultCheckpointDogInterval {
		t.Errorf("expected default interval %v, got %v", defaultCheckpointDogInterval, interval)
	}
}

func TestCheckpointDogInterval_NilPatrols(t *testing.T) {
	config := &DaemonPatrolConfig{}
	interval := checkpointDogInterval(config)
	if interval != defaultCheckpointDogInterval {
		t.Errorf("expected default interval %v, got %v", defaultCheckpointDogInterval, interval)
	}
}

func TestCheckpointDogInterval_NilCheckpointDog(t *testing.T) {
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{},
	}
	interval := checkpointDogInterval(config)
	if interval != defaultCheckpointDogInterval {
		t.Errorf("expected default interval %v, got %v", defaultCheckpointDogInterval, interval)
	}
}

func TestCheckpointDogInterval_Configured(t *testing.T) {
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			CheckpointDog: &CheckpointDogConfig{
				Enabled:     true,
				IntervalStr: "5m",
			},
		},
	}
	interval := checkpointDogInterval(config)
	if interval != 5*time.Minute {
		t.Errorf("expected 5m, got %v", interval)
	}
}

func TestCheckpointDogInterval_InvalidFallsBack(t *testing.T) {
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			CheckpointDog: &CheckpointDogConfig{
				Enabled:     true,
				IntervalStr: "not-a-duration",
			},
		},
	}
	interval := checkpointDogInterval(config)
	if interval != defaultCheckpointDogInterval {
		t.Errorf("expected default interval for invalid config, got %v", interval)
	}
}

func TestCheckpointDogInterval_ZeroFallsBack(t *testing.T) {
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			CheckpointDog: &CheckpointDogConfig{
				Enabled:     true,
				IntervalStr: "0s",
			},
		},
	}
	interval := checkpointDogInterval(config)
	if interval != defaultCheckpointDogInterval {
		t.Errorf("expected default interval for zero config, got %v", interval)
	}
}

func TestCheckpointDogEnabled(t *testing.T) {
	// Nil config → disabled (opt-in patrol)
	if IsPatrolEnabled(nil, "checkpoint_dog") {
		t.Error("expected checkpoint_dog disabled for nil config")
	}

	// Explicitly enabled
	config := &DaemonPatrolConfig{
		Patrols: &PatrolsConfig{
			CheckpointDog: &CheckpointDogConfig{
				Enabled: true,
			},
		},
	}
	if !IsPatrolEnabled(config, "checkpoint_dog") {
		t.Error("expected checkpoint_dog enabled")
	}

	// Explicitly disabled
	config.Patrols.CheckpointDog.Enabled = false
	if IsPatrolEnabled(config, "checkpoint_dog") {
		t.Error("expected checkpoint_dog disabled when Enabled=false")
	}
}

// TestFindPolecatGitRoot_NewStructure tests that findPolecatGitRoot finds the
// git root at polecats/<name>/<rigname>/ (new worktree structure).
func TestFindPolecatGitRoot_NewStructure(t *testing.T) {
	polecatDir := t.TempDir()
	rigName := "myrig"

	// Create polecats/<name>/<rigname>/.git (new structure)
	gitRoot := filepath.Join(polecatDir, rigName)
	if err := os.MkdirAll(gitRoot, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(gitRoot, ".git"), []byte("gitdir: ../../.repo.git/worktrees/x"), 0644); err != nil {
		t.Fatal(err)
	}

	got := findPolecatGitRoot(polecatDir, rigName)
	if got != gitRoot {
		t.Errorf("expected %s, got %s", gitRoot, got)
	}
}

// TestFindPolecatGitRoot_OldStructure tests that findPolecatGitRoot finds the
// git root at polecats/<name>/ (old worktree structure, backward compat).
func TestFindPolecatGitRoot_OldStructure(t *testing.T) {
	polecatDir := t.TempDir()

	// Create polecats/<name>/.git (old structure)
	if err := os.WriteFile(filepath.Join(polecatDir, ".git"), []byte("gitdir: ../.repo.git/worktrees/x"), 0644); err != nil {
		t.Fatal(err)
	}

	got := findPolecatGitRoot(polecatDir, "myrig")
	if got != polecatDir {
		t.Errorf("expected %s, got %s", polecatDir, got)
	}
}

// TestFindPolecatGitRoot_FallbackScan tests that findPolecatGitRoot falls back
// to scanning subdirectories when the rig name doesn't match.
func TestFindPolecatGitRoot_FallbackScan(t *testing.T) {
	polecatDir := t.TempDir()

	// Create polecats/<name>/other-repo/.git (rig name mismatch)
	otherRepo := filepath.Join(polecatDir, "other-repo")
	if err := os.MkdirAll(otherRepo, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(otherRepo, ".git"), []byte("gitdir: ..."), 0644); err != nil {
		t.Fatal(err)
	}

	got := findPolecatGitRoot(polecatDir, "myrig")
	if got != otherRepo {
		t.Errorf("expected %s, got %s", otherRepo, got)
	}
}

// TestFindPolecatGitRoot_NoGit tests that findPolecatGitRoot returns empty
// when no git root is found.
func TestFindPolecatGitRoot_NoGit(t *testing.T) {
	polecatDir := t.TempDir()

	got := findPolecatGitRoot(polecatDir, "myrig")
	if got != "" {
		t.Errorf("expected empty string, got %s", got)
	}
}

// initTestGitRepo creates a real git repo with an initial commit at the given path.
func initTestGitRepo(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init"},
		{"config", "user.email", "test@test.com"},
		{"config", "user.name", "Test"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	// Create an initial commit so HEAD exists
	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte("# test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"add", "README.md"},
		{"commit", "-m", "initial commit"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
}

// TestCommitWIPCheckpoint_CommitsDirtyWork tests that commitWIPCheckpoint
// creates a WIP commit when there are uncommitted changes.
func TestCommitWIPCheckpoint_CommitsDirtyWork(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix git repos")
	}

	dir := t.TempDir()
	initTestGitRepo(t, dir)

	// Create a dirty file
	if err := os.WriteFile(filepath.Join(dir, "dirty.txt"), []byte("work in progress"), 0644); err != nil {
		t.Fatal(err)
	}

	logger, _ := newTestLoggerWithBuf()
	d := &Daemon{logger: logger}
	committed := d.commitWIPCheckpoint(dir, "testrig", "testcat")
	if !committed {
		t.Error("expected commitWIPCheckpoint to return true for dirty worktree")
	}

	// Verify the commit exists
	cmd := exec.Command("git", "log", "--oneline", "-1", "--format=%s")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log failed: %v", err)
	}
	subject := string(out)
	if !strings.Contains(subject, "WIP: checkpoint (auto)") {
		t.Errorf("expected WIP checkpoint commit message, got: %q", subject)
	}
}

// TestCommitWIPCheckpoint_SkipsCleanWorktree tests that commitWIPCheckpoint
// returns false for a clean worktree.
func TestCommitWIPCheckpoint_SkipsCleanWorktree(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix git repos")
	}

	dir := t.TempDir()
	initTestGitRepo(t, dir)

	logger, _ := newTestLoggerWithBuf()
	d := &Daemon{logger: logger}
	committed := d.commitWIPCheckpoint(dir, "testrig", "testcat")
	if committed {
		t.Error("expected commitWIPCheckpoint to return false for clean worktree")
	}
}

// TestCommitWIPCheckpoint_ExcludesRuntimeDirs tests that runtime directories
// (.claude/, .beads/, .runtime/) are excluded from checkpoint commits.
func TestCommitWIPCheckpoint_ExcludesRuntimeDirs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix git repos")
	}

	dir := t.TempDir()
	initTestGitRepo(t, dir)

	// Create files only in runtime directories
	for _, dirName := range []string{".claude", ".beads", ".runtime"} {
		runtimeDir := filepath.Join(dir, dirName)
		if err := os.MkdirAll(runtimeDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(runtimeDir, "data.json"), []byte("{}"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	logger, _ := newTestLoggerWithBuf()
	daemon := &Daemon{logger: logger}
	committed := daemon.commitWIPCheckpoint(dir, "testrig", "testcat")
	if committed {
		t.Error("expected commitWIPCheckpoint to return false when only runtime dirs are dirty")
	}
}

// TestCheckpointAndPushWorktree_CommitsAndPushes tests the full commit+push
// flow for preserving dead session WIP.
func TestCheckpointAndPushWorktree_CommitsAndPushes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix git repos")
	}

	// Create a bare "origin" repo
	bareDir := t.TempDir()
	cmd := exec.Command("git", "init", "--bare")
	cmd.Dir = bareDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare failed: %v\n%s", err, out)
	}

	// Create the working repo (simulating a polecat worktree)
	polecatDir := t.TempDir()
	rigName := "testrig"
	gitRoot := filepath.Join(polecatDir, rigName)
	if err := os.MkdirAll(gitRoot, 0755); err != nil {
		t.Fatal(err)
	}
	initTestGitRepo(t, gitRoot)

	// Add bare repo as origin
	for _, args := range [][]string{
		{"remote", "add", "origin", bareDir},
		{"checkout", "-b", "polecat/testcat-abc123"},
		{"push", "-u", "origin", "polecat/testcat-abc123"},
		// Also push to create an origin/main ref
		{"branch", "main"},
		{"push", "origin", "main"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = gitRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	// Create dirty files
	if err := os.WriteFile(filepath.Join(gitRoot, "work.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	logger, _ := newTestLoggerWithBuf()
	d := &Daemon{logger: logger}
	preserved := d.CheckpointAndPushWorktree(polecatDir, rigName, "testcat")
	if !preserved {
		t.Error("expected CheckpointAndPushWorktree to return true")
	}

	// Verify the commit was pushed to origin
	cmd = exec.Command("git", "log", "--oneline", "polecat/testcat-abc123", "-1", "--format=%s")
	cmd.Dir = bareDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git log on bare repo failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "WIP: checkpoint (auto)") {
		t.Errorf("expected WIP checkpoint commit on origin, got: %q", string(out))
	}
}

// TestCheckpointAndPushWorktree_SkipsMainBranch tests that CheckpointAndPushWorktree
// does not push when on the main branch.
func TestCheckpointAndPushWorktree_SkipsMainBranch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix git repos")
	}

	polecatDir := t.TempDir()
	rigName := "testrig"
	gitRoot := filepath.Join(polecatDir, rigName)
	if err := os.MkdirAll(gitRoot, 0755); err != nil {
		t.Fatal(err)
	}
	initTestGitRepo(t, gitRoot)

	// Rename the branch to main
	cmd := exec.Command("git", "branch", "-M", "main")
	cmd.Dir = gitRoot
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git branch -M main failed: %v\n%s", err, out)
	}

	// Create dirty files
	if err := os.WriteFile(filepath.Join(gitRoot, "work.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	logger, logBuf := newTestLoggerWithBuf()
	d := &Daemon{logger: logger}
	d.CheckpointAndPushWorktree(polecatDir, rigName, "testcat")

	if !strings.Contains(logBuf.String(), "skipping push") {
		t.Errorf("expected 'skipping push' log for main branch, got: %q", logBuf.String())
	}
}

// TestCheckpointAndPushWorktree_CleanWorktreeNothingToPush tests that
// CheckpointAndPushWorktree handles clean worktrees with no commits gracefully.
func TestCheckpointAndPushWorktree_CleanWorktreeNothingToPush(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test uses Unix git repos")
	}

	// Create a bare "origin" repo
	bareDir := t.TempDir()
	cmd := exec.Command("git", "init", "--bare")
	cmd.Dir = bareDir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init --bare failed: %v\n%s", err, out)
	}

	polecatDir := t.TempDir()
	rigName := "testrig"
	gitRoot := filepath.Join(polecatDir, rigName)
	if err := os.MkdirAll(gitRoot, 0755); err != nil {
		t.Fatal(err)
	}
	initTestGitRepo(t, gitRoot)

	// Add origin and push initial commit
	for _, args := range [][]string{
		{"remote", "add", "origin", bareDir},
		{"push", "-u", "origin", "master"},
		// Create main branch on origin for the origin/main..HEAD check
		{"branch", "main"},
		{"push", "origin", "main"},
		{"checkout", "-b", "polecat/testcat-abc123"},
		{"push", "-u", "origin", "polecat/testcat-abc123"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = gitRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	logger, _ := newTestLoggerWithBuf()
	d := &Daemon{logger: logger}
	preserved := d.CheckpointAndPushWorktree(polecatDir, rigName, "testcat")
	if preserved {
		t.Error("expected CheckpointAndPushWorktree to return false for clean worktree with no new commits")
	}
}
