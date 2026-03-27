# Gas Town Shakedown Bug List

Compiled: 2026-03-27
Source: gt-iim shakedown report + individual bug beads + quartz research (gt-c05)

## Summary Table

| Bead | Title | Priority | Status | Root Cause Known? |
|------|-------|----------|--------|-------------------|
| gt-4ao | SetAgentState FK violation on every polecat spawn | P1 | open | Yes |
| gt-2p7 | Polecat not auto-nuking on session end | P1 | open | Partial |
| gt-ivf | Orphaned dependency references in bd doctor | P2 | open | Yes |
| gt-8m1 | bd close on already-closed bead succeeds silently | P3 | open | Yes |
| gt-zam | Runtime/backup files tracked by git in .beads/ | P3 | open | Yes |
| gt-8c1 | gt dolt cleanup doesn't detect orphaned 'te' database | P3 | open | Yes |
| n/a | Witness too aggressive — killed quartz (gt-c05) mid-work | P3 | open | Suspected |
| n/a | Cannot inspect another agent's mail inbox | P3 | open | Yes |
| n/a | gt plugin status command missing (desire path) | P4 | open | N/A (feature gap) |
| n/a | bd lint flags wisps for missing acceptance criteria | P4 | open | N/A (template gap) |

---

## P1 — High Priority

### gt-4ao: SetAgentState FK violation on every polecat spawn

- **Priority:** P1
- **Status:** open
- **Type:** bug

**Root cause:** `bd create --type=agent` creates EPHEMERAL beads (not written to Dolt SQL `issues` table). `bd set-state` then creates child events requiring a FK reference to the `issues` table. Since the agent bead is ephemeral (not in SQL), the `child_counters` INSERT fails the FK check on `fk_counter_parent`.

**Repro steps:**
1. `gt sling <any-bead> gastown`
2. Observe SetAgentState warnings during polecat spawn

**Error:**
```
Error 1452 (HY000): cannot add or update a child row -
Foreign key violation on fk: fk_counter_parent, table: child_counters,
referenced table: issues, key: [gt-gastown-polecat-<name>]
```

**Impact:** 100% repro rate. Adds ~60s latency to every polecat spawn due to 7-10 retry attempts with exponential backoff. Floods logs with FK violation errors. Observed on both obsidian and quartz spawns (4 times on 2026-03-27).

**Fix direction:** In `UpdateAgentState`, don't fail-hard on `set-state` error; fall through to `UpdateAgentDescriptionFields` which works on ephemeral beads. The description `agent_state` is already the fallback read path.

**Related:** hq-cv-mnlc6

---

### gt-2p7: Polecat not auto-nuking on session end

- **Priority:** P1
- **Status:** open
- **Type:** bug

**Root cause (partial):** Polecat session ended without calling `gt done`. Possible causes: (1) polecat didn't invoke `/done` before session ended (context exhaustion), (2) `/done` skill failed silently, or (3) witness patrol not detecting dead sessions fast enough.

**Repro steps:**
1. Observed on gastown/obsidian working gt-4ao
2. Session ended naturally after writing root cause analysis to bead notes
3. Post-session state: polecat not nuked, bead still `in_progress`, worktree still exists

**Impact:** High. Orphaned polecats block future slings to the same name, waste disk space, and leave beads stuck in `in_progress`. Requires manual witness intervention to clean up.

**Related:** Observed during shakedown when obsidian was working on gt-4ao FK violation analysis.

---

## P2 — Medium Priority

### gt-ivf: Orphaned dependency references in bd doctor

- **Priority:** P2
- **Status:** open
- **Type:** bug

**Root cause:** Dependency records point to beads that no longer exist in the database. Likely caused by beads being deleted without cleaning up their dependency edges.

**Repro steps:**
1. Run `bd doctor` in gastown rig
2. Observe orphaned dependency warnings:
   - `gt-71w` depends on `gt-wisp-6avv` (not found)
   - `gt-iim` depends on `gt-wisp-u8nq` (not found)
   - `gt-ji4` depends on `gt-wisp-w73l` (not found)

**Impact:** Medium. Could confuse dependency resolution (`bd blocked`, `bd ready`), producing incorrect results about which beads are blocked.

**Fix direction:** Run `bd doctor --fix` to remove orphaned dependencies. Also fix the deletion path to cascade-remove dependency edges when a bead is deleted.

---

## P3 — Low-Medium Priority

### gt-8m1: bd close on already-closed bead succeeds silently

- **Priority:** P3
- **Status:** open
- **Type:** bug

**Root cause:** `bd close` doesn't check current status before applying the close operation.

