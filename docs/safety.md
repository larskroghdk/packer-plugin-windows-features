# Connectivity and restarts

The provisioner uses Packer's supplied `packer.Communicator` throughout. It does not create a second WinRM client, replace builder connection settings or attempt to manage Packer's process lifecycle. Servicing calls suppress automatic restart; the Go orchestrator issues a delayed `shutdown.exe` restart only when `restart_if_required` is true.

After a restart it retries commands through the same communicator, re-uploads scripts if needed, waits for a changed boot time to remain stable across 30 seconds, checks WinRM and elevation, and reads inventories again. This follows the command/retry approach of Packer's [windows-restart provisioner](https://github.com/hashicorp/packer/blob/main/provisioner/windows-restart/provisioner.go). Each wait is bounded by `restart_timeout`; at most three completion restarts are attempted, in addition to a pre-servicing restart when necessary.

The plugin detects CBS, Windows Update, pending file rename and computer rename indicators. A restart indicator is not proof a requested feature is usable. Unknown restart detection remains visible as a warning. An already pending servicing restart is completed first when enabled; otherwise the plugin warns that servicing may fail.

## Protection boundary

PowerShell, PowerShellRoot, .NET Framework 4.x and WoW64 on Server, and NetFx4-AdvSrvs on client are protected from removal. Server DNS, DHCP, RemoteAccess, Routing, DirectAccess-VPN, AD-Domain-Services and Hyper-V, and client Hyper-V core features are also protected because they may carry networking or authentication for the current session. Known installed dependent features are included in this check. Missing baseline WinRM facts block removals. After changes, a fresh command verifies the service, enabled listeners, authentication and elevation against baseline. Connectivity failures always fail the run, even with `fail_on_error = false`.

These checks do not fully model every role's firewall, routing, DNS, virtual switch, domain or third-party extension effects. No firewall rules are intentionally modified by the plugin itself; Windows servicing can change role-specific rules. Review Hyper-V, Remote Access, routing and infrastructure roles with the builder's network configuration before use. Do not remove infrastructure the active connection depends on. Use disposable guests with snapshots and out-of-band access for live validation.

## Operation failures

An operation timeout stops subsequent operations; Windows servicing may still run after its PowerShell command is cancelled. Do not immediately start another servicing tool. Inspect CBS/DISM logs and pending restart state first. A partial failure is reported per request and has no automatic rollback.

Server Manager may remove child/dependent role services as part of an explicit removal. The report lists observed side effects. DISM does not provide the same preflight dependency graph. Explicitly list important desired features so the final inventory can catch unexpected state changes.
