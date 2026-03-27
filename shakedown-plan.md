# Gas Town Shakedown Test Plan — Refined

> Generated from research (gt-c05) and initial shakedown (gt-iim) findings.
> Ordered by likelihood of finding real bugs, informed by known failure modes.

---

## Priority Legend

| Priority | Meaning |
|----------|---------|
| P0 | System-breaking, blocks all operations |
| P1 | Data corruption or major functionality loss |
| P2 | Significant degradation, workarounds exist |
| P3 | Minor issue, cosmetic or edge case |

---

## Test Suite

### TEST-01: SetAgentState FK Violation on Polecat Spawn
- **Category:** Data Integrity / Dolt
- **Priority:** P0
- **Known Bug:** gt-4ao (100% repro, P1)
- **Root Cause:** `bd create --type=agent` creates ephemeral beads not written to Dolt SQL `issues` table. `bd set-state` then inserts child events requiring FK to `issues`. FK check fails because agent bead is ephemeral.
- **Steps:**
  1. From mayor/witness role, run `gt sling <any-open-bead> gastown`
  2. Monitor Dolt logs for FK violation errors on `fk_counter_parent`
  3. Time how long the retry loop takes (expected: ~60s, 7-10 retries)
  4. Verify the polecat eventually starts despite the error
  5. Verify `UpdateAgentDescriptionFields` fallback path works correctly
- **Expected Result:** After fix (gt-33m): no FK violation. Before fix: polecat starts with ~60s delay and log spam.
- **Cleanup:** None required (polecat auto-nukes on completion)
- **Parallel:** No — requires exclusive sling operation

### TEST-02: Polecat Auto-Nuke on Session End
- **Category:** Polecat Lifecycle
- **Priority:** P0
- **Known Bug:** gt-2p7 (P1 — polecat not auto-nuking)
- **Steps:**
  1. Sling a trivial bead (e.g., a no-op task) to a polecat
  2. Wait for polecat to complete `gt done`
  3. Verify tmux session is destroyed: `tmux has-session -t gastown-polecat-<name> 2>&1`
  4. Verify git worktree is removed: `git worktree list | grep <name>`
  5. Verify branch was pushed to origin before nuke
  6. Check witness detected the completion
- **Expected Result:** Session destroyed, worktree cleaned up, branch pushed, witness logged completion.
- **Cleanup:** If worktree persists, manual `git worktree remove`
- **Parallel:** No — requires observing single polecat lifecycle

