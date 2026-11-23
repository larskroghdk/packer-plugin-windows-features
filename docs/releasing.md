# 🏷️ Releases

Release automation does not publish anything during local development. Do not push a tag or publish without maintainer authorization.

1. Complete checks in [testing](testing.md), inspect regenerated HCL, and record successful live validation for every supported guest claim. Resolve limitations before promoting a production release.
2. Update `main.go`'s version for the next cycle and use stable semantic version tags such as `v1.0.0`. Normal local builds advertise the stable version. Packer accepts stable plugin releases and local `-dev` builds, not arbitrary alpha/beta/rc identifiers. Release builds inject the tag version and clear `versionPrerelease` with linker flags.
3. Review `.goreleaser.yml` with `goreleaser check`. Export `API_VERSION` from `go run . describe` (`api_version`), then run `goreleaser release --snapshot --skip=publish,sign --clean`. Snapshot mode tolerates missing Git history and uses `0.0.0-dev`; real releases require a committed, tagged checkout.
4. Inspect both Windows archives (amd64 and arm64), executable names, versions and SHA256 entries. Install the native executable with `packer plugins install --path`, run `packer init`, `packer plugins installed`, and `packer validate` against the integration template.
5. Configure repository secrets `GPG_PRIVATE_KEY` and `GPG_PASSPHRASE` using a dedicated signing key. The release workflow imports it, signs the checksum file and publishes a signed GitHub release for an authorized version tag after reusable checks pass.

Archives use `packer-plugin-windows-features_v<VERSION>_<API>_<OS>_<ARCH>.zip`; the contained executable has the same stem and Windows adds `.exe`. The checksum manifest is `packer-plugin-windows-features_v<VERSION>_SHA256SUMS`, with its detached `.sig`. These follow the [Packer release conventions](https://developer.hashicorp.com/packer/docs/plugins/creation) and [loading specification](https://developer.hashicorp.com/packer/docs/plugins/creation/plugin-load-spec). The plugin source address omits `packer-plugin-`: `github.com/larskroghdk/windows-features`.

Tags publish signed releases after checks; no repository or tag is created by the workflow. Review the independent check workflow as well as release packaging; successful cross-compilation does not prove that Windows servicing or WinRM recovery works on each guest.
