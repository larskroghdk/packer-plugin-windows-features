# 🏗️ Architecture

## 🔎 Foundation reviewed

The sibling `packer-plugin-windows-optimize` supplied the architectural starting point: SDK `plugin.NewSet`, default provisioner registration, generated HCL2 schema, `Prepare` validation, JSON files exchanged over the Packer communicator, embedded PowerShell parts, Go-side planning, a fake guest implementing `packer.Communicator`, report generation, and reboot probes through the existing communicator.

This project reuses those patterns but has no dependency on the sibling repository. It replaces optimization catalogs, profiles, registry changes and rollback journals with Windows servicing inventories and desired feature states. Feature removal is not reliably reversible without payload media, so no automatic rollback is claimed.

Releases are packaged for Windows, Linux and macOS on amd64 and arm64, so the plugin runs wherever Packer runs; only the guest must be Windows. CI separates read-only checks from signed release publication. The configuration/schema, guest scripts and Go module are owned by this repository.

## 🧱 Components

| Path | Responsibility |
| --- | --- |
| `main.go` | Version and SDK default provisioner registration. |
| `provisioner/config.go` | Strict decoding, defaults, name and policy validation. |
| `provisioner/config.hcl2spec.go` | SDK-generated HCL2 contract. |
| `provisioner/provisioner.go` | Inventory, planning, operations, verification and reporting. |
| `provisioner/remote.go` | Upload, execute and download via the supplied communicator. |
| `provisioner/restart.go` | Delayed shutdown and bounded boot-time/WinRM polling. |
| `internal/facts` | Structured facts, OS classification and WinRM comparisons. |
| `internal/plan` | Pure desired-state, dependency, protection and verification logic. |
| `internal/report` | Versioned JSON and readable console summary. |
| `guest/scripts` | PowerShell 5.1 detection and supported servicing cmdlets. |
| `test/pester` | Mocked guest contracts with no servicing changes. |
| `test/integration` | Explicitly configured real-guest Packer tests. |

## 🔄 Run flow

Upload → Detect → classify by product type/build/installation and available cmdlets → Inventory → resolve names/dependencies/protection/conflicts → stop on preflight failure or dry run → restart first if servicing is blocked and enabled → removals then installations → new inventory → verify requested states and record side effects → new WinRM probe → optional restart and re-verification → report and cleanup.

Each request gets a stable identity and each guest action exchanges a separate UTF-8 JSON file under a unique `C:\Windows\Temp\packer-windows-features-*` directory. No feature names are interpolated into PowerShell commands. The guest reports structured objects; it does not parse human-readable DISM output. Go determines desired-state outcomes.

Required dependencies are delegated to Windows servicing. Server Manager exposes a dependency graph for preflight; optional features and capabilities do not expose an equivalent complete graph through their inventory cmdlets. A successful command never replaces inventory verification.

See the [official SDK architecture](https://developer.hashicorp.com/packer/docs/plugins/creation) and [plugin loading specification](https://developer.hashicorp.com/packer/docs/plugins/creation/plugin-load-spec).