### TEST-03: Dolt Database Integrity Check
- **Category:** Data Integrity / Dolt
- **Priority:** P0
- **Steps:**
  1. Run `bd doctor` on each rig database (gt, hq, tessera)
  2. Check for orphaned dependency references (known: gt-71w→gt-wisp-6avv, gt-iim→gt-wisp-u8nq, gt-ji4→gt-wisp-w73l)
  3. Run `bd lint` and categorize findings
  4. Verify `bd doctor --fix` resolves orphaned deps cleanly
  5. Query each database: `gt dolt sql -q "SELECT count(*) FROM issues" --db <db>`
  6. Verify Dolt commit counts are reasonable (not runaway growth)
  7. Check for the orphaned `te` database: `gt dolt list` — verify it shows as orphan
  8. Run `gt dolt cleanup --dry-run` — verify it detects `te` orphan (known bug gt-8c1: it doesn't)
- **Expected Result:** No errors from doctor (after --fix). Lint returns only template warnings. Databases respond. `te` orphan detected by cleanup.
- **Cleanup:** Run `bd doctor --fix` if orphans found
- **Parallel:** Yes — read-only queries can run concurrently

### TEST-04: Concurrent Polecat Spawn Race Condition
- **Category:** Race Conditions
- **Priority:** P1
- **Research Finding:** Race conditions in concurrent spawning identified as high-risk area
- **Steps:**
  1. Prepare two open beads with no blockers
  2. Sling both beads to the same rig in rapid succession (~1s apart)
  3. Monitor for: Dolt lock contention, FK violations, worktree conflicts
  4. Verify both polecats start with independent worktrees
  5. Check `git worktree list` shows two distinct entries
  6. Verify tmux sessions are independent: `tmux list-sessions | grep gastown-polecat`
  7. Let both complete and verify clean nuke of both
- **Expected Result:** Both polecats start cleanly (possibly with staggered timing). No Dolt corruption or worktree conflicts.
- **Cleanup:** Nuke any surviving worktrees/sessions
- **Parallel:** No — this IS the concurrency test

### TEST-05: Polecat `gt done` End-to-End Flow
- **Category:** Polecat Lifecycle / Merge Queue
- **Priority:** P1
- **Steps:**
  1. Sling a bead requiring a trivial code change
  2. Wait for polecat to complete implementation
  3. Monitor `gt done` execution:
     a. Branch pushed to origin
     b. MR bead created in merge queue
     c. Worktree nuked
     d. Session exited
  4. Verify `gt mq list gastown` shows the new MR
  5. Verify the MR bead has correct source branch and target (main)
  6. Check refinery picks up the MR and processes it
- **Expected Result:** Clean end-to-end: branch pushed → MR queued → worktree nuked → refinery processes.
- **Cleanup:** If MR stalls in queue, manually remove with `gt mq remove`
- **Parallel:** No — single lifecycle observation

### TEST-06: Witness Detection and Recovery
- **Category:** Witness / Monitoring
- **Priority:** P1
- **Steps:**
  1. Verify witness is running: `gt peek gastown/witness`
  2. Kill a polecat's tmux session abruptly: `tmux kill-session -t gastown-polecat-<name>`
  3. Wait for witness patrol cycle (check interval in witness config)
  4. Verify witness detects the dead session
  5. Verify witness logs the crash and attempts recovery
  6. Check if bead status is updated (should go back to open/available)
  7. Verify worktree is cleaned up by witness
- **Expected Result:** Witness detects crash within one patrol cycle, cleans up worktree, resets bead status.
- **Cleanup:** Manual cleanup if witness doesn't handle it
- **Parallel:** No — requires controlled failure injection

### TEST-07: Mail System Delivery and Ordering
- **Category:** Mail System
- **Priority:** P2
- **Steps:**
  1. Send mail to self: `gt mail send --self -s "Test 1" -m "First message"`
  2. Send mail to witness: `gt mail send gastown/witness -s "Test 2" -m "Second message"`
  3. Send mail to refinery: `gt mail send gastown/refinery -s "Test 3" -m "Third message"`
  4. Verify inbox shows correct message: `gt mail inbox`
  5. Read specific message: `gt mail read <id>`
  6. Mark as read: `gt mail mark-read <id>`
  7. Verify read status persists across commands
  8. Test nudge delivery: `gt nudge gastown/witness "Test nudge"`
  9. Verify nudge arrives (ephemeral, no Dolt commit)
- **Expected Result:** All mail delivered, read/unread tracking works, nudges delivered without Dolt overhead.
- **Cleanup:** None needed (mail is persistent by design)
- **Parallel:** Yes — sending is independent

### TEST-08: Beads State Transitions
- **Category:** Beads Integrity
- **Priority:** P2
- **Steps:**
  1. Create test bead: `bd create --title "Shakedown test bead" --type task`
  2. Transition: open → in_progress: `bd update <id> --status in_progress`
  3. Transition: in_progress → blocked: `bd update <id> --status blocked`
  4. Transition: blocked → in_progress: `bd update <id> --status in_progress`
  5. Transition: in_progress → closed: `bd close <id> --reason "test complete"`
  6. Test double-close (known bug gt-8m1): `bd close <id> --reason "double close"`
     - Expected: should warn "already closed" but currently succeeds silently
  7. Create dependency chain: A depends on B depends on C
  8. Verify `bd blocked` shows A and B correctly
  9. Close C, verify B becomes unblocked
  10. Close B, verify A becomes unblocked
  11. Test circular dependency rejection: try `bd dep add A C` (should fail)
- **Expected Result:** All transitions work. Double-close is silent (known bug). Dependency chain resolves correctly. Circular deps rejected.
- **Cleanup:** Close all test beads: `bd close <id> --reason "shakedown cleanup"`
- **Parallel:** Yes — independent of other tests

### TEST-09: Prefix Routing Across Rigs
- **Category:** Cross-Rig Operations
- **Priority:** P2
- **Steps:**
  1. Read routes.jsonl: verify entries for hq-, hq-cv-, te-, gt-
  2. From town root, `bd show gt-4ao` — verify routes to gastown
  3. From town root, `bd show hq-<some-id>` — verify routes to HQ
  4. Enable debug: `BD_DEBUG_ROUTING=1 bd show gt-4ao` — verify redirect chain
  5. Create bead with `--rig gastown`: verify prefix is gt-
  6. Create bead with `--rig beads` (if available): verify different prefix
  7. Test invalid prefix: `bd show zz-nonexistent` — verify clean error
- **Expected Result:** All routing works as documented. Debug output shows correct chain. Invalid prefixes produce clean errors.
- **Cleanup:** Close any test beads created
- **Parallel:** Yes — read-only routing tests

### TEST-10: Refinery Merge Queue Processing
- **Category:** Refinery / Merge Queue
- **Priority:** P2
- **Steps:**
  1. Verify refinery is running: `gt peek gastown/refinery`
  2. Check MQ status: `gt mq list gastown`
  3. If MQ has pending items, observe refinery processing
  4. If MQ is empty, submit a trivial MR (via polecat gt done) and observe:
     a. Refinery picks up MR
     b. Refinery runs gates (build, test, lint)
     c. Refinery merges to main on green
     d. Refinery closes the source bead
  5. Check refinery handles gate failure correctly (if observable)
- **Expected Result:** Refinery processes MRs, runs gates, merges green results, closes beads.
- **Cleanup:** None — refinery handles its own state
- **Parallel:** No — requires observation of sequential MQ processing

### TEST-11: Plugin System Health
- **Category:** Plugin System
- **Priority:** P2
- **Steps:**
  1. List all plugins: `gt plugin list`
  2. For each plugin, check last run: `gt plugin history <name>`
  3. Verify all cooldown-based plugins ran within their intervals
  4. Check dolt-snapshots plugin: event-gated on convoy.created — verify it hasn't run (correct if no convoys created)
  5. Test `gt plugin status` (known bug gt-8c1: command doesn't exist)
  6. Verify plugin cooldown logic: trigger a plugin, check it respects cooldown on re-trigger
- **Expected Result:** All plugins healthy, running within intervals. `gt plugin status` missing (known).
- **Cleanup:** None
- **Parallel:** Yes — read-only checks

### TEST-12: Convoy System Operations
- **Category:** Convoy System
- **Priority:** P2
- **Steps:**
  1. List convoys: `gt convoy list`
  2. Show details of existing convoy (e.g., hq-cv-iosm2): verify status, tracked issues, progress
  3. Create test convoy tracking a set of test beads
  4. Verify convoy appears in list
  5. Close tracked beads, verify convoy progress updates
  6. Verify convoy closes when all tracked beads complete
  7. Test convoy close/reopen dedup (recent fix: 205fe1d5)
- **Expected Result:** Convoy tracking accurate, progress updates work, close dedup fix holds.
- **Cleanup:** Close/delete test convoy and test beads
- **Parallel:** No — sequential state changes

### TEST-13: Git Worktree State Consistency
- **Category:** Architecture / State Consistency
- **Priority:** P2
- **Research Finding:** State consistency between Dolt, git worktrees, and tmux sessions is a weak point
- **Steps:**
  1. List all worktrees: `git worktree list` (from rig root)
  2. Cross-reference with tmux sessions: `tmux list-sessions`
  3. Cross-reference with polecat list: `gt polecat list`
  4. Verify 1:1:1 mapping: each active polecat has exactly one worktree and one tmux session
  5. Check for orphaned worktrees (worktree exists, no tmux session)
  6. Check for orphaned sessions (tmux exists, no worktree)
  7. Verify worktree branches match expected naming: `polecat/<name>/<bead>@<hash>`
- **Expected Result:** Perfect 1:1:1 mapping. No orphans.
- **Cleanup:** Remove any orphaned worktrees/sessions found
- **Parallel:** Yes — read-only checks

### TEST-14: Slice Panic Prevention on Abbreviated SHAs
- **Category:** Regression / Data Integrity
- **Priority:** P2
- **Recent Fix:** 5eedf1b2 — "fix: prevent slice panics on abbreviated SHA hashes"
- **Steps:**
  1. Find code paths that handle SHA hashes (git operations, branch naming)
  2. Test with abbreviated SHA (< 7 chars): verify no panic
  3. Test with empty SHA string: verify no panic
  4. Test with full 40-char SHA: verify normal operation
  5. Test with non-hex characters in SHA position: verify clean error
- **Expected Result:** No panics on any SHA input. Clean errors for invalid input.
- **Cleanup:** None
- **Parallel:** Yes — isolated unit test

### TEST-15: Dolt Recovery Procedures
- **Category:** Dolt / Operational
- **Priority:** P2
- **Steps:**
  1. Verify `gt dolt recover` command exists and shows help
  2. Check Dolt commit history: `gt dolt log --db gt -n 10`
  3. Verify Dolt auto-commit policy is set correctly for each context
  4. Test `gt dolt backup` dry-run (if available)
  5. Verify backup state file exists and is current
  6. Test read query after simulated load: `gt dolt sql -q "SELECT count(*) FROM issues" --db gt`
- **Expected Result:** Recovery tools available, backup state current, queries work under normal load.
- **Cleanup:** None
- **Parallel:** Yes — read-only

### TEST-16: Runtime Files Not Tracked by Git
- **Category:** Hygiene
- **Priority:** P3
- **Known Bug:** gt-zam — runtime/backup files tracked in .beads/
- **Steps:**
  1. Check `.beads/` directory for files that should be gitignored
  2. Verify: `git ls-files .beads/backup/` — should return nothing
  3. If files are tracked: `git rm --cached .beads/backup/*` to untrack
  4. Verify `.gitignore` has appropriate patterns for `.beads/backup/`
  5. Run `bd doctor` — check for "tracked runtime files" warnings
- **Expected Result:** No runtime files tracked by git. Doctor reports no warnings for tracked files.
- **Cleanup:** Untrack files if found
- **Parallel:** Yes

### TEST-17: bd close Idempotency Warning
- **Category:** Beads / Edge Case
- **Priority:** P3
- **Known Bug:** gt-8m1
- **Steps:**
  1. Create and close a test bead
  2. Attempt to close it again: `bd close <id> --reason "double close"`
  3. Observe output — currently succeeds silently
  4. Verify no data corruption from double close
  5. Verify close_reason is not overwritten by second close
- **Expected Result:** Second close should warn (not currently implemented). No data corruption.
- **Cleanup:** None
- **Parallel:** Yes

### TEST-18: gt dolt cleanup Orphan Detection
- **Category:** Dolt / Cleanup
- **Priority:** P3
- **Known Bug:** gt-8c1
- **Steps:**
  1. Run `gt dolt list` — note which databases show as orphan (expected: `te`)
  2. Run `gt dolt cleanup --dry-run` — verify it reports the orphans
  3. If it doesn't detect `te` (known bug), document the discrepancy
  4. Verify `te` database has schema but 0 issues (expected state)
- **Expected Result:** Cleanup should detect orphans but currently doesn't (known bug).
- **Cleanup:** None (dry-run only)
- **Parallel:** Yes

### TEST-19: Orphaned Dependency References
- **Category:** Beads / Data Integrity
- **Priority:** P2
- **Known Bug:** gt-ivf
- **Steps:**
  1. Run `bd doctor` and capture orphaned dependency output
  2. List known orphans: gt-71w→gt-wisp-6avv, gt-iim→gt-wisp-u8nq, gt-ji4→gt-wisp-w73l
  3. For each orphan, verify the target doesn't exist: `bd show <target-id>`
  4. Run `bd doctor --fix` and verify orphans are cleaned
  5. Re-run `bd doctor` to confirm no orphans remain
  6. Verify no side effects on dependent beads after fix
- **Expected Result:** `bd doctor --fix` removes all orphaned deps cleanly.
- **Cleanup:** Orphans removed by fix
- **Parallel:** No — modifies database state

### TEST-20: Sling to Docked Rig Prevention
- **Category:** Polecat Lifecycle / Safety
- **Priority:** P3
- **Steps:**
  1. Dock a rig (if not already docked): `gt rig dock <rig-name>`
  2. Attempt to sling a bead to the docked rig
  3. Verify the sling is blocked with a clear error message
  4. Undock the rig: `gt rig undock <rig-name>`
  5. Verify sling works after undocking
- **Expected Result:** Sling to docked rig is blocked. Undocking re-enables slinging.
- **Cleanup:** Restore rig to original dock state
- **Parallel:** No — modifies rig state

### TEST-21: Handoff and Session Continuity
- **Category:** Polecat Lifecycle
- **Priority:** P2
- **Steps:**
  1. Sling a bead to a polecat
  2. Wait for polecat to start working
  3. Trigger handoff: polecat runs `gt handoff -s "Test" -m "Testing handoff"`
  4. Verify new session starts with the same hook
  5. Verify hook bead is preserved
  6. Verify molecule/formula attachment is preserved
  7. Verify the new session can read handoff notes
- **Expected Result:** Seamless handoff — hook, molecule, and context preserved.
- **Cleanup:** Let polecat complete normally
- **Parallel:** No — single lifecycle

### TEST-22: Escalation Flow
- **Category:** Communication / Recovery
- **Priority:** P2
- **Steps:**
  1. From a polecat, run `gt escalate "Test escalation" -s HIGH -m "Testing escalation flow"`
  2. Verify witness receives the escalation
  3. Verify escalation bead is created with correct severity
  4. Check witness response behavior
  5. Verify polecat can continue working after escalation
- **Expected Result:** Escalation delivered to witness, bead created, polecat continues.
- **Cleanup:** Close escalation bead
- **Parallel:** Yes — independent of other tests

---

## Execution Order (Recommended)

Tests are grouped into phases for efficient execution:

### Phase 1: Data Integrity (run first — these catch corruption)
1. TEST-03: Dolt Database Integrity Check
2. TEST-19: Orphaned Dependency References
3. TEST-08: Beads State Transitions

### Phase 2: Known Active Bugs (highest value — validate fixes or confirm bugs)
4. TEST-01: SetAgentState FK Violation
5. TEST-02: Polecat Auto-Nuke
6. TEST-14: Slice Panic Prevention

### Phase 3: Core Lifecycle (end-to-end flows)
7. TEST-05: Polecat `gt done` Flow
8. TEST-06: Witness Detection and Recovery
9. TEST-10: Refinery Merge Queue Processing
10. TEST-04: Concurrent Polecat Spawn Race

### Phase 4: Supporting Systems (can run in parallel)
11. TEST-07: Mail System *(parallel)*
12. TEST-09: Prefix Routing *(parallel)*
13. TEST-11: Plugin System Health *(parallel)*
14. TEST-13: Git Worktree Consistency *(parallel)*
15. TEST-15: Dolt Recovery Procedures *(parallel)*
16. TEST-22: Escalation Flow *(parallel)*

### Phase 5: Edge Cases and Known Bugs
17. TEST-12: Convoy System
18. TEST-16: Runtime Files in Git
19. TEST-17: bd close Idempotency
20. TEST-18: gt dolt cleanup Orphan Detection
21. TEST-20: Sling to Docked Rig
22. TEST-21: Handoff Continuity

---

## Parallelism Summary

| Can Run in Parallel | Must Be Sequential |
|---------------------|--------------------|
| TEST-03, TEST-08, TEST-07, TEST-09, TEST-11, TEST-13, TEST-14, TEST-15, TEST-16, TEST-17, TEST-18, TEST-22 | TEST-01, TEST-02, TEST-04, TEST-05, TEST-06, TEST-10, TEST-12, TEST-19, TEST-20, TEST-21 |

---

## Gap Analysis vs Original Shakedown (gt-iim)

### Tests Added (not in original plan)
- **TEST-01**: FK violation — the #1 known active bug, wasn't directly tested
- **TEST-04**: Concurrent spawn — identified as high-risk by research, was SKIP in original
- **TEST-14**: SHA panic regression — recent fix needs validation
- **TEST-19**: Orphaned deps — found by doctor but not tested for fix
- **TEST-21**: Handoff continuity — critical for session survival
- **TEST-22**: Escalation flow — never tested end-to-end

### Tests Deprioritized (stable in original)
- Cross-rig `bd show` from different directories — already PASS, low risk
- Plugin individual history checks — already PASS, folded into TEST-11
- Basic mail send/read — already PASS, kept but lowered priority

### Tests That Cannot Be Safely Run in Production
- Dolt corruption injection (could break live databases)
- Killing witness process (could orphan active polecats)
- Force-killing Dolt mid-write (could corrupt WAL)
- These should be tested in a staging environment only
