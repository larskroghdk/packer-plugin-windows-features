# 🪟 Packer Plugin Windows Features

Manage Windows Server roles and features, Windows client optional features, and Windows capabilities during Packer builds. The provisioner reads the guest inventory, plans changes, skips compliant requests, applies supported servicing cmdlets, and verifies the resulting state. It writes a JSON report on successful and failed runs.

Releases include binaries for Windows, Linux and macOS on amd64 and arm64, so Packer can run on any of them. Feature management applies only to Windows guests reached through WinRM with an elevated, 64-bit Windows PowerShell 5.1 session.

## 📦 Install

Install the signed release with:

```hcl
packer {
  required_plugins {
    windows-features = {
      source  = "github.com/larskroghdk/windows-features"
      version = ">= 0.1.0"
    }
  }
}
```

```powershell
packer init .
packer plugins installed
```

For local development before a release exists:

```powershell
go build -trimpath -ldflags='-X main.versionPrerelease=' -o dist/packer-plugin-windows-features.exe .
packer plugins install --path ./dist/packer-plugin-windows-features.exe github.com/larskroghdk/windows-features
```

Local builds advertise `0.1.0`, matching the examples' `>= 0.1.0` constraint. The SDK registers the default provisioner, so the HCL name is `windows-features`, without a repeated component suffix.

## 🚀 Use

Add this block to a build using your Windows builder and WinRM settings:

```hcl
provisioner "windows-features" {
  install = ["Web-Server", "Web-Mgmt-Tools", "NET-Framework-Features"]
  remove  = ["Print-Server"]

  include_management_tools = true
  restart_if_required      = true
  report_path              = "reports/server-features.json"
}
```

On Windows 10 or 11, use optional feature names instead:

```hcl
provisioner "windows-features" {
  install             = ["NetFx3"]
  source              = "D:\\sources\\sxs"
  limit_access        = true
  restart_if_required = true
}
```

Capabilities use their own lists, for example `install_capabilities = ["OpenSSH.Server"]`. The short capability identity resolves only if the guest inventory contains exactly one matching version. Feature lists require exact names, ignoring case; wildcards are rejected.

## 🛡️ Compatibility and safety

The implementation targets Server 2016, 2019, 2022 and 2025 (Core and Desktop Experience), Windows 10 build 14393 onward, and Windows 11. Availability depends on edition, servicing support, payload source and hardware. These are implementation targets, not a claim of successful live testing on each release. See [support and validation status](docs/support.md).

Removals of PowerShell and its .NET runtime are blocked, including known transitive Server Manager dependencies. The plugin checks a new WinRM command and configuration after changes and restarts. This is a conservative guard, not a complete model of every role's networking effects. Hyper-V, routing, domain and remote-access changes need a disposable image and builder-specific network review. See [connectivity and restarts](docs/safety.md).

`dry_run = true` inventories and validates without servicing or restarting. `restart_if_required` defaults to false. A pending state is reported explicitly; it does not mean the feature is usable before reboot.

## 📚 Documentation

- [Configuration reference](docs/configuration.md)
- [Architecture and source-project analysis](docs/architecture.md)
- [Supported systems and servicing interfaces](docs/support.md)
- [Restarts and connectivity](docs/safety.md)
- [Examples](examples/README.md)
- [Troubleshooting](docs/troubleshooting.md)
- [Testing](docs/testing.md) and [releasing](docs/releasing.md)
- [Contributing](CONTRIBUTING.md)
- [Security policy](.github/SECURITY.md)

Built with the [HashiCorp Packer Plugin SDK](https://developer.hashicorp.com/packer/docs/plugins/creation). Licensed under [MPL 2.0](LICENSE).
