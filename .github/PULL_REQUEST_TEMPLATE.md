## Summary

<!-- What changes and why. For behavior changes, describe the before and after. -->

## Related issues

<!-- `Closes #` / `Refs #`. Larger changes should start with an issue (see CONTRIBUTING.md). -->

## Breaking changes and compatibility

- [ ] No breaking change, or the change to configuration, report output or behavior is described above
- [ ] Configuration change regenerates `config.hcl2spec.go` with `make generate` and updates [docs/configuration.md](https://github.com/larskroghdk/packer-plugin-windows-features/blob/main/docs/configuration.md) and an example (I10), or N/A
- [ ] Incompatible report JSON change increments `schema_version` (I10), or N/A
- [ ] Release classification or support claims changed only with a validation record in [docs/validation.md](https://github.com/larskroghdk/packer-plugin-windows-features/blob/main/docs/validation.md) (I11), or N/A

Affected Windows releases:

## Testing

- [ ] `make check` passes
- [ ] `make generate-check` passes for configuration changes
- [ ] Regression tests fail on the old behavior and pass on the new one
- [ ] The fake guest in `provisioner/guest_test.go`, Go `json` tags and PowerShell property names stay in step (I10)
- [ ] Guest-script tests pass under Windows PowerShell 5.1
- [ ] Live guest run: release, edition, build, installation type, builder and attached report, or "Not run" with the reason

Mocked tests and cross-compilation don't validate servicing, restarts or WinRM recovery on a real guest (I11).

## Servicing behavior

- [ ] Installation and removal end in the requested state in the final inventory, not only a successful cmdlet exit code (I5)
- [ ] A second run with the same configuration reports every request as `compliant` and changes nothing
- [ ] Dependencies are left to the servicing cmdlets' own switches, and observed side effects are reported (I3), or N/A
- [ ] Restarts happen only with `restart_if_required = true`, and the requests are verified again after the restart (I8), or N/A
- [ ] Failures keep per-request states, and `fail_on_error = false` doesn't hide preflight, inventory, cancellation, restart or connectivity failures (I6), or N/A
- [ ] Server Manager, optional-feature and capability handling stay separate (I1), or N/A

## Session safety and security

- [ ] Removals of protected components stay fatal, including transitive ones, and missing WinRM facts still block removals (I4), or N/A
- [ ] The change doesn't open a second WinRM connection or change the builder's connection settings (I7)
- [ ] Feature names and options pass through the JSON contract, not interpolated into PowerShell, or N/A
- [ ] No credentials or secrets in code, reports, logs or artifacts
- [ ] Actions remain pinned to commit SHAs with minimum permissions, or N/A

Security and performance impact:

## Documentation

- [ ] README, `docs/`, examples and AGENTS.md updated where behavior changed, or N/A
- [ ] Go doc comments and PowerShell help follow I19
- [ ] User-visible changes have a commit subject that the release notes pick up (`docs:` and `test:` commits are excluded), or N/A
