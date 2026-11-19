# Configuration

At least one request is required. Names cannot be empty, duplicated across the same namespace, contain wildcards, or contradict another request. Unrecognized options fail preparation. A feature name is not interchangeable with a capability identity.

| Option | Type | Default | Behavior |
| --- | --- | --- | --- |
| `install` | list(string) | `[]` | Install Server roles/features, or enable client optional features. |
| `remove` | list(string) | `[]` | Uninstall Server roles/features, or disable client optional features. |
| `install_capabilities` | list(string) | `[]` | Add capabilities through DISM on either family, when available. |
| `remove_capabilities` | list(string) | `[]` | Remove capabilities through DISM. |
| `include_management_tools` | bool | `false` | Pass Server Manager's management-tools switch on installation and removal. Ignored with a warning for client features. |
| `include_all_subfeatures` | bool | `false` | Install all Server Manager subfeatures; requires a nonempty `install`. DISM optional-feature installation always enables required parents with `-All`. |
| `source` | string | `""` | Guest-visible payload path for installations; Windows servicing resolves it. Match the guest's release, architecture and language. |
| `limit_access` | bool | `false` | DISM installations do not contact Windows Update; requires `source`. Server Manager has no equivalent switch. |
| `remove_payload` | bool | `false` | Remove payload when disabling/uninstalling features; requires `remove`. Does not apply to capabilities. Already disabled features are skipped, so this is not a payload-cleanup operation. |
| `ignore_unavailable` | bool | `false` | Warn and skip names absent from the inventory or unsupported namespaces. Unknown states, protection and contradictions remain fatal. |
| `dry_run` | bool | `false` | Detect, inventory and plan; no servicing or restart. Guest scripts and temporary JSON files are uploaded and cleaned up. |
| `fail_on_error` | bool | `true` | Fail on unsuccessful requests. False retains failed requests in the report and warns. Does not soften configuration, preflight, inventory, restart or connectivity failures. |
| `restart_if_required` | bool | `false` | Restart for pending servicing or operations needing restart, then verify again. |
| `restart_timeout` | duration string | `"30m"` | Maximum wait per restart, including settling; minimum `1m`. |
| `operation_timeout` | duration string | `"60m"` | Timeout per servicing operation; minimum `1m`. A timeout stops subsequent operations because servicing may continue on the guest. |
| `report_path` | string | `windows-features-report-<build>.json` | JSON file on the build host; parent directories are created. Set a unique path for parallel builds. |

Management-tools and all-subfeatures switches modify a necessary Server Manager operation. They do not independently reconcile children of an already-installed role. List required role services or tools explicitly to validate them and preserve idempotency.

## Report contract

Schema version 1 includes `plugin`, start/finish times, duration, detected `system`, `summary`, per-request states, `side_effects`, `connectivity`, `reboot`, `warnings`, and `validation`. A failed run includes an `error` field and fails report validation even if the inventory was already compliant. Incompatible changes increment `schema_version`.

Request statuses are `planned`, `compliant`, `installed`, `removed`, `pending_reboot`, `unavailable`, `protected`, `conflict`, `failed` and `not_attempted`. Raw servicing states are normalized without assuming an unknown state is compliant. Exit code zero is insufficient: the inventory must show the requested state or its matching pending state.

`validation.passed` means the inventory reached the desired state or its pending-restart state, subject to `ignore_unavailable`, and recorded connectivity checks passed. Inspect `reboot.required` and `pending_reboot` before treating the guest as ready. Dry runs with planned changes fail final-state validation while returning a successful planning result. `fail_on_error = false` can return success with `validation.passed = false`; image promotion should check both.
