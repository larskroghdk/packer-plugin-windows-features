# Invariants

Hard constraints for implementation and review. Stable IDs follow the source project's documentation-check convention where applicable.

- **I1 — Separate namespaces.** Server features, client optional features and capabilities use the matching supported servicing cmdlets. Never silently reinterpret a capability as a feature.
- **I2 — Inspect before changing.** Resolve exact names against inventory, reject unknown states, skip compliant requests, and detect contradictions before any operation.
- **I3 — Delegate dependencies to Windows.** Use supported servicing switches; inspect known Server Manager dependencies for conflicts/protection and report observed side effects. Do not invent a generic dependency installer.
- **I4 — Preserve communication.** Protected removals are fatal, including known transitive effects. Missing baseline WinRM facts block removals. Risky networking/authentication installations must be flagged.
- **I5 — Verify after changing.** A successful command is only evidence. Re-read inventory for final state, check requests initially compliant, and re-verify after restart. Never call an unattempted/planned apply request successful.
- **I6 — Report partial failures honestly.** Keep per-request states and restart requirements. `fail_on_error=false` never suppresses configuration, preflight, inventory, cancellation, restart or connectivity failures.
- **I7 — Use the supplied communicator.** The SDK owns connection lifecycle. No independent WinRM client or builder changes. Check a fresh command and WinRM configuration after changes/restarts.
- **I8 — Restart only with permission in configuration.** Servicing scripts suppress automatic restarts. `restart_if_required` gates orchestrated restarts; waits and cleanup are bounded.
- **I9 — Target supported Windows sessions.** Require WinRM, elevation, 64-bit PowerShell and a supported guest classification. Build-host OS does not imply guest OS support.
- **I10 — Preserve versioned contracts.** Regenerate HCL2 for configuration changes. Increment report schema version for incompatible JSON changes. Keep Go and PowerShell JSON contracts and fake-guest fixtures aligned.
- **I11 — Record evidence.** Mocked tests and cross-compilation are not live Windows validation. Keep destructive/infrastructure tests separate and opt-in. Never publish releases or push tags without authorization.
- **I18 — Keep checks reproducible.** Go formatting, vet, staticcheck, documentation checks, HCL formatting, PSScriptAnalyzer and Windows PowerShell Pester tests must pass. Guest `.ps1` files remain ASCII with CRLF, enforced by Go tests.
- **I19 — Comments document contracts.** Follow the inherited source standard: each Go function/method, including tests, has a name-leading doc comment with author `Lars Krogh`, semantic version and a `Returns:` section for error or multiple returns. Exported types and non-test packages have documentation. Only doc comments and functional `//go:` directives are allowed; generated files are exempt. PowerShell uses comment-based help and `#Requires`; reusable functions describe parameters, outputs and examples with author/version metadata. Put rationale and operational guidance in Markdown. `cmd/doccheck` and `psdoccheck.sh` enforce the mechanical subset.
