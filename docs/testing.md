# Testing

Use the Go version in `go.mod` (automatic toolchain selection is supported), Packer, staticcheck 2025.1.1, PSScriptAnalyzer 1.22.0 and Pester 6.2.0. Guest tests must pass under Windows PowerShell 5.1, not only PowerShell 7.

```powershell
go test ./...
go vet ./...
staticcheck ./...
go run ./cmd/doccheck .
packer fmt -check -recursive examples
packer fmt -check -recursive test/integration
powershell.exe -NoProfile -Command 'Import-Module Pester -MinimumVersion 5.0; Invoke-Pester -Path ./test/pester -Output Detailed -CI'
$issues = 'guest/scripts','test/pester' | ForEach-Object { Invoke-ScriptAnalyzer -Path $_ -Recurse -Settings ./PSScriptAnalyzerSettings.psd1 }
$issues
if ($issues) { throw 'PSScriptAnalyzer failed' }
```

`make check` runs these checks where GNU Make, POSIX utilities and the PowerShell tools are available. CI runs Go and mocked servicing tests on Windows. The fake guest covers orchestration without requiring administrator rights or changing host features. All guest `.ps1` files are ASCII with CRLF; the Go embedding test enforces this.

## Packer smoke check

Use an isolated installation directory to avoid changing your normal plugin installation:

```powershell
$env:PACKER_PLUGIN_PATH = "$PWD/dist/plugins"
go build -ldflags='-X main.versionPrerelease=' -o dist/packer-plugin-windows-features.exe .
packer plugins install --path ./dist/packer-plugin-windows-features.exe github.com/larskroghdk/windows-features
packer init test/integration
packer plugins installed
packer validate -var host=127.0.0.1 -var password=unused -var report_path=reports/check.json test/integration
```

This validates SDK loading and HCL configuration without contacting a guest. Run `go test -race ./...` where a C compiler and the race runtime are available. For schema changes regenerate using `make generate`, then compare `provisioner/config.hcl2spec.go` with its previous contents; generated code should be reproducible.

## Live integration

Live tests are build-tagged and skip unless `WF_TEST_HOST` is set. There is no public infrastructure or embedded password. Run against an authorized disposable VM with a checkpoint and a working elevated WinRM endpoint.

```powershell
$env:WF_TEST_HOST = '192.0.2.10'
$env:WF_TEST_USERNAME = 'Administrator'
$env:WF_TEST_PASSWORD = '<supply privately>'
$env:WF_TEST_FEATURE = 'Telnet-Client'
go test -tags integration -v -count=1 -timeout 90m ./test/integration
```

The default is dry run. Server can use `Telnet-Client`; client uses `TelnetClient`, where the edition exposes it. Choose an initially absent, payload-available feature: the suite refuses to remove pre-existing features. TLS certificate validation is on by default. `WF_TEST_USE_SSL=false` selects HTTP; `WF_TEST_INSECURE=true` is for disposable self-signed test endpoints only.

Set `WF_TEST_ALLOW_CHANGES=yes` to install, run again to check idempotency, and remove the feature. The suite permits required reboots. A failed test can leave changes behind; restore the external checkpoint. Do not assume removal restores every dependency or payload to its original state.

Repeat on Server 2016/2019/2022/2025 and Windows 10/11, Core/Desktop, supported architectures and actual builder types before recording platform validation. Separately validate source-backed .NET, Hyper-V nested virtualization, capabilities, role dependency changes, interrupted WinRM and real reboot behavior. These require lab infrastructure and are not covered by a green unit suite.
