package polecat

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// initGitRepo creates a temporary git repo with an initial commit.
func initGitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "commit", "--allow-empty", "-m", "initial"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %s: %v", args, out, err)
		}
	}
	return dir
}

func TestIsDirty_CleanRepo(t *testing.T) {
	dir := initGitRepo(t)

	dirty, err := IsDirty(dir)
	if err != nil {
		t.Fatalf("IsDirty: %v", err)
	}
	if dirty {
		t.Error("expected clean repo to not be dirty")
	}
}

func TestIsDirty_WithModifiedFile(t *testing.T) {
	dir := initGitRepo(t)

	// Create and commit a file, then modify it.
	path := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "git", "add", "file.txt")
	run(t, dir, "git", "commit", "-m", "add file")
	if err := os.WriteFile(path, []byte("modified"), 0644); err != nil {
		t.Fatal(err)
	}

	dirty, err := IsDirty(dir)
	if err != nil {
		t.Fatalf("IsDirty: %v", err)
	}
	if !dirty {
		t.Error("expected modified file to make repo dirty")
	}
}

func TestIsDirty_RuntimeOnlyChanges(t *testing.T) {
	dir := initGitRepo(t)

	// Create a file only under a runtime-excluded directory.
	runtimeDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(runtimeDir, "state.json"), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	dirty, err := IsDirty(dir)
	if err != nil {
		t.Fatalf("IsDirty: %v", err)
	}
	if dirty {
		t.Error("expected runtime-only changes to not be considered dirty")
	}
}

func TestCheckpointWorktree_CleanRepo(t *testing.T) {
	dir := initGitRepo(t)

	created, err := CheckpointWorktree(dir)
	if err != nil {
		t.Fatalf("CheckpointWorktree: %v", err)
	}
	if created {
		t.Error("expected no checkpoint for clean repo")
	}
}

func TestCheckpointWorktree_WithChanges(t *testing.T) {
	dir := initGitRepo(t)

	if err := os.WriteFile(filepath.Join(dir, "work.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	created, err := CheckpointWorktree(dir)
	if err != nil {
		t.Fatalf("CheckpointWorktree: %v", err)
	}
	if !created {
		t.Error("expected checkpoint to be created for dirty repo")
	}

	// Verify the commit was made.
	out := runOutput(t, dir, "git", "log", "--oneline", "-1")
	if out == "" {
		t.Error("expected at least one commit")
	}
	if got := runOutput(t, dir, "git", "log", "--format=%s", "-1"); got != "WIP: checkpoint (auto)" {
		t.Errorf("commit message = %q, want %q", got, "WIP: checkpoint (auto)")
	}

	// Verify working tree is clean after checkpoint.
	dirty, err := IsDirty(dir)
	if err != nil {
		t.Fatalf("IsDirty: %v", err)
	}
	if dirty {
		t.Error("expected clean tree after checkpoint")
	}
}

func TestCheckpointWorktree_RuntimeExcluded(t *testing.T) {
	dir := initGitRepo(t)

	// Create only runtime files.
	for _, d := range RuntimeExcludeDirs {
		full := filepath.Join(dir, d)
		if err := os.MkdirAll(full, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(full, "data"), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	created, err := CheckpointWorktree(dir)
	if err != nil {
		t.Fatalf("CheckpointWorktree: %v", err)
	}
	if created {
		t.Error("expected no checkpoint when only runtime dirs are dirty")
	}
}

func TestIsRuntimePath(t *testing.T) {
	tests := []struct {
		path   string
		expect bool
	}{
		{".claude/state.json", true},
		{".beads/db.json", true},
		{".runtime/pid", true},
		{"__pycache__/mod.pyc", true},
		{"main.go", false},
		{"internal/polecat/checkpoint.go", false},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isRuntimePath(tt.path); got != tt.expect {
				t.Errorf("isRuntimePath(%q) = %v, want %v", tt.path, got, tt.expect)
			}
		})
	}
}

// initGitRepoWithRemote creates a git repo with a bare "origin" remote.
// Returns (workDir, bareDir).
func initGitRepoWithRemote(t *testing.T) (string, string) {
	t.Helper()

	// Create bare repo as remote.
	bare := t.TempDir()
	run(t, bare, "git", "init", "--bare", "--initial-branch=main")

	// Create working repo.
	dir := t.TempDir()
	cmds := [][]string{
		{"git", "init", "--initial-branch=main"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "remote", "add", "origin", bare},
		{"git", "commit", "--allow-empty", "-m", "initial"},
		{"git", "push", "-u", "origin", "main"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %s: %v", args, out, err)
		}
	}
	return dir, bare
}

func TestCheckpointAndPush_CleanRepo(t *testing.T) {
	dir, _ := initGitRepoWithRemote(t)

	committed, pushed, err := CheckpointAndPush(dir, "origin")
	if err != nil {
		t.Fatalf("CheckpointAndPush: %v", err)
	}
	if committed || pushed {
		t.Errorf("expected no action for clean repo, got committed=%v pushed=%v", committed, pushed)
	}
}

func TestCheckpointAndPush_DirtyWorktree(t *testing.T) {
	dir, bare := initGitRepoWithRemote(t)

	// Switch to a polecat branch.
	run(t, dir, "git", "checkout", "-b", "polecat/alpha")

	// Create uncommitted work.
	if err := os.WriteFile(filepath.Join(dir, "work.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	committed, pushed, err := CheckpointAndPush(dir, "origin")
	if err != nil {
		t.Fatalf("CheckpointAndPush: %v", err)
	}
	if !committed {
		t.Error("expected committed=true")
	}
	if !pushed {
		t.Error("expected pushed=true")
	}

	// Verify the branch was pushed to the bare remote.
	out := runOutput(t, bare, "git", "branch")
	if !filepath.IsAbs(bare) {
		t.Skip("bare dir not absolute")
	}
	cmd := exec.Command("git", "branch", "--list", "polecat/alpha")
	cmd.Dir = bare
	branchOut, _ := cmd.CombinedOutput()
	if len(branchOut) == 0 {
		t.Error("expected polecat/alpha branch on remote")
	}
	_ = out
}

func TestCheckpointAndPush_UnpushedCommitsOnly(t *testing.T) {
	dir, _ := initGitRepoWithRemote(t)

	// Switch to a polecat branch.
	run(t, dir, "git", "checkout", "-b", "polecat/beta")

	// Create a committed but unpushed change.
	if err := os.WriteFile(filepath.Join(dir, "done.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	run(t, dir, "git", "add", "done.go")
	run(t, dir, "git", "commit", "-m", "add done.go")

	committed, pushed, err := CheckpointAndPush(dir, "origin")
	if err != nil {
		t.Fatalf("CheckpointAndPush: %v", err)
	}
	if committed {
		t.Error("expected committed=false (nothing dirty)")
	}
	if !pushed {
		t.Error("expected pushed=true (unpushed commit)")
	}
}

func TestCheckpointAndPush_SkipsMainBranch(t *testing.T) {
	dir, _ := initGitRepoWithRemote(t)

	// Create uncommitted work on main.
	if err := os.WriteFile(filepath.Join(dir, "oops.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}

	committed, pushed, err := CheckpointAndPush(dir, "origin")
	if err != nil {
		t.Fatalf("CheckpointAndPush: %v", err)
	}
	if !committed {
		t.Error("expected committed=true (dirty files)")
	}
	if pushed {
		t.Error("expected pushed=false (should not push main)")
	}
}

func run(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v failed: %s: %v", args, out, err)
	}
}

func runOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v failed: %s: %v", args, out, err)
	}
	return string(out[:len(out)-1]) // trim trailing newline
}
