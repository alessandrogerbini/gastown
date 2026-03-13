package cmd

import (
	"testing"

	"github.com/steveyegge/gastown/internal/polecat"
)

func TestEffectivePolecatState(t *testing.T) {
	tests := []struct {
		name string
		item PolecatListItem
		want polecat.State
	}{
		{
			name: "session-running-done-becomes-working",
			item: PolecatListItem{
				State:          polecat.StateDone,
				SessionRunning: true,
			},
			want: polecat.StateWorking,
		},
		{
			name: "session-dead-working-becomes-done",
			item: PolecatListItem{
				State:          polecat.StateWorking,
				SessionRunning: false,
			},
			want: polecat.StateDone,
		},
		{
			name: "zombie-is-never-rewritten",
			item: PolecatListItem{
				State:          polecat.StateZombie,
				SessionRunning: false,
				Zombie:         true,
			},
			want: polecat.StateZombie,
		},
		{
			name: "idle-unchanged",
			item: PolecatListItem{
				State:          polecat.StateIdle,
				SessionRunning: false,
			},
			want: polecat.StateIdle,
		},
		{
			name: "session-running-heartbeat-stale-working-becomes-done",
			item: PolecatListItem{
				State:          polecat.StateWorking,
				SessionRunning: true,
				HeartbeatStale: true,
			},
			want: polecat.StateDone,
		},
		{
			name: "session-running-heartbeat-fresh-stays-working",
			item: PolecatListItem{
				State:          polecat.StateWorking,
				SessionRunning: true,
				HeartbeatStale: false,
			},
			want: polecat.StateWorking,
		},
		{
			name: "session-running-heartbeat-stale-done-stays-working",
			item: PolecatListItem{
				State:          polecat.StateDone,
				SessionRunning: true,
				HeartbeatStale: true,
			},
			want: polecat.StateWorking,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := effectivePolecatState(tt.item)
			if got != tt.want {
				t.Fatalf("effectivePolecatState() = %q, want %q", got, tt.want)
			}
		})
	}
}

