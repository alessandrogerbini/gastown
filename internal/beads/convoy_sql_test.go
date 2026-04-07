package beads

import (
	"strings"
	"testing"
)

func TestConvoyListSQLArgs_OpenStatus(t *testing.T) {
	args := ConvoyListSQLArgs("open", false, "")
	if len(args) != 3 {
		t.Fatalf("expected 3 args, got %d: %v", len(args), args)
	}
	if args[0] != "sql" {
		t.Errorf("args[0] = %q, want sql", args[0])
	}
	if args[1] != "--json" {
		t.Errorf("args[1] = %q, want --json", args[1])
	}
	query := args[2]
	if !strings.Contains(query, "issue_type = 'convoy'") {
		t.Errorf("query missing issue_type filter: %s", query)
	}
	if !strings.Contains(query, "status = 'open'") {
		t.Errorf("query missing status filter: %s", query)
	}
}

func TestConvoyListSQLArgs_AllStatuses(t *testing.T) {
	args := ConvoyListSQLArgs("", true, "")
	query := args[2]
	// Should not have a WHERE clause filtering by status
	if strings.Contains(query, "status =") || strings.Contains(query, "status !=") {
		t.Errorf("allStatuses=true should have no status filter: %s", query)
	}
	if !strings.Contains(query, "issue_type = 'convoy'") {
		t.Errorf("query missing issue_type filter: %s", query)
	}
}

func TestConvoyListSQLArgs_DefaultStatus(t *testing.T) {
	args := ConvoyListSQLArgs("", false, "")
	query := args[2]
	if !strings.Contains(query, "status != 'closed'") {
		t.Errorf("default should exclude closed: %s", query)
	}
}

func TestConvoyListSQLArgs_WithLabel(t *testing.T) {
	args := ConvoyListSQLArgs("open", false, "mountain")
	query := args[2]
	if !strings.Contains(query, "label = 'mountain'") {
		t.Errorf("query missing label filter: %s", query)
	}
}

func TestConvoyCreateFixType(t *testing.T) {
	args := ConvoyCreateFixType("hq-cv-abc123")
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d: %v", len(args), args)
	}
	if args[0] != "sql" {
		t.Errorf("args[0] = %q, want sql", args[0])
	}
	if !strings.Contains(args[1], "issue_type = 'convoy'") {
		t.Errorf("query missing type update: %s", args[1])
	}
	if !strings.Contains(args[1], "hq-cv-abc123") {
		t.Errorf("query missing bead ID: %s", args[1])
	}
}
