# Supported systems

| Guest target | Feature mechanism | Capabilities |
| --- | --- | --- |
| Windows Server 2016 (14393) | ServerManager | When cmdlets exist |
| Windows Server 2019 (17763) | ServerManager | When cmdlets exist |
| Windows Server 2022 (20348) | ServerManager | When cmdlets exist |
| Windows Server 2025 (26100) | ServerManager | When cmdlets exist |
| Windows 10, build 14393+ | DISM optional features | When cmdlets exist |
| Windows 11, build 22000+ | DISM optional features | When cmdlets exist |

Server Core and Desktop Experience are distinguished. Product type identifies servers and domain controllers independently of localized product names. Build numbers distinguish Windows 10 and 11 even when the registry product name says Windows 10. Newer unlisted builds are accepted with an untested-release warning. Older unlisted builds above the minimum are also identified by build with a warning; the release table is not an exhaustive lifecycle list.

Nano Server, pre-14393 Windows, non-10 major versions, missing required servicing interfaces, 32-bit guest PowerShell processes and unelevated sessions are rejected. Domain-controller detection does not make role removal on a domain controller safe.

Feature availability varies with edition and hardware. Hyper-V requires a supported edition and virtualization extensions; a VM needs nested virtualization exposed by its host. Windows Home editions cannot install every feature available in Pro/Enterprise. Payloads and Features on Demand must match the guest version, architecture and language.

## Validation status

Go unit and fake-communicator tests cover Server/client selection, request planning, installation/removal, idempotency, source errors, partial failures, timeouts, pending restart, reconnect checks and verification. Mocked PowerShell tests cover servicing command contracts under Windows PowerShell 5.1.

No live Server 2016/2019/2022/2025 or Windows 10/11 servicing run has been recorded for this initial implementation. Live OS validation remains required before promoting a release for production. Record edition, build, architecture, builder, servicing operation and test output for every claimed platform. Host cross-compilation and Packer schema validation do not validate guest servicing.

See the [local validation record](validation.md) for executed checks and remaining limitations.
