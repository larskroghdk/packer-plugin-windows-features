# 🔧 Troubleshooting

| Symptom | Action |
| --- | --- |
| Feature is unavailable | Query the target with `Get-WindowsFeature` (Server), `Get-WindowsOptionalFeature -Online` (client), or `Get-WindowsCapability -Online`; use the exact namespace/name. Check edition and release. |
| `0x800F081F` / source missing | Supply matching installation media or FoD content through a guest-visible `source`; confirm architecture, language, build and access rights. |
| `0x800F0906` / download failure | Check source access, update policy and networking; use matching local media when appropriate. |
| `0x800F0954` / WSUS policy | Check servicing source policy with the administrator; do not weaken update policy to work around the error. |
| Elevation or 64-bit check fails | Configure the builder to use an administrative WinRM account and a 64-bit endpoint/PowerShell process. |
| Protected or conflicting request | Remove the contradiction or keep the required runtime/network component. `fail_on_error` cannot bypass protection. |
| Operation timeout | Servicing may still run. Inspect `%WINDIR%\Logs\CBS\CBS.log` and `%WINDIR%\Logs\DISM\dism.log`, pending restart state and source health. Avoid concurrent servicing. |
| Restart timeout | Use the VM console to check boot and servicing; verify networking, WinRM startup, listeners and builder credentials. Increase the timeout only if servicing is progressing. |
| `pending_reboot` | Enable managed restart or add an appropriate Packer restart stage and validate with a subsequent provisioner run. |
| `packer init` cannot find a release | Local source builds require `packer plugins install --path`; remote installation requires published, correctly named release artifacts and checksums. |

Use `PACKER_LOG=1` for SDK/communicator diagnostics. Review and redact logs and reports before sharing: they contain host identity, feature names, paths and possibly Windows error details. Never include passwords, WinRM secrets or private signing keys in issues.

The host report is attempted even on a guest error. A non-WinRM communicator is rejected before a guest run and report creation. Report-write failure fails an otherwise successful run. Temporary guest files are cleaned up on a best-effort bounded attempt; an unreachable guest can retain its unique working directory.
