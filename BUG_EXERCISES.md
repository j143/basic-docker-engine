# Bug-Fix Exercises (Debugging Practice)

This document lists real bug scenarios in this repository as hands-on exercises.

## How to use this exercise set

- Reproduce the issue first.
- Write a failing test (or script) that proves the bug.
- Fix with the smallest safe change.
- Re-run `go test ./...` and `go build ./...`.

---

## Exercise 1 — Runtime panic from short container IDs

- **Category:** Runtime panic / edge case
- **Area:** Monitoring
- **Location:** `/home/runner/work/basic-docker-engine/basic-docker-engine/monitor.go` (`ContainerMonitor.GetMetrics`)
- **Symptom:** Program can panic with `slice bounds out of range` when container ID length is less than 8.
- **Repro hint:** Run monitoring code against a short container ID (for example `"abc"`).
- **What to debug:** String slicing assumptions.
- **Done when:** Monitoring no longer panics for short IDs and returns metrics safely.

---

## Exercise 2 — Nil pointer risk from deferred close before error check

- **Category:** File I/O correctness
- **Area:** Layer metadata persistence
- **Location:** `/home/runner/work/basic-docker-engine/basic-docker-engine/main.go` (`saveLayerMetadata`)
- **Symptom:** `defer file.Close()` is scheduled before confirming `os.Create(...)` succeeded.
- **Repro hint:** Force create failure (permissions/path issue) and observe behavior.
- **What to debug:** Ordering of error handling and cleanup.
- **Done when:** File handles are only closed when file creation succeeds.

---

## Exercise 3 — Wrong PID tracked for a running container

- **Category:** State inconsistency / observability bug
- **Area:** Container lifecycle
- **Location:** `/home/runner/work/basic-docker-engine/basic-docker-engine/main.go` (`runWithoutNamespaces`)
- **Symptom:** Stored PID reflects the CLI process instead of the spawned container command process.
- **Repro hint:** Run a container and compare metadata PID with actual child process PID.
- **What to debug:** Process lifecycle timing (`cmd.Start`/`cmd.Run`) and which PID should be recorded.
- **Done when:** Metadata PID maps to the actual container command process.

---

## Exercise 4 — `ps`/`exec` status flow relies on PID file that is never written

- **Category:** Feature integration bug
- **Area:** CLI status and exec path
- **Location:** 
  - Reader: `/home/runner/work/basic-docker-engine/basic-docker-engine/main.go` (`getContainerStatus`, `execCommand`)
  - Writer path gap: `/home/runner/work/basic-docker-engine/basic-docker-engine/main.go` (`runWithoutNamespaces`)
- **Symptom:** Status/exec behavior can be incorrect because PID file expectations are not met.
- **Repro hint:** Run a container, inspect files under `/tmp/basic-docker/containers/<id>/`, then try `ps`/`exec`.
- **What to debug:** Contract mismatch between producer and consumer of runtime state.
- **Done when:** PID file lifecycle is consistent and dependent commands behave correctly.

---

## Exercise 5 — Deferred close in loop can leak descriptors under many layers

- **Category:** Resource leak / scalability bug
- **Area:** Image pulling
- **Location:** `/home/runner/work/basic-docker-engine/basic-docker-engine/image.go` (`Pull`)
- **Symptom:** Layer readers are closed with `defer` inside loop, so descriptors stay open until function returns.
- **Repro hint:** Pull image with many layers and watch file descriptor count/pressure.
- **What to debug:** Resource lifetime in loops.
- **Done when:** Each layer reader is closed promptly after extraction.

---

## Exercise 6 — Hardcoded image directory bypasses shared runtime configuration

- **Category:** Configuration consistency bug
- **Area:** Image management
- **Location:**
  - `/home/runner/work/basic-docker-engine/basic-docker-engine/image.go` (`ListImages`, `Pull`, `LoadImageFromTar`)
  - `/home/runner/work/basic-docker-engine/basic-docker-engine/main.go` (`listImages`)
- **Symptom:** Multiple code paths hardcode `/tmp/basic-docker/images` instead of consistently using shared directory variables.
- **Repro hint:** Change runtime base directory and observe path inconsistencies.
- **What to debug:** Single source of truth for storage paths.
- **Done when:** Image paths are consistently derived from shared config.

---

## Exercise 7 — Potential path traversal / unsafe extraction behavior

- **Category:** Security bug
- **Area:** Image extraction
- **Location:** `/home/runner/work/basic-docker-engine/basic-docker-engine/image.go` (`extractLayer`, `LoadImageFromTar`)
- **Symptom:** Tar extraction trusts archive entries and may write outside intended rootfs depending on archive content/tool behavior.
- **Repro hint:** Create a crafted tar with traversal-like paths and evaluate extraction safety.
- **What to debug:** Archive extraction safety boundaries.
- **Done when:** Extraction guarantees writes stay within target rootfs.

---

## Exercise 8 — Error handling is partially ignored in state transitions

- **Category:** Reliability / silent failure
- **Area:** Container state persistence
- **Location:** `/home/runner/work/basic-docker-engine/basic-docker-engine/main.go` (`runWithoutNamespaces`)
- **Symptom:** Calls to `UpdateContainerState(...)` are not checked for errors.
- **Repro hint:** Make state file path unwritable and run container lifecycle.
- **What to debug:** Error propagation in critical state updates.
- **Done when:** Failures to persist state are surfaced and handled predictably.

---

## Exercise 9 — Network state management uses global mutable state

- **Category:** Concurrency and maintainability risk
- **Area:** Network management
- **Location:** `/home/runner/work/basic-docker-engine/basic-docker-engine/network.go`
- **Symptom:** Global `networks` slice is read/written without synchronization.
- **Repro hint:** Simulate concurrent network operations (or parallel tests) and look for races.
- **What to debug:** Shared state ownership and synchronization strategy.
- **Done when:** Concurrent operations are safe and deterministic.

---

## Suggested debugging workflow (work-like)

1. Reproduce and capture exact failure output.
2. Add a failing test that isolates one bug.
3. Fix one bug at a time (avoid broad refactors).
4. Re-run:
   - `go test ./...`
   - `go build ./...`
5. Add a short note in your commit message: *root cause + prevention idea*.
