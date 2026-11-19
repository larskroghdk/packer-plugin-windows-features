---
name: support-windows-release
description: Add or correct Windows release classification and servicing compatibility in this plugin, with honest validation records.
---

Read `internal/facts/release.go`, facts tests and `docs/support.md`. Use numeric product type, build and installation type; product names alone can mislabel Windows 11 as Windows 10. Confirm build and servicing availability against official Microsoft sources.

Add Server Core/Desktop or client fixtures and rejection tests as appropriate. Keep capability availability based on actual cmdlets. Do not infer architecture or feature availability from release name. Unknown newer builds should remain explicitly untested.

Update documentation and run unit tests. Execute live `test/integration` only against configured authorized guests, recording edition, build, architecture, builder, tested feature, reboot and connectivity results. Without a real successful run, document implementation support rather than validation.
