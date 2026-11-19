---
name: change-servicing
description: Change this plugin's Go orchestration or PowerShell servicing contract, including options, inventory, operations and regression tests.
---

Read `AGENTS.md`, `INVARIANTS.md`, and the affected modules. Trace one request from `Config` through `operationRequest`, guest dispatch, `OperationResult`, inventory verification and report. Keep ServerManager, optional-feature and capability behavior separate.

For a configuration change, regenerate HCL with the SDK version pinned in `go.mod`, then update `docs/configuration.md` and a relevant example. Do not manually edit generated schema. For a guest change, test the real cmdlet parameters through Pester mocks under Windows PowerShell 5.1 and keep the fake guest synchronized.

Prefer a regression test that fails on the old behavior: contradictory installed dependencies, dependent side effects, timeout/cancellation, successful command with wrong inventory, or a state lost after restart. Run the checks in `docs/testing.md`. Preserve ASCII/CRLF when writing scripts.
