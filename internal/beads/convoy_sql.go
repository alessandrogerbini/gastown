package beads

import "fmt"

// ConvoyListSQLArgs returns the bd args for listing convoy-type issues via SQL,
// bypassing bd's --type flag validation which rejects 'convoy' because it is
// not in bd's hardcoded type list (bug, feature, task, epic, chore, decision).
//
// Parameters:
//   - statusFilter: if non-empty, filter by this exact status (e.g. "open")
//   - allStatuses: if true, include all statuses (overrides statusFilter)
//   - labelFilter: if non-empty, only include convoys with this label
//
// Returns args suitable for exec.Command("bd", args...) or runBdJSON(dir, args...).
func ConvoyListSQLArgs(statusFilter string, allStatuses bool, labelFilter string) []string {
	query := `SELECT id, title, description, status, priority, issue_type, owner, created_at, created_by, updated_at, assignee FROM issues WHERE issue_type = 'convoy'`

	if allStatuses {
		// No status filter — include all
	} else if statusFilter != "" {
		query += fmt.Sprintf(` AND status = '%s'`, statusFilter)
	} else {
		// Default: exclude closed (matches bd list default behavior)
		query += ` AND status != 'closed'`
	}

	if labelFilter != "" {
		query += fmt.Sprintf(` AND id IN (SELECT issue_id FROM labels WHERE label = '%s')`, labelFilter)
	}

	query += ` ORDER BY created_at DESC`

	return []string{"sql", "--json", query}
}

// ConvoyCreateFixType returns the bd args for a SQL UPDATE that sets
// issue_type='convoy' on a bead that was created with --type=task.
// This is needed because bd create --type=convoy fails validation.
//
// Usage: after a successful bd create with --type=task, run:
//
//	exec.Command("bd", ConvoyCreateFixType(beadID)...)
func ConvoyCreateFixType(beadID string) []string {
	query := fmt.Sprintf(`UPDATE issues SET issue_type = 'convoy' WHERE id = '%s'`, beadID)
	return []string{"sql", query}
}
