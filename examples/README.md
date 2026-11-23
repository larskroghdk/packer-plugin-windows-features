# 🧩 Windows examples

Each directory is a separate template for one VM purpose. IIS and Hyper-V are never selected in the same template. These examples use an existing disposable Windows VM through Packer's built-in null builder; copy the relevant provisioner block into your actual image builder.

| Directory | Target and purpose |
| --- | --- |
| `server-iis` | Windows Server web server with IIS and explicitly validated management tools. |
| `server-netfx` | Windows Server .NET Framework 3.5 core from matching installation media. |
| `server-hyperv` | Dedicated Windows Server Hyper-V host with management tools. |
| `client-netfx` | Windows 10/11 .NET Framework 3.5 from matching media. |
| `client-hyperv` | Dedicated Windows client Hyper-V environment on a supported Pro/Enterprise/Education edition. |
| `client-optional` | Windows 10/11 optional-feature example using TelnetClient, where available. |
| `capability-openssh` | OpenSSH.Server capability, kept separate from optional features. Does not configure/start sshd or open firewall ports. |

Run only the directory appropriate to the intended VM. Supply connection secrets privately through environment variables or an untracked variable file:

```powershell
$env:PKR_VAR_host = '192.0.2.10'
$env:PKR_VAR_password = '<supply privately>'
packer init examples/server-iis
packer validate examples/server-iis
packer build examples/server-iis
```

Every template uses `version = ">= 0.1.0"` and defaults to dry run. After reviewing the plan on an authorized disposable target, `packer build -var dry_run=false examples/server-iis` performs servicing and permits required restarts.

The .NET templates require a payload source matching the guest's release, architecture and language:

```powershell
packer build -var 'payload_source=D:\sources\sxs' examples/server-netfx
```

TLS certificate verification is enabled by default. Use a valid certificate; `insecure=true` is an explicit opt-in for a disposable self-signed test endpoint. `use_ssl=false` selects HTTP.

Hyper-V requires supported hardware and edition, and virtualized targets require nested virtualization exposed by the host. Review network effects and maintain console access before applying either Hyper-V template. Windows Home does not support the client Hyper-V role.

For removal use a separate provisioner request such as `remove = ["Print-Server"]` on Server or `remove = ["TelnetClient"]` on client. Known networking/authentication and runtime removals are protected. Hyper-V examples demonstrate installation only.

Template validation does not prove live servicing on every OS. See [support status](../docs/support.md) and [live testing](../docs/testing.md).
