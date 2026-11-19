package provisioner

import "strings"

var servicingHints = []struct {
	match []string
	hint  string
}{
	{
		[]string{"0x800f081f", "0x800f0906", "0x800f0907", "0x800f0950", "source files could not be found", "source files could not be downloaded"},
		"the feature payload is not on the system: set source to the sources\\sxs folder or a WIM:<path>:<index> of matching installation media, or let the guest reach Windows Update",
	},
	{
		[]string{"0x800f0954"},
		"a WSUS or Windows Update for Business policy blocks the download: set source, or allow optional content from Windows Update by policy",
	},
	{
		[]string{"0x800f0922", "0x800f0831"},
		"a servicing step failed: check for a pending restart, then C:\\Windows\\Logs\\CBS\\CBS.log on the guest",
	},
	{
		[]string{"0x80070bc2", "0x800f0821", "pending restart", "reboot is pending", "restart is pending"},
		"Windows needs a restart before it can service features: set restart_if_required, or run a windows-restart provisioner first",
	},
	{
		[]string{"hyper-v cannot be installed", "processor does not have required virtualization capabilities"},
		"the virtual machine does not expose virtualization extensions: enable nested virtualization on the host",
	},
}

// withHint appends the troubleshooting hint for a known servicing error code
// or message to msg.
//
// Author: Lars Krogh
// Version: 1.0.0
func withHint(msg string) string {
	lower := strings.ToLower(msg)
	for _, h := range servicingHints {
		for _, m := range h.match {
			if strings.Contains(lower, m) {
				return msg + " (hint: " + h.hint + ")"
			}
		}
	}
	return msg
}
