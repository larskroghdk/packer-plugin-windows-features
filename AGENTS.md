# Agent instructions

`windows-features` is a Go Packer provisioner that installs, removes and verifies Windows Server roles and features, Windows optional features and Windows capabilities over WinRM without breaking the Packer session. Hard constraints are in [INVARIANTS.md](INVARIANTS.md) and are referenced as `I<n>`. Package ownership and the source-project analysis are in [docs/architecture.md](docs/architecture.md).

## Module map

| Path | Role |
| --- | --- |
| `main.go` | Plugin registration and version |
| `provisioner/` | Configuration, run orchestration, communicator calls (`remote.go`), restart (`restart.go`) |
| `internal/plan/` | Desired state, dependency closure, protection rules and verification |
| `internal/facts/` | Guest facts, release classification (`release.go`), WinRM comparisons |
| `internal/report/` | Versioned JSON report and console summary |
| `guest/scripts/` | Embedded Windows PowerShell 5.1: detection and supported servicing cmdlets |
| `cmd/doccheck/` | Lint for the Go documentation rules of I19 |
| `test/pester/` | Mocked guest contracts that make no servicing changes |
| `test/integration/` | Opt-in live tests (`integration` build tag, `WF_TEST_*` variables) |

## Run flow

Upload → detect → classify by product type, build, installation type and available cmdlets → inventory → resolve names, dependencies, protection and conflicts → stop on preflight failure or dry run → removals, then installations → new inventory → verify requested states and record side effects → WinRM probe → optional restart and re-verification → report and cleanup. Each guest action is one `powershell.exe -File Invoke-WindowsFeatures.ps1 -Action <name>` call that exchanges JSON files in a private `C:\Windows\Temp\packer-windows-features-<id>` directory.

## Go

- Keep planning and verification in `internal/plan`, OS and connectivity interpretation in `internal/facts`, and communicator I/O in `provisioner` (I7). Use the SDK's default component registration, so the HCL name stays `windows-features`.
- Pass the supplied communicator through restart handling. Don't open a separate WinRM connection.
- Pass the caller's context and bound every wait. An operation timeout can't promise that Windows stopped servicing. `fail_on_error` never softens cancellation or connectivity failures (I6).
- Change `Config`, the generated HCL2 spec, [docs/configuration.md](docs/configuration.md) and an example together. Run `make generate` with the SDK's pinned `packer-sdc` (I10).
- Document every function per I19, including unexported and test functions.
- Don't add optimization profiles, registry tuning or rollback journals. Servicing dependencies and payload availability aren't reversible configuration values.

## PowerShell

- The guest runs elevated, 64-bit Windows PowerShell 5.1, including Server Core. Analysis under host `pwsh` doesn't establish guest compatibility.
- Guest scripts detect state and call the supported servicing cmdlets. They make no policy decisions. Keep ServerManager, optional-feature and capability handling separate (I1).
- Keep incidental output off the success pipeline. Keep Go `json` tags, PowerShell property names and the fake guest in `provisioner/guest_test.go` in step.
- Keep `.ps1` files ASCII with CRLF. A Go test enforces this.

## Security

- Missing WinRM facts block removals, and known session-critical removals are fatal, including transitive ones (I4). Don't weaken a rule to make an example pass.
- Pass feature names and options through the JSON contract, never by interpolating them into PowerShell command text.
- Keep credentials out of source, reports and logs. Keep Actions pinned to commit SHAs with minimum permissions.

## Checks

Run `make check` before committing (I18). If `make` is missing (Git Bash on Windows), run its recipe lines directly. It checks module tidiness, gofmt, vet, pinned staticcheck and govulncheck, doccheck, psdoccheck, PSScriptAnalyzer, `packer fmt`, Go tests and Pester under `powershell.exe` 5.1. Run `make generate-check` for configuration changes and `make release-snapshot` for packaging changes. Tool versions are in [docs/testing.md](docs/testing.md).

Prefer a regression test that fails on the old behavior. Mocked tests and cross-compilation don't validate a Windows release (I11). Live tests need an authorized disposable guest, so report results you actually observed and say what wasn't run.

## Skills

Task procedures live in `.agents/skills/<name>/SKILL.md`. Read only the one that matches the task:

- [change-servicing](.agents/skills/change-servicing/SKILL.md): Go orchestration or the PowerShell servicing contract, with regression tests.
- [review-connectivity](.agents/skills/review-connectivity/SKILL.md): protection rules and WinRM checks.
- [support-windows-release](.agents/skills/support-windows-release/SKILL.md): release classification and validation records.
- [release-plugin](.agents/skills/release-plugin/SKILL.md): CI, packaging and authorized signed releases.

Activating a skill grants no authority to publish or to operate on a guest. [docs/agent-guidance.md](docs/agent-guidance.md) explains how each tool finds these files.
