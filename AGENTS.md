# Repository guidance

This independent Packer plugin manages Windows servicing over WinRM. Read [INVARIANTS.md](INVARIANTS.md) for constraints and [docs/architecture.md](docs/architecture.md) for package ownership and source-project analysis.

## Work here

- Keep planning and verification in `internal/plan`, OS and connectivity interpretation in `internal/facts`, and communicator I/O in `provisioner`. Guest scripts detect and invoke supported cmdlets; they do not decide policy.
- Use the SDK's default component registration. The user-facing name is `windows-features`. Pass the supplied communicator through restart handling; do not open an independent connection.
- Change `Config`, generated HCL2 spec, reference and examples together. Generate with the pinned SDK's `packer-sdc`; verify repeatability.
- Use contexts and bounded waits. An operation timeout cannot promise Windows stopped servicing. Never soften cancellation or connectivity failure with `fail_on_error`.
- Do not add optimization profiles, registry tuning or rollback journals. Windows servicing dependencies and payload availability are not reversible configuration values.

## Checks

Run `go test ./...`, `go vet ./...`, `staticcheck ./...`, `go run ./cmd/doccheck .`, HCL formatting and PSScriptAnalyzer. Run mocked Pester tests with `powershell.exe` 5.1. Commands and tool versions are in [docs/testing.md](docs/testing.md); `make check` combines them when GNU Make is available.

Guest scripts must remain ASCII with CRLF. Do not claim an OS validated from a fake guest or cross-compile. Live tests use build tag `integration` and explicit `WF_TEST_*` configuration; only enable changes on authorized disposable targets. No tags, pushes, remote repository creation or release publication without explicit authorization.

## Focused skills

- [Change servicing](.agents/skills/change-servicing/SKILL.md): Go, PowerShell and Packer contract changes with regression tests.
- [Review connectivity](.agents/skills/review-connectivity/SKILL.md): protection and WinRM regression work.
- [Support a Windows release](.agents/skills/support-windows-release/SKILL.md): facts, build tables and validation evidence.
- [Release the plugin](.agents/skills/release-plugin/SKILL.md): CI, packaging, versioning and documentation checks.

Keep authoritative rules here and in the linked invariants. `CLAUDE.md` and Copilot instructions route here; do not copy divergent standards into them.
