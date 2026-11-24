# Security policy

## Supported versions

Only the latest release receives security fixes. Fixes ship in a new release, not as backports. Because the project is pre-1.0, a release with a fix can also contain breaking changes.

| Version | Supported |
| --- | --- |
| Latest release | Yes |
| Older releases | No |
| Unreleased `main` | Fixed in the next release |

## Reporting a vulnerability

Don't open a public issue, pull request or discussion.

Use [private vulnerability reporting](https://github.com/larskroghdk/packer-plugin-windows-features/security/advisories/new): the "Report a vulnerability" button on the repository's Security tab.

Include:

- Plugin version, Packer version, builder and WinRM settings (HTTP or HTTPS, authentication type).
- Guest edition, version, build and installation type (Server Core or Desktop Experience).
- The `windows-features` provisioner block, the affected feature, role or capability names, and the payload `source` type.
- Steps to reproduce, the impact, and what an attacker needs (network position, guest account, access to the build host).
- The relevant parts of the JSON report or `PACKER_LOG=1` output.

Remove passwords, tokens, signing material, host names, IP addresses, internal share paths and domain names first. The report records the WinRM user name in `system.winrm_user`. Don't test against systems you don't own or aren't authorized to test.

## Scope

In scope:

- Credentials or signing material exposed in reports, Packer logs, files uploaded to the guest, or release artifacts.
- Feature names, options or other request data reaching PowerShell command text instead of the JSON files of the guest contract.
- Weak protection of the guest working directory `C:\Windows\Temp\packer-windows-features-<id>`, for example a non-administrator being able to replace the scripts or JSON files that the elevated session runs.
- Removal of a protected component, directly or as a known transitive Server Manager dependency, or a removal that proceeds without the baseline WinRM facts. The protected set is listed under "Protection boundary" in [Connectivity and restarts](../docs/safety.md).
- A report that shows a request as `installed`, `removed` or `compliant` when the final inventory doesn't, for example a removal of `FS-SMB1` or `SMB1Protocol` reported as removed while the feature is still enabled.
- A restart without `restart_if_required = true`, or `fail_on_error = false` suppressing a preflight, inventory, cancellation, restart or connectivity failure.
- Weaknesses in the CI or release workflows, such as unpinned Actions or excessive token permissions, that could compromise published binaries.

Out of scope:

- Vulnerabilities in Windows, Windows servicing (Server Manager, DISM, CBS), WinRM or Windows PowerShell. Report these to the [Microsoft Security Response Center](https://msrc.microsoft.com/report).
- Vulnerabilities in Packer or the Packer plugin SDK. Report these through [HashiCorp's security policy](https://www.hashicorp.com/security).
- The security effect of a feature that the configuration asks to install or remove. The plugin applies the requested state; see [Security considerations](#security-considerations).
- Limits documented in [Connectivity and restarts](../docs/safety.md), such as networking effects of roles the protection rules don't model. Report an unexpected result as a bug.

## Process

The maintainer acknowledges the report, confirms or rejects it, and agrees a disclosure date with the reporter. This is a single-maintainer project, so response times are best effort.

Fixes are released in a new version and disclosed through a GitHub Security Advisory. A CVE is requested through GitHub when appropriate. Reporters are credited unless they prefer not to be.

## Security considerations

These follow from how the plugin works. They aren't vulnerabilities, but they affect the images you build.

- **Elevated execution.** Each guest action runs `powershell.exe -File` in an elevated, 64-bit Windows PowerShell 5.1 session through Packer's WinRM communicator. The build account is an administrator of the guest. Use dedicated build credentials, pass them as sensitive Packer variables or environment variables, and build on disposable guests.
- **WinRM transport.** The plugin uses the builder's communicator and doesn't change the WinRM configuration. Use HTTPS (`winrm_use_ssl = true`) with certificate validation outside isolated build networks; `winrm_insecure = true` disables that validation.
- **Payload sources.** `source` is resolved by Windows servicing on the guest, with the build account's rights. Use trusted, version-matched media on a read-only location. Set `limit_access = true` to keep DISM from falling back to Windows Update.
- **Installing adds attack surface.** Roles and features can add services, listeners and firewall rules; examples are `Web-Server`, `FS-SMB1`, `Telnet-Client`, Hyper-V and Remote Access. Windows servicing can change role-specific firewall rules; the plugin doesn't. Install only what the image needs and review the firewall afterwards.
- **Removing can remove security controls.** Protection covers components that the Packer session depends on, not security tooling. A removal such as `Windows-Defender` on Server proceeds if requested. `remove_payload = true` also deletes the payload from the component store, so reinstalling needs a `source`.
- **Pending restarts.** A request in `pending_reboot` isn't in effect yet. A removed feature can stay active until the restart. Check `reboot.required` and `validation.passed` in the report before promoting an image.
- **Reports.** Reports contain the guest's edition, build, feature states and WinRM user name. Treat them as build artifacts, write them outside shared directories, and redact them before attaching them to an issue.

## Verifying releases

Download `packer-plugin-windows-features_v<VERSION>_SHA256SUMS` and its detached `.sig` from the GitHub release. Verify the signature with GPG using the maintainer's release signing key, then check the plugin ZIP against the checksums.
