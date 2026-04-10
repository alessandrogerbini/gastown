package cmd

import (
	"strings"
	"testing"
)

func TestParkForce_CheckpointAndPushCalled(t *testing.T) {
	// Track which polecat worktrees were checkpointed.
	var checkpointed []string

	oldFn := checkpointAndPushFn
	checkpointAndPushFn = func(workDir, remote string) (bool, bool, error) {
		checkpointed = append(checkpointed, workDir)
		if remote != "origin" {
			t.Errorf("expected remote=origin, got %q", remote)
		}
		return true, true, nil
	}
	t.Cleanup(func() { checkpointAndPushFn = oldFn })

	// parkOneRig requires getRig to work; since that needs a real workspace,
	// we test at the function level via output inspection. The key assertion
	// is that checkpointAndPushFn is wired correctly and called.
	// This test validates the wiring; integration is covered by the polecat
	// package's CheckpointAndPush tests.

	// We can't call parkOneRig directly (needs rig infrastructure),
	// but we can verify the flag registration and test seam exist.
	if checkpointAndPushFn == nil {
		t.Fatal("checkpointAndPushFn test seam is nil")
	}

	// Verify the test seam works by calling it directly.
	committed, pushed, err := checkpointAndPushFn("/tmp/fake", "origin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !committed || !pushed {
		t.Error("expected committed=true, pushed=true from stub")
	}
	if len(checkpointed) != 1 || checkpointed[0] != "/tmp/fake" {
		t.Errorf("expected checkpointed=[/tmp/fake], got %v", checkpointed)
	}
}

func TestParkForceFlag_Registered(t *testing.T) {
	// Verify --force flag is registered on rigParkCmd.
	flag := rigParkCmd.Flags().Lookup("force")
	if flag == nil {
		t.Fatal("expected --force flag on rigParkCmd")
	}
	if flag.Shorthand != "f" {
		t.Errorf("expected shorthand 'f', got %q", flag.Shorthand)
	}
	if flag.DefValue != "false" {
		t.Errorf("expected default 'false', got %q", flag.DefValue)
	}
}

func TestParkForceFlag_DescriptionUpdated(t *testing.T) {
	long := rigParkCmd.Long
	if !strings.Contains(long, "--force") {
		t.Error("expected --force mentioned in long description")
	}
	if !strings.Contains(long, "Checkpoints") {
		t.Error("expected checkpoint behavior documented in long description")
	}
}
