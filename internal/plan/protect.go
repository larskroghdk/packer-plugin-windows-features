package plan

import (
	"fmt"
	"strings"

	"github.com/larskroghdk/packer-plugin-windows-features/internal/facts"
)

var sessionCritical = map[facts.Interface]map[string]string{
	facts.InterfaceServerManager: {
		"dns":                       "the active build connection may depend on this DNS server",
		"dhcp":                      "the active build connection may depend on this DHCP server",
		"remoteaccess":              "remote access and routing may carry the active build connection",
		"routing":                   "routing may carry the active build connection",
		"directaccess-vpn":          "VPN services may carry the active build connection",
		"hyper-v":                   "removing the hypervisor can remove virtual network connectivity",
		"ad-domain-services":        "domain services may provide authentication for the active build connection",
		"powershellroot":            "Windows PowerShell runs the plugin's guest actions and Packer's PowerShell provisioners",
		"powershell":                "Windows PowerShell runs the plugin's guest actions and Packer's PowerShell provisioners",
		"net-framework-45-features": "Windows PowerShell 5.1 and the WinRM PowerShell endpoint run on .NET Framework 4.x",
		"net-framework-45-core":     "Windows PowerShell 5.1 and the WinRM PowerShell endpoint run on .NET Framework 4.x",
		"wow64-support":             "32-bit programs, including parts of the WinRM toolchain and most installers, need WoW64",
	},
	facts.InterfaceOptionalFeature: {
		"microsoft-hyper-v-all":        "removing Hyper-V can remove virtual network connectivity",
		"microsoft-hyper-v":            "removing Hyper-V can remove virtual network connectivity",
		"microsoft-hyper-v-services":   "removing Hyper-V services can remove virtual network connectivity",
		"microsoft-hyper-v-hypervisor": "removing the hypervisor can remove virtual network connectivity",
		"netfx4-advsrvs":               "Windows PowerShell 5.1 and the WinRM PowerShell endpoint run on .NET Framework 4.x",
	},
	facts.InterfaceCapability: {},
}

// Protect returns why removing name, together with the features its removal
// also removes, would break the Packer session, or "" when it is safe. It is
// checked before any change is made.
//
// Parameters:
//   - also: The features removed as a side effect, from
//     Inventory.RemovalClosure.
//
// Author: Lars Krogh
// Version: 1.0.0
func Protect(iface facts.Interface, name string, also []string) string {
	rules := sessionCritical[iface]
	if why, ok := rules[strings.ToLower(name)]; ok {
		return fmt.Sprintf("removing %s would break the Packer session: %s", name, why)
	}
	for _, dep := range also {
		if why, ok := rules[strings.ToLower(dep)]; ok {
			return fmt.Sprintf("removing %s would also remove %s, which would break the Packer session: %s", name, dep, why)
		}
	}
	return ""
}

// SessionCritical returns the protected names of iface, in lower case, for
// documentation and tests.
//
// Author: Lars Krogh
// Version: 1.0.0
func SessionCritical(iface facts.Interface) []string {
	out := make([]string, 0, len(sessionCritical[iface]))
	for name := range sessionCritical[iface] {
		out = append(out, name)
	}
	return out
}
