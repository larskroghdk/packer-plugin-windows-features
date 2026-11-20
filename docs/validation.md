# 📋 Local validation — 2026-11-19

Executed on a Windows amd64 development host with Go 1.26.10, Packer 1.16.2, GoReleaser 2.20.0, staticcheck 2025.1.1, PSScriptAnalyzer 1.22.0 and Pester 6.2.0 under Windows PowerShell 5.1.

| Check | Result |
| --- | --- |
| `go test ./...` | Passed, including entry-point description, configuration, facts, planning, report and fake-communicator tests. |
| `go vet ./...` and staticcheck | Passed. |
| Go formatting and documentation checks | Passed. |
| Generated HCL2 schema repeatability | Regeneration produced the same SHA256. |
| `go mod tidy -diff` | No changes; `github.com/zclconf/go-cty v1.19.0` is direct. |
| Windows PowerShell Pester | 17 tests passed, no failures. Servicing commands and fact reads were mocked. |
| PSScriptAnalyzer and PowerShell documentation check | Passed. |
| HCL formatting | Passed for integration and example templates. |
| Native Packer install and discovery | Passed with an isolated plugin directory and version `1.0.0`. |
| `packer init` and `packer validate` | Passed for the integration template and all seven independent examples, each with `>= 1.0.0`. |
| GoReleaser Windows snapshot | Windows amd64 and arm64 executables and ZIP archives built; expected executable names and both archive SHA256 hashes verified. |
| Repository skills | Four skills passed the skill-creator validator. |

Snapshot mode used `0.0.0-dev` because this repository had no commits/tags. Stable local installation checks used a normal `1.0.0` build. Automatic Go toolchain network verification stalled during validation; the final checks used the already installed Go 1.26.10 toolchain and cached dependencies.

## 🏗️ Checks still requiring infrastructure

- The live integration suite compiled and skipped because `WF_TEST_HOST` was not configured. No Windows Server or client release has been validated by a real servicing run here.
- Real reboots, source-backed feature installations, Hyper-V/nested virtualization and builder-specific WinRM recovery require disposable Windows guests.
- ARM64 artifacts were cross-compiled, not executed on ARM64 hardware.
- The local race test could not run because CGO was disabled and no C compiler was available in the task environment.
- Signed release publication, remote `packer init` downloads and GitHub-hosted workflow execution were not performed. Snapshot packaging skipped signing and publication. No commit, tag, push or remote repository was created.

See [testing](testing.md), [support](support.md) and [release preparation](releasing.md) before production promotion.
