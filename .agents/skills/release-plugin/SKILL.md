---
name: release-plugin
description: Prepare or review this plugin's GitHub Actions, semantic versioning, GoReleaser packaging and release documentation without implicitly publishing.
---

Read `docs/releasing.md`, `.goreleaser.yml` and both workflows. Use the SDK's `describe` output for the API version. Preserve Packer's executable, archive, checksum and signature names and the source address without `packer-plugin-`.

Run available checks, schema repeatability, GoReleaser validation, both Windows architecture builds and native Packer install/HCL validation. Inspect snapshot archives/checksums. Snapshot mode tolerates missing Git history; real releases need an existing committed/tagged checkout. Do not create source history or tags as a side effect of a packaging check.

Keep PR checks read-only, signing secrets restricted to release jobs and releases draft pending review. Report which native OS and live guest checks actually ran. Updating automation does not authorize pushing tags, publishing, creating a remote repository or changing external infrastructure.
