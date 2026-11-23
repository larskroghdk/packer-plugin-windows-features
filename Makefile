GOPATH := $(shell go env GOPATH | tr '\\' '/')
PACKER_SDC := $(GOPATH)/bin/packer-sdc
PACKER_SDC_VERSION := $(shell go list -m -f '{{.Version}}' github.com/hashicorp/packer-plugin-sdk)
API_VERSION = $(shell go run . describe | sed -n 's/.*"api_version":"\([^"]*\)".*/\1/p')
PESTER_SHELL ?= powershell.exe

.PHONY: check lint test test-go test-ps test-integration build generate generate-check release-snapshot
check: lint test

lint:
	go mod tidy -diff
	@test -z "$$(gofmt -l .)" || (gofmt -l .; exit 1)
	go vet ./...
	go vet -tags integration ./test/integration
	go run honnef.co/go/tools/cmd/staticcheck@2025.1.1 ./...
	go run golang.org/x/vuln/cmd/govulncheck@v1.10.0 ./...
	go run ./cmd/doccheck .
	sh ./psdoccheck.sh
	pwsh -NoProfile -Command '$$ErrorActionPreference = "Stop"; $$r = "guest/scripts", "test/pester" | ForEach-Object { Invoke-ScriptAnalyzer -Path $$_ -Recurse -Settings ./PSScriptAnalyzerSettings.psd1 }; $$r; if ($$r) { exit 1 }'
	packer fmt -check -recursive examples
	packer fmt -check -recursive test/integration

test: test-go test-ps
test-go:
	go test ./...
test-ps:
	$(PESTER_SHELL) -NoProfile -ExecutionPolicy Bypass -Command 'Import-Module Pester -MinimumVersion 5.0; Invoke-Pester -Path ./test/pester -Output Detailed -CI'
test-integration:
	go test -tags integration -v -count=1 -timeout 90m ./test/integration
build:
	go build -trimpath -o dist/packer-plugin-windows-features.exe .
generate:
	go install github.com/hashicorp/packer-plugin-sdk/cmd/packer-sdc@$(PACKER_SDC_VERSION)
	cd provisioner && GOFILE=config.go GOPACKAGE=provisioner $(PACKER_SDC) mapstructure-to-hcl2 -type Config
	gofmt -w provisioner/config.hcl2spec.go
generate-check: generate
	git diff --exit-code -- provisioner/config.hcl2spec.go
release-snapshot:
	API_VERSION="$(API_VERSION)" goreleaser release --snapshot --skip=publish,sign --clean
