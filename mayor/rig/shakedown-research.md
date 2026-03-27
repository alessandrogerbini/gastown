# Gas Town Shakedown Research: Known Difficulties, Failure Modes, and Best Practices

> Compiled by polecat quartz (gt-c05) on 2026-03-27
> Input context for follow-up test planning agent (gt-hnt)

---

## Table of Contents

1. [Critical Data Corruption & Consistency Bugs](#1-critical-data-corruption--consistency-bugs)
2. [Race Conditions & Concurrency Issues](#2-race-conditions--concurrency-issues)
3. [Panic & Crash Bugs](#3-panic--crash-bugs)
4. [Process & Session Management Failures](#4-process--session-management-failures)
5. [Deadlock Bugs](#5-deadlock-bugs)
6. [Dolt-Specific Issues](#6-dolt-specific-issues)
7. [Environment & Configuration Bugs](#7-environment--configuration-bugs)
8. [Cleanup & Recovery Mechanisms](#8-cleanup--recovery-mechanisms)
9. [Active Beads (Open Bugs)](#9-active-beads-open-bugs)
10. [Codebase Warnings & Known Limitations](#10-codebase-warnings--known-limitations)
11. [Architectural Weak Points](#11-architectural-weak-points)
12. [Validated Patterns & Best Practices](#12-validated-patterns--best-practices)
13. [Testing Priority Matrix](#13-testing-priority-matrix)

---

## 1. Critical Data Corruption & Consistency Bugs

### 1.1 FK Violation on Polecat Spawn (ACTIVE - gt-4ao)

- **Severity:** CRITICAL (P1, 100% repro rate)
- **Commits:** `de48410f` (fix), `gt-4ao` (still active), `gt-33m` (fix task open)
- **Bead:** gt-4ao (in_progress, assigned to obsidian), gt-ji4 (closed, original report)
- **Impact:** Every polecat spawn hits FK constraint failure (`Error 1452: fk_counter_parent`), retries 7-10 times with exponential backoff (~60s total delay). Polecat starts but with significant latency and log spam.
- **Root Cause:** `SetAgentState` inserts counter row referencing parent issue that doesn't exist in `issues` table.
- **Fix Applied:** Detect FK violations, ensure parent exists via `Show + CreateAgentBead` fallback, retry set-state. But issue persists per gt-4ao.
- **Fix Task:** gt-33m proposes two approaches: (a) `gt sling` creates agent state issue before session, or (b) `SetAgentState` upserts parent row if missing.
- **Blocks:** gt-71w (lifecycle smoke test), gt-9of (CI integration test)

### 1.2 Orphaned Wisps Unreapable

- **Severity:** HIGH
- **Commit:** `5b9aafc3`
- **Impact:** Wisps stuck in database permanently, unreachable by reaper
- **Root Causes:** (1) `batchDeleteRows` only cleaned `issue_id` references, not `depends_on_id`; (2) `parentCheckWhere` didn't handle dangling parent references
- **Fix:** Clean reverse dependency references; add LEFT JOIN + IS NULL branch for missing parents

### 1.3 Orphaned Dolt Processes After Shutdown

- **Severity:** HIGH
- **Commit:** `a3e902c9`
- **Impact:** Rogue Dolt servers serve incomplete databases after `gt down`
- **Root Cause:** Only stopped canonical server, leaving idle-monitors and orphan processes that respawn rogues
- **Fix:** 4-phase shutdown: stop idle-monitors -> stop canonical -> stop orphans -> remove `.beads/dolt` directories

---

## 2. Race Conditions & Concurrency Issues

### 2.1 Dolt Startup Timing Race + lsof Dependency

- **Severity:** HIGH
- **Commit:** `66e1adb5`
- **Impact:** Startup failure on slow storage (NFS/CSI) and systems without `lsof`
- **Root Causes:** (1) `IsRunning()` removed valid PID file when process alive but not listening; (2) `findDoltServerOnPort()` required unavailable `lsof`
- **Fix:** Replace `IsRunning()` with `cmd.Process.Signal(0)`; add fallbacks (`lsof -> ss -> /proc/net/tcp`)

### 2.2 Dolt Restart Race with Idle-Monitor

- **Severity:** HIGH
- **Commit:** `496d69a8`
- **Impact:** Rogue Dolt processes spawned during restart
- **Root Cause:** Idle-monitors respawned servers between shutdown and restart
- **Fix:** Stop idle-monitor processes before server restart; replace fixed sleep with port-release polling

### 2.3 TOCTOU Race in FindRigBeadsDir

- **Severity:** MEDIUM
- **Code:** `internal/doltserver/doltserver.go:3071`
- **Impact:** Returned directory may change between Stat check and caller's operation
- **Mitigation:** Use `FindOrCreateRigBeadsDir` for write operations (documented)

### 2.4 Scheduler/Convoy Timing Race

- **Severity:** MEDIUM
- **Commit:** `d66e2837`
- **Impact:** Flaky tests (`bd "database out of sync"` error)
- **Root Cause:** `bd` checks Dolt import timestamp >= `issues.jsonl` mtime (1-second granularity); create + import straddle second boundary
- **Fix:** Add 1.1s sleep after bead creation to settle timestamps

### 2.5 Double-Spawn Bug (Issue #1752)

- **Severity:** MEDIUM
- **Code:** `internal/daemon/daemon.go:2071-2078`
- **Impact:** Multiple Claude processes spawned for same polecat
- **Root Cause:** Time-bound skip (5 minutes) — if `gt sling` crashes during spawn, polecat stuck in 'spawning' indefinitely
- **Mitigation:** Spawning guard with time-bound expiry; regression tests at `internal/daemon/polecat_health_test.go:73, 109`

### 2.6 Stuck Agent Dog Duplicate Wisps

- **Severity:** MEDIUM
- **Commit:** `4f523f1b`
- **Impact:** 84+ duplicate wisps per witness patrol cycle
- **Root Cause:** Plugin only filtered `agent_state=spawning`, missed done/nuked states
- **Fix:** Add filters for done and nuked polecats

---

## 3. Panic & Crash Bugs

### 3.1 Slice Panics on Abbreviated SHA Hashes

- **Severity:** HIGH
- **Commit:** `5eedf1b2`
- **Impact:** Runtime panics in audit, dolt_rebase, orphans, compactor_dog, refinery
- **Root Cause:** Array bounds checking missing on abbreviated hash slicing
- **Fix:** `SafeAbbrevHash` utility with proper bounds checks + tests

### 3.2 Nil Pointer Panics (Multiple)

- **Severity:** HIGH
- **Commits:** `3db786a4`, `25535b8e`, `9f33b97d`, `29643452`, `0eb6e414`
- **Patterns:**
  - Nil Execution context
  - Nil cobra.Command in sling
  - Nil cmd in mol telemetry
  - Dolt nil pointer dereference (SEGV) — upstream dolthub/dolt#10539 (still open)
- **Fix:** Guard against nil before dereferencing; fork workaround for Dolt upstream

### 3.3 Merge Slot Status Nil Panic

- **Severity:** MEDIUM
- **Commit:** `2aa1e082` (part of 5-bug fix)
- **Impact:** Refinery/engineer crash
- **Fix:** Add nil check for merge slot status

### 3.4 Slice Index Out of Bounds

- **Severity:** MEDIUM
- **Commit:** `61ef6819` (Issue #1228)
- **Fix:** Bounds checks for all string indexing panic points

---

## 4. Process & Session Management Failures

### 4.1 The Idle Polecat Heresy

- **Severity:** CRITICAL (system-level)
- **Impact:** Cascading delays in entire work pipeline
- **Root Cause:** Polecat completes work but sits idle waiting for approval
- **Prevention:** Mandatory `gt done` protocol, no approval step exists
- **Detection:** Witness patrol detects `agent_state=idle` with `hook_bead` still set

### 4.2 Spawn Storms

- **Severity:** CRITICAL
- **Impact:** 6-7 polecats assigned same bead, all duplicate effort
- **Trigger:** Polecat exits without closing bead (no `gt done` AND no `bd close`)
- **Chain:** Witness zombie patrol resets bead to "open" -> Deacon dispatches to new polecat -> repeat
- **Prevention:** Every session must end with either `gt done` OR explicit `bd close`

### 4.3 Zombie Polecats

- **Severity:** HIGH
- **States:**
  1. Dead session + running state + stale activity
  2. Session stuck in "done" intent (`gt done` failed mid-execution)
  3. Dead session + dirty git state (data loss risk)
  4. Bead closed but session alive
- **Detection:** Deacon zombie-scan, Witness `DetectZombiePolecats()`, pattern matching (dead session + running state = ZOMBIE)
- **Recovery:** Deacon files death warrant -> Boot executes -> Witness pre-kill verification -> SAFE_TO_NUKE vs NEEDS_RECOVERY

### 4.4 Hung Dogs Consume Memory

- **Severity:** HIGH
- **Commit:** `15d5d5eb`
- **Impact:** ~500MB RAM per stuck dog
- **Root Cause:** Dogs finish work without calling `gt dog done`, sit idle forever
- **Fix:** Auto-clear in health checker; lower `maxInactivity` from 30m to 10m

### 4.5 Orphaned Hooked Beads After Crash

- **Severity:** MEDIUM
- **Commit:** `066f2a8a`
- **Impact:** Beads stuck in hooked/in_progress state, lost work
- **Fix:** `gt up` scans for beads assigned to dead polecats, resets to open for re-dispatch

### 4.6 Default Prefix Ghost Sessions

- **Severity:** MEDIUM
- **Commit:** Bug fix in `daemon.go:1399-1407` (hq-ouz, hq-eqf, hq-3i4)
- **Impact:** Stale duplicate tmux sessions when daemon starts before rig registered
- **Fix:** `killDefaultPrefixGhosts()` removes sessions matching rigs with own prefix

### 4.7 Orphan Processes from Manager

- **Severity:** MEDIUM
- **Commit:** `2aa1e082`
- **Root Cause:** `mayor/manager` didn't use `KillSessionWithProcesses`
- **Fix:** Use `KillSessionWithProcesses` to prevent orphan processes

---

## 5. Deadlock Bugs

### 5.1 Windows Pipe Deadlock in Tests

- **Severity:** HIGH
- **Commit:** `bb797cd0`
- **Impact:** Test timeout (8m48s hang, 10m suite timeout)
- **Root Cause:** Manual `os.Pipe` stdout capture deadlocked on Windows (~4KB buffer vs 64KB on Linux)
- **Fix:** Concurrent `captureStdout` helper, replaced 17 manual pipe blocks

### 5.2 TestRestartWithBackoff Deadlock

- **Severity:** HIGH
- **Commit:** `5c2863e2`
- **Root Cause:** `time.Sleep(5ms)` race relied on precise OS scheduling; goroutine held lock through 500ms sleep
- **Fix:** Test hook infrastructure (channels instead of sleeps for determinism)

### 5.3 Agent Config Resolution Deadlock

- **Severity:** MEDIUM
- **Commit:** `9e49ccd2`
- **Fix:** Move agent config resolution mutex to package config level

---

## 6. Dolt-Specific Issues

### 6.1 DOLT_REBASE Safety Warning

- **Severity:** CRITICAL (documented risk)
- **Code:** `internal/daemon/compactor_dog.go:136, 184, 314, 494, 498`
- **Issue:** Surgical mode uses `DOLT_REBASE` which is NOT safe with concurrent writes. System retries on graph-change errors, but this is a known unsafe operation.
- **Mitigation:** Row count verification after rebase (cannot detect all corruption cases)

### 6.2 Dolt Nil Pointer Dereference (SEGV)

- **Severity:** HIGH
- **Commit:** `0eb6e414`
- **Upstream:** dolthub/dolt#10539 (still open)
- **Workaround:** Replace directive to zfogg/dolt fork with nil pointer checks

### 6.3 Signal-Induced Corruption

- **Severity:** HIGH
- **Code:** `internal/daemon/dolt.go:964`
- **Issue:** SIGKILL mid-journal-write causes corruption requiring `dolt fsck` to recover
- **Mitigation:** 30s grace period to flush journal under load

### 6.4 Stale MySQL/Dolt Socket Cleanup

- **Severity:** MEDIUM
- **Commits:** `2e058fa1`, `775dbc79`, `0e6d7c91`
- **Impact:** Dolt restart failure from stale Unix socket, connection failures
- **Fix:** Remove socket if no process holds it open; detect stale `sql-server.info` files

### 6.5 Write Contention at Scale

- **Severity:** MEDIUM (ongoing pressure)
- **Impact:** Every `bd create`, `bd update`, `gt mail send` = 1 permanent Dolt commit
- **Scale:** 20 polecats = concurrent writes, slowdown after hours
- **Mitigation:** Use `gt nudge` (zero cost) over `gt mail send`; don't create unnecessary beads

### 6.6 Orphaned Database Cleanup Gap

- **Severity:** LOW
- **Bead:** gt-8c1
- **Issue:** `gt dolt list` shows `te (orphan)` but `gt dolt cleanup --dry-run` says "No orphaned databases found"

---

## 7. Environment & Configuration Bugs

### 7.1 BD_BRANCH Permanent Unset (REVERTED)

- **Severity:** HIGH
- **Commit:** `8c768e3c` (revert of #1797/#1815)
- **Problem:** `os.Unsetenv` permanent for process; breaks write isolation; copy-pasted in 3 places
- **Lesson:** Root cause was in flush/branch-creation sequence, not callers

### 7.2 Cross-Rig BD_BRANCH Contamination

- **Severity:** HIGH
- **Commit:** `4562513d`
- **Impact:** Cross-rig contamination
- **Fix:** Strict cross-rig guard in sling + revert broken BD_BRANCH bypass

### 7.3 BEADS_DB/BEADS_DOLT_SERVER_DATABASE Missing

- **Severity:** MEDIUM
- **Commits:** `c01869d6`, `ba639cd1`
- **Impact:** Subprocess helpers spawn wrong database connections
- **Fix:** Pass env vars to subprocess; strip inherited BEADS_DB in subprocess helpers

---

## 8. Cleanup & Recovery Mechanisms

### 8.1 Infrastructure Cleanup Layers

| Layer | Scope | Commands | Severity |
|-------|-------|----------|----------|
| L0 | Ephemeral data | `gt compact`, `gt krc prune` | Safe |
| L1 | Processes | `gt cleanup`, `gt orphans procs kill` | Low |
| L2 | Git artifacts | `gt prune-branches`, `gt polecat gc` | Medium |
| L3 | Agents/sessions | `gt polecat nuke`, `gt done`, `gt shutdown` | High |
| L4 | Workspace | `gt rig reset`, `gt doctor --fix` | High |
| L5 | System | `gt uninstall`, `gt disable --clean` | Critical |

### 8.2 Pre-Kill Verification Protocol

1. `gt polecat check-recovery` -> SAFE_TO_NUKE or NEEDS_RECOVERY
2. `gt polecat git-state` -> must be clean
3. `bd show` -> should show closed
4. If NEEDS_RECOVERY -> escalate to Mayor, do NOT force nuke

### 8.3 Spike Baseline Export Blocking Fix

- **Commit:** `1efc1ecd`
- **Problem:** Permanent export blocking after spike halt (HEAD never updates on halt)
- **Fix:** Spike baseline file; if next run sees stable count, accept new level and proceed

### 8.4 Stale Socket/PID File Cleanup

- **Commits:** `2e058fa1` (MySQL socket), `775dbc79` (Dolt socket), `0e6d7c91` (sql-server.info), `5c2863e2` (PID file)
- **Pattern:** Check if process holds file open before cleanup; treat stale files as successful recovery, not errors

---

## 9. Active Beads (Open Bugs)

| Bead | P | Status | Title | Impact |
|------|---|--------|-------|--------|
| gt-4ao | P1 | in_progress | SetAgentState FK violation on every polecat spawn | 100% repro, ~60s delay per spawn |
| gt-33m | P1 | open | Ensure parent issue row exists before SetAgentState | Fix task for FK violation |
| gt-ivf | P2 | open | bd doctor reports orphaned dependency references | 3 orphans, confuses dependency resolution |
| gt-71w | P2 | open | Polecat lifecycle smoke test | Blocked by gt-33m |
| gt-9of | P2 | open | Add polecat spawn integration test to CI | Blocked by gt-33m |
| gt-8c1 | P3 | open | gt dolt cleanup doesn't detect orphaned 'te' database | Detection/cleanup inconsistency |
| gt-8m1 | P3 | open | bd close on already-closed bead succeeds silently | Idempotency gap |
| gt-zam | P3 | open | Runtime/backup files tracked by git in .beads/ | Git hygiene |

---

## 10. Codebase Warnings & Known Limitations

### 10.1 Active Code Warnings

| Location | Issue | Severity |
|----------|-------|----------|
| `compactor_dog.go:136` | DOLT_REBASE not safe with concurrent writes | HIGH |
| `dolt.go:964` | SIGKILL mid-journal-write causes corruption | HIGH |
| `daemon.go:2071` | Double-spawn guard has 5-min time-bound gap | MEDIUM |
| `doltserver.go:3071` | TOCTOU race in FindRigBeadsDir | MEDIUM |
| `convoy/operations.go:200` | Convoy checks may be stale for cross-rig issues (GH #2624) | MEDIUM |
| `rig/manager.go:944` | merge_queue.target_branch deprecated | LOW |

### 10.2 Integration Test Limitations

- **Code:** `internal/cmd/install_integration_test.go:542-547`
- Doctor fix has bugs with bead creation (UNIQUE constraint errors)
- Container environment lacks tmux for session checks
- Test repos don't satisfy priming expectations (AGENTS.md length)

### 10.3 Deferred Architectural TODOs

| Location | TODO | Priority |
|----------|------|----------|
| `synthesis.go:367` | Notification system (parse "Notify: <address>") | LOW |
| `molecule_step.go:365` | Parallel molecule step execution | MEDIUM |
| `sling.go:652` | Single-sling dispatch unification (scheduler-unify) | LOW |
| `rig/manager.go:953` | Beads config YAML -> JSON migration | LOW |
| `daemon.go:1563` | Consolidate duplicate parked/docked checking (Issue #2120) | LOW |

---

## 11. Architectural Weak Points

### 11.1 Single Points of Failure

- **Witness as bottleneck:** Single-threaded patrol cycle; at high throughput becomes bottleneck
- **Dolt write contention:** All agents share Dolt, every mutation is a permanent commit
- **Tmux session dependency:** Session management coupled to tmux; crashes lose context

### 11.2 State Consistency Gaps

- **Dolt vs Git vs Tmux:** Three sources of truth (bead state, git branches, tmux sessions) can diverge
- **Cross-rig dependency resolution:** Time-based staleness checks can fail with multiple rigs (GH #2624)
- **BD_BRANCH environment propagation:** Subprocess env inheritance can cause cross-rig contamination

### 11.3 Cleanup/Hygiene Gaps

- **Orphaned worktrees:** Dead polecats may leave worktrees unreferenced
- **Orphaned Dolt databases:** Detection exists but cleanup logic misses some cases (gt-8c1)
- **Orphaned dependency references:** 3 confirmed orphans in current state (gt-ivf)
- **Runtime files in git:** 7 files should be gitignored (gt-zam)

### 11.4 Major Feature Reverts (Design Flaw Indicators)

| Commit | Feature | Lines Reverted | Reason |
|--------|---------|---------------|--------|
| `220ddfc0` | Deacon Pending Command | 632 | Feature didn't work as intended |
| `d3a84d58` | Deferred Polecat Dispatch | 6,073 | Design flaw in capacity scheduler |
| `88574f28` | Symlink Testutil Refactor | 405 | Architectural issue |
| `cf3b3a67` | Issues.jsonl Removal | 933 | Backend incompatibility |

These reverts indicate areas where the design was under-validated before implementation.

---

## 12. Validated Patterns & Best Practices

### 12.1 Patterns That Work

| Pattern | Evidence | Status |
|---------|----------|--------|
| Propulsion Principle (hook -> execute) | Documented, enforced in CLAUDE.md | Validated |
| Pre-kill verification before nuke | Recovery protocol prevents data loss | Validated |
| `gt nudge` over `gt mail send` | Zero Dolt cost vs permanent commit | Validated |
| Molecule formula workflow | Step-by-step execution with handoff | Validated |
| Deacon zombie-scan + death warrants | Detects dead sessions reliably | Validated |
| 4-phase Dolt shutdown | Prevents orphan process respawning | Validated (commit a3e902c9) |
| `SafeAbbrevHash` utility | Prevents 5+ panic sites | Validated (commit 5eedf1b2) |
| Circular dependency detection | Passing in shakedown (gt-iim) | Validated |
| Prefix routing | Passing in shakedown (gt-iim) | Validated |
| Plugin cooldowns | Passing in shakedown (gt-iim) | Validated |
| Convoy system | Passing in shakedown (gt-iim) | Validated |
| Mail send/receive | Passing in shakedown (gt-iim) | Validated |

### 12.2 Operational Procedures That Prevent Issues

1. **Completion protocol:** lint/format/test -> stage -> commit -> `gt done`
2. **Context survival:** Persist findings to bead as you work (`bd update --notes`)
3. **Work discovery:** File as new bead, don't fix yourself
4. **Help-seeking:** Mail Witness after 15min stuck
5. **Directory discipline:** Stay in your worktree, use absolute paths
6. **Swim lane rule:** Only close wisps YOU created

### 12.3 Monitoring That Catches Problems Reliably

- Witness patrol cycle: zombies, orphans, stalled workers, pending spawns
- Deacon zombie-scan: dead session + running state detection
- `bd doctor`: orphaned references, runtime file tracking, dependency health
- `gt dolt list`: orphan database detection (though cleanup has gaps)
- Health checker auto-clear: hung dogs with `--auto-clear`

---

## 13. Testing Priority Matrix

Based on findings, recommended shakedown test priorities:

### Tier 1: Critical (Must Test)

| Test Scenario | Risk | Evidence |
|---------------|------|----------|
| FK violation on polecat spawn | Data integrity | gt-4ao: 100% repro, P1 |
| Spawn storm prevention | Resource waste | Documented in CLAUDE.md, 6-7 polecat duplication |
| Zombie detection & recovery | Session leak | 4 distinct zombie states documented |
| `gt done` completion path | Pipeline stall | Idle Polecat Heresy = critical failure mode |
| Dolt shutdown (4-phase) | Orphan processes | commit a3e902c9, high severity |
| Slice/nil panic prevention | Crash | 7 panic commits, 5+ nil pointer fixes |

### Tier 2: High (Should Test)

| Test Scenario | Risk | Evidence |
|---------------|------|----------|
| Dolt startup race conditions | Startup failure | commit 66e1adb5 |
| Idle-monitor restart race | Rogue processes | commit 496d69a8 |
| Cross-rig BD_BRANCH isolation | Data contamination | commit 4562513d |
| Stale socket/PID cleanup | Restart failure | 4 separate fix commits |
| Pre-kill verification | Data loss prevention | Recovery protocol documented |
| Hung dog memory consumption | Resource exhaustion | commit 15d5d5eb, ~500MB/dog |

### Tier 3: Medium (Good to Test)

| Test Scenario | Risk | Evidence |
|---------------|------|----------|
| DOLT_REBASE concurrent write safety | Data corruption | compactor_dog.go warning |
| TOCTOU race in FindRigBeadsDir | State inconsistency | doltserver.go:3071 |
| Double-spawn guard timing | Duplicate work | daemon.go:2071, Issue #1752 |
| Convoy check staleness | Cross-rig issues | GH #2624 |
| Orphaned dependency cleanup | Data hygiene | gt-ivf: 3 orphans |
| bd close idempotency | Operator confusion | gt-8m1 |

### Tier 4: Low (Nice to Have)

| Test Scenario | Risk | Evidence |
|---------------|------|----------|
| Orphan database cleanup consistency | Minor hygiene | gt-8c1 |
| Runtime file gitignore | Git hygiene | gt-zam |
| Notification system | Feature gap | synthesis.go:367 TODO |
| Config migration (YAML -> JSON) | Technical debt | rig/manager.go:953 |

---

## Appendix A: Key Commit References

| Commit | Category | Description |
|--------|----------|-------------|
| `de48410f` | Data Integrity | FK violation fix with retry/fallback |
| `5b9aafc3` | Data Integrity | Orphaned wisps unreapable fix |
| `a3e902c9` | Process Mgmt | 4-phase Dolt shutdown |
| `66e1adb5` | Race Condition | Dolt startup timing + lsof fallback |
| `496d69a8` | Race Condition | Idle-monitor restart race |
| `5eedf1b2` | Panic Fix | SafeAbbrevHash for slice panics |
| `bb797cd0` | Deadlock | Windows pipe deadlock in tests |
| `5c2863e2` | Deadlock | TestRestartWithBackoff fix |
| `15d5d5eb` | Resource Leak | Hung dogs memory fix |
| `4f523f1b` | Duplicate Work | Stuck agent duplicate wisps |
| `066f2a8a` | Orphan Recovery | Hooked beads after crash |
| `2aa1e082` | Multi-fix | 5 code-review bugs (nil, orphan, path, error, throttle) |
| `4562513d` | Env Safety | Cross-rig BD_BRANCH guard |
| `8c768e3c` | Revert | BD_BRANCH permanent unset (lesson: fix root cause) |
| `d3a84d58` | Major Revert | Deferred polecat dispatch (6,073 lines) |

## Appendix B: Shakedown (gt-iim) Results Summary

The previous comprehensive shakedown ran 64 checks and found 7 bugs, all filed as beads:

| Finding | Severity | Bead |
|---------|----------|------|
| bd close silent on already-closed bead | P3 | gt-8m1 |
| 3 orphaned dependency references | P2 | gt-ivf |
| 7 runtime files tracked by git | P3 | gt-zam |
| Cannot inspect another agent's mail inbox | P3 | (noted) |
| gt dolt cleanup misses te orphan DB | P3 | gt-8c1 |
| gt plugin status command missing | P4 | (noted) |
| bd lint flags wisps for missing criteria | P4 | (noted) |

**Passing areas:** circular dependency detection, prefix routing, plugin cooldowns, convoy system, mail send/receive, Dolt connectivity, witness zombie cleanup.
