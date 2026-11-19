---
name: review-connectivity
description: Review or change feature protection and WinRM checks when servicing could affect networking, authentication, PowerShell or the active Packer connection.
---

Read `docs/safety.md`, `internal/plan/protect.go`, inventory closures and `internal/facts/connectivity.go`. Identify direct and transitive effects, including installed dependencies and initially compliant requests. Separate what inventory proves from effects it cannot model.

Use static protection for known session-critical removals; missing facts must not authorize a removal. Add focused tests for direct protection, dependent closure, preflight before any operation and unsoftened connectivity failure. A warning alone is insufficient for a known session-breaking removal. Do not weaken a rule to make an example pass.

Inspect the real supported cmdlet contract using official Microsoft documentation. For lab verification use an authorized disposable VM with out-of-band access; do not run destructive tests on the host. Record guest build, builder and network topology rather than claiming universal safety from a successful mock.