**Repro steps:**
1. `bd create --title='test' --type=task`
2. `bd close <id>`
3. `bd close <id> --reason='Double close test'`
4. Second close prints `Closed <id>` — no warning

**Impact:** Low. No data corruption, but confusing for operators. Could mask automation bugs where the same bead is closed multiple times.

---

### gt-zam: Runtime/backup files tracked by git in .beads/

- **Priority:** P3
- **Status:** open
- **Type:** bug

**Root cause:** `.gitignore` missing entries for `.beads/backup/` runtime files.

**Repro steps:**
1. Run `bd doctor` in gastown rig
2. Observe 7 tracked runtime files:
   - `.beads/backup/backup_state.json`
   - `.beads/backup/comments.jsonl`
   - `.beads/backup/config.jsonl`
   - +4 more backup/runtime files

**Impact:** Low. Runtime state leaking into git history. Could cause merge conflicts or expose sensitive runtime data.

**Fix direction:** Add `.beads/backup/` to `.gitignore` and `git rm --cached` the tracked files.

---

### gt-8c1: gt dolt cleanup doesn't detect orphaned 'te' database

- **Priority:** P3
- **Status:** open
- **Type:** bug

**Root cause:** `gt dolt cleanup` detection logic doesn't match `gt dolt list` orphan detection. The `te` database has full schema (22 tables) but 0 issues.

**Repro steps:**
1. `gt dolt list` — shows `te (orphan)`
2. `gt dolt cleanup --dry-run` — says "No orphaned databases found"

**Impact:** Medium-low. Orphaned database wastes disk space. The cleanup tool's purpose is exactly to catch this, but it misses it.

---

### Witness too aggressive — killed quartz (gt-c05) mid-work

- **Priority:** P3
- **Status:** open (no bead filed)
- **Type:** bug

**Root cause (suspected):** Witness zombie patrol killed quartz's first research session before it finished. Quartz (gt-c05) was the first research polecat run and was terminated mid-work.

**Repro steps:**
1. Observed during shakedown: quartz dispatched to gt-c05
2. Witness terminated the session before quartz completed
3. Required re-dispatch to complete the work

**Impact:** Medium. Premature termination wastes compute and delays work. If the witness health-check thresholds are too aggressive, it could cause repeated spawn storms for long-running tasks.

**Note:** This needs investigation — was quartz actually stuck, or was the witness threshold too low?

---

### Cannot inspect another agent's mail inbox

- **Priority:** P3
- **Status:** open (no bead filed)
- **Type:** bug/feature gap

**Root cause:** `gt mail inbox --target <agent>` flag doesn't exist.

**Repro steps:**
1. `gt mail inbox --target gastown/witness`
2. Error: "unknown flag: --target"

**Impact:** Low. Operators cannot easily audit mail queues for debugging. Workaround unclear.

---

## P4 — Low Priority

### gt plugin status command missing (desire path)

- **Priority:** P4
- **Status:** open (no bead filed)
- **Type:** feature gap (desire path)

**Description:** No `gt plugin status` command to see a summary view of all plugin run times and statuses. Must check each plugin individually with `gt plugin history <name>`.

**Impact:** Low. Desire path for operators doing health checks.

---

### bd lint flags wisps for missing acceptance criteria

- **Priority:** P4
- **Status:** open (no bead filed)
- **Type:** template gap

**Description:** `bd lint` warns about missing "Success Criteria" on epic wisps (e.g., `gt-wisp-u8nq`). Wisps may not need the same template fields as regular beads.

**Impact:** Low. Noisy lint output.

---

## Cross-References

| Bug | Related Beads/Convoys |
|-----|----------------------|
| gt-4ao (FK violation) | hq-cv-mnlc6 |
| gt-2p7 (auto-nuke) | gt-4ao (was working this when session died) |
| gt-ivf (orphaned deps) | gt-71w, gt-wisp-6avv, gt-iim, gt-wisp-u8nq, gt-ji4, gt-wisp-w73l |
| gt-c05 (quartz killed) | gt-2p7 (related failure mode) |

## Shakedown Context

- **Source report:** gt-iim (DESIGN field contains full 64-check shakedown)
- **Research report:** gt-c05 / `mayor/rig/shakedown-research.md` (quartz findings)
- **Date:** 2026-03-27
- **Polecats tested:** obsidian, quartz
- **Overall system health:** Fundamentally healthy. 64/64 checks passed. 7 bugs found (0 P0, 2 P1, 1 P2, 4 P3, 2 P4). The FK violation (gt-4ao) is the most impactful active bug with 100% repro rate.
