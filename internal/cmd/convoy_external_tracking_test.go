package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
)

func writeExternalTrackingBdStub(t *testing.T, scriptBody string) {
	t.Helper()

	binDir := t.TempDir()
	bdPath := filepath.Join(binDir, "bd")
	script := "#!/bin/sh\n" + scriptBody
	if err := os.WriteFile(bdPath, []byte(script), 0755); err != nil {
		t.Fatalf("write bd stub: %v", err)
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func chdirExternalTrackingTest(t *testing.T, dir string) {
	t.Helper()

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })
}

func makeExternalTrackingTownWorkspace(t *testing.T) (string, string, string) {
	t.Helper()

	townRoot := t.TempDir()
	townBeads := filepath.Join(townRoot, ".beads")
	if err := os.MkdirAll(townBeads, 0755); err != nil {
		t.Fatalf("mkdir .beads: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(townRoot, "mayor"), 0755); err != nil {
		t.Fatalf("mkdir mayor: %v", err)
	}
	if err := os.WriteFile(filepath.Join(townRoot, "mayor", "town.json"), []byte(`{"name":"test-town"}`), 0644); err != nil {
		t.Fatalf("write town.json: %v", err)
	}

	expectedWD := townRoot
	if resolved, err := filepath.EvalSymlinks(townRoot); err == nil && resolved != "" {
		expectedWD = resolved
	}
	return townRoot, townBeads, expectedWD
}

func TestGetTrackedIssues_FallsBackToShowTrackedDependencies(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows - shell stubs")
	}

	townRoot, townBeads, _ := makeExternalTrackingTownWorkspace(t)
	chdirExternalTrackingTest(t, townRoot)

	scriptBody := fmt.Sprintf(`
case "$*" in
  "dep list hq-cv-ext --direction=down --type=tracks --json")
    echo '[]'
    ;;
  "show hq-cv-ext --json")
    echo '[{"id":"hq-cv-ext","title":"External convoy","status":"open","issue_type":"convoy","dependencies":[{"id":"external:ghostty:ghostty-123","title":"Ghost 123","status":"open","type":"task","dependency_type":"tracks"},{"id":"external:ghostty:ghostty-456","title":"Ghost 456","status":"closed","type":"task","dependency_type":"tracks"},{"id":"gt-ignore","title":"Ignore me","status":"open","type":"task","dependency_type":"blocks"}]}]'
    ;;
  "show ghostty-123 ghostty-456 --json"|"show ghostty-456 ghostty-123 --json")
    echo '[{"id":"ghostty-123","title":"Ghost 123","status":"open","issue_type":"task"},{"id":"ghostty-456","title":"Ghost 456","status":"closed","issue_type":"task"}]'
    ;;
  "show ghostty-123 --json")
    echo '[{"id":"ghostty-123","title":"Ghost 123","status":"open","issue_type":"task"}]'
    ;;
  "show ghostty-456 --json")
    echo '[{"id":"ghostty-456","title":"Ghost 456","status":"closed","issue_type":"task"}]'
    ;;
  *)
    echo "unexpected bd args: $*" >&2
    exit 1
    ;;
esac
`)
	writeExternalTrackingBdStub(t, scriptBody)

	tracked, err := getTrackedIssues(townBeads, "hq-cv-ext")
	if err != nil {
		t.Fatalf("getTrackedIssues: %v", err)
	}
	if len(tracked) != 2 {
		t.Fatalf("expected 2 tracked issues, got %d", len(tracked))
	}

	ids := []string{tracked[0].ID, tracked[1].ID}
	sort.Strings(ids)
	if ids[0] != "ghostty-123" || ids[1] != "ghostty-456" {
		t.Fatalf("unexpected tracked IDs: %v", ids)
	}

	statusByID := map[string]string{}
	for _, item := range tracked {
		statusByID[item.ID] = item.Status
	}
	if statusByID["ghostty-123"] != "open" || statusByID["ghostty-456"] != "closed" {
		t.Fatalf("unexpected tracked statuses: %#v", statusByID)
	}
}

// TestGetTrackedIssues_RawSQLResolvesCrossDBDeps verifies that getTrackedIssues
// uses bdDepListRawIDs (raw SQL) as the primary path, which works for
// cross-database deps where bd dep list returns empty. This is the core
// fix for GH #gt-7zm where convoys showed 0/0 and auto-closed.
func TestGetTrackedIssues_RawSQLResolvesCrossDBDeps(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows - shell stubs")
	}

	townRoot, townBeads, _ := makeExternalTrackingTownWorkspace(t)
	chdirExternalTrackingTest(t, townRoot)

	// Mock bd: sql returns cross-db deps (the working path),
	// dep list returns empty (the broken path that caused the bug).
	scriptBody := `
# Find the subcommand (first non-flag arg)
cmd=""
for arg in "$@"; do
  case "$arg" in
    --*) ;;
    *) cmd="$arg"; break ;;
  esac
done

case "$cmd" in
  sql)
    # bdDepListRawIDs: raw SQL returns cross-database deps correctly
    case "$*" in
      *"issue_id = 'hq-cv-crossdb'"*tracks*)
        echo '[{"depends_on_id":"external:te:te-xt6"},{"depends_on_id":"external:te:te-zkh"},{"depends_on_id":"external:te:te-dkr"}]'
        ;;
      *)
        echo '[]'
        ;;
    esac
    ;;
  dep)
    # bd dep list returns empty for cross-db deps (the bug)
    echo '[]'
    ;;
  show)
    # Return issue details for batch show
    echo '[{"id":"te-xt6","title":"Issue 1","status":"open","issue_type":"task"},{"id":"te-zkh","title":"Issue 2","status":"closed","issue_type":"task"},{"id":"te-dkr","title":"Issue 3","status":"open","issue_type":"bug"}]'
    ;;
  *)
    exit 0
    ;;
esac
`
	writeExternalTrackingBdStub(t, scriptBody)

	tracked, err := getTrackedIssues(townBeads, "hq-cv-crossdb")
	if err != nil {
		t.Fatalf("getTrackedIssues: %v", err)
	}
	if len(tracked) != 3 {
		t.Fatalf("expected 3 tracked issues, got %d", len(tracked))
	}

	ids := make([]string, len(tracked))
	for i, item := range tracked {
		ids[i] = item.ID
	}
	sort.Strings(ids)
	if ids[0] != "te-dkr" || ids[1] != "te-xt6" || ids[2] != "te-zkh" {
		t.Fatalf("unexpected tracked IDs: %v", ids)
	}

	statusByID := map[string]string{}
	for _, item := range tracked {
		statusByID[item.ID] = item.Status
	}
	if statusByID["te-xt6"] != "open" {
		t.Errorf("te-xt6 status = %q, want open", statusByID["te-xt6"])
	}
	if statusByID["te-zkh"] != "closed" {
		t.Errorf("te-zkh status = %q, want closed", statusByID["te-zkh"])
	}
	if statusByID["te-dkr"] != "open" {
		t.Errorf("te-dkr status = %q, want open", statusByID["te-dkr"])
	}
}
