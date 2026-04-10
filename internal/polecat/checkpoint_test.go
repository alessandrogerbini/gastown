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
