package plan

import (
	"strings"
	"testing"

	"github.com/larskroghdk/packer-plugin-windows-features/internal/facts"
)

// TestNetworkRoleProtection rejects removals that could carry the active
// Packer connection while allowing ordinary role and client feature changes.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestNetworkRoleProtection(t *testing.T) {
	for _, name := range []string{"DNS", "DHCP", "RemoteAccess", "Routing", "DirectAccess-VPN", "AD-Domain-Services", "Hyper-V"} {
		if Protect(facts.InterfaceServerManager, name, nil) == "" {
			t.Errorf("network role %s was not protected", name)
		}
	}
	if Protect(facts.InterfaceOptionalFeature, "Microsoft-Hyper-V-All", nil) == "" || Protect(facts.InterfaceOptionalFeature, "TelnetClient", nil) != "" {
		t.Fatal("incorrect client protection")
	}
}

// TestInstalledDependencyConflict rejects removing a dependency even when
// the requested installation and its dependency were already compliant.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestInstalledDependencyConflict(t *testing.T) {
	inv := serverInventory(t)
	p := Build([]Request{{Name: "PowerShell", Kind: KindFeature, Action: ActionInstall}, {Name: "Shell-Base", Kind: KindFeature, Action: ActionRemove}}, facts.InterfaceServerManager, Inventories{facts.InterfaceServerManager: inv})
	if p.Count(StatusConflict) != 2 {
		t.Fatalf("installed dependency conflict was not rejected: %+v", p.Items)
	}
}

// TestCompliantStateAfterRestart verifies that a restart cannot silently
// invalidate a feature that was compliant before changes.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestCompliantStateAfterRestart(t *testing.T) {
	inv := serverInventory(t)
	p := Build([]Request{{Name: "Print-Server", Kind: KindFeature, Action: ActionInstall}}, facts.InterfaceServerManager, Inventories{facts.InterfaceServerManager: inv})
	inv.Get("Print-Server").State = "Available"
	p.Reverify(Inventories{facts.InterfaceServerManager: inv})
	if p.Count(StatusFailed) != 1 {
		t.Fatal("a compliant request lost its state without failing validation")
	}
}

// serverInventory returns a Server Manager inventory with IIS, the print
// role, PowerShell and .NET Framework 4.
//
// Author: Lars Krogh
// Version: 1.0.0
func serverInventory(t *testing.T) *Inventory {
	t.Helper()
	inv, err := NewInventory(facts.InterfaceServerManager, []Feature{
		{Name: "Web-Server", State: "Available", Type: "Role"},
		{Name: "Web-WebServer", State: "Available", Parent: "Web-Server"},
		{Name: "Web-Mgmt-Tools", State: "Available", Parent: "Web-Server"},
		{Name: "Web-Mgmt-Console", State: "Available", Parent: "Web-Mgmt-Tools"},
		{Name: "NET-Framework-45-Features", State: "Installed"},
		{Name: "NET-Framework-45-Core", State: "Installed", Parent: "NET-Framework-45-Features"},
		{Name: "PowerShellRoot", State: "Installed"},
		{Name: "PowerShell", State: "Installed", Parent: "PowerShellRoot", DependsOn: []string{"NET-Framework-45-Core", "Shell-Base"}},
		{Name: "Shell-Base", State: "Installed"},
		{Name: "Print-Services", State: "Installed"},
		{Name: "Print-Server", State: "Installed", Parent: "Print-Services"},
		{Name: "Hyper-V", State: "InstallPending"},
		{Name: "Telnet-Client", State: "UninstallPending"},
		{Name: "Odd", State: "PartiallyInstalled"},
	})
	if err != nil {
		t.Fatal(err)
	}
	return inv
}

// item returns the plan item requested as name.
//
// Author: Lars Krogh
// Version: 1.0.0
func item(t *testing.T, p *Plan, name string) *Item {
	t.Helper()
	for _, it := range p.Items {
		if it.Request.Name == name {
			return it
		}
	}
	t.Fatalf("%s is not in the plan", name)
	return nil
}

// TestNormalize checks the state mapping of every interface, and that
// unknown states are never compliant.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestNormalize(t *testing.T) {
	tests := []struct {
		iface facts.Interface
		raw   string
		want  State
	}{
		{facts.InterfaceServerManager, "Installed", StateInstalled},
		{facts.InterfaceServerManager, "Available", StateAvailable},
		{facts.InterfaceServerManager, "Removed", StateRemoved},
		{facts.InterfaceServerManager, "InstallPending", StateInstallPending},
		{facts.InterfaceServerManager, "UninstallPending", StateRemovePending},
		{facts.InterfaceOptionalFeature, "Enabled", StateInstalled},
		{facts.InterfaceOptionalFeature, "disabled", StateAvailable},
		{facts.InterfaceOptionalFeature, "DisabledWithPayloadRemoved", StateRemoved},
		{facts.InterfaceOptionalFeature, "EnablePending", StateInstallPending},
		{facts.InterfaceOptionalFeature, "DisablePending", StateRemovePending},
		{facts.InterfaceCapability, "NotPresent", StateAvailable},
		{facts.InterfaceCapability, "Staged", StateAvailable},
		{facts.InterfaceCapability, "Installed", StateInstalled},
		{facts.InterfaceCapability, "Superseded", StateUnknown},
		{facts.InterfaceOptionalFeature, "Installed", StateUnknown},
		{facts.InterfaceServerManager, "", StateUnknown},
	}
	for _, tc := range tests {
		if got := Normalize(tc.iface, tc.raw); got != tc.want {
			t.Errorf("%s %q: %s, want %s", tc.iface, tc.raw, got, tc.want)
		}
	}
}

// TestParseInventory checks decoding and the rejection of unknown
// interfaces, duplicates and empty names.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestParseInventory(t *testing.T) {
	inv, err := ParseInventory([]byte(`{"interface":"optional_feature","features":[{"name":"NetFx3","state":"Enabled"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if st, ok := inv.StateOf("netfx3"); !ok || st != StateInstalled {
		t.Errorf("state: %s %v", st, ok)
	}
	for _, bad := range []string{
		`{"interface":"winget","features":[]}`,
		`{"interface":"capability","features":[{"name":"A"},{"name":"a"}]}`,
		`{"interface":"capability","features":[{"name":""}]}`,
		`[`,
	} {
		if _, err := ParseInventory([]byte(bad)); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
}

// TestResolveCapability checks exact, case-insensitive and version-less
// capability names, and that an ambiguous identity is not guessed.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestResolveCapability(t *testing.T) {
	inv, _ := NewInventory(facts.InterfaceCapability, []Feature{
		{Name: "OpenSSH.Server~~~~0.0.1.0", State: "NotPresent"},
		{Name: "Language.Basic~~~en-US~0.0.1.0", State: "Installed"},
		{Name: "Language.Basic~~~de-DE~0.0.1.0", State: "NotPresent"},
	})
	for _, name := range []string{"OpenSSH.Server~~~~0.0.1.0", "openssh.server", "Language.Basic~~~de-DE~0.0.1.0"} {
		if f, why := inv.Resolve(name); f == nil {
			t.Errorf("%s: %s", name, why)
		}
	}
	if f, why := inv.Resolve("Language.Basic"); f != nil || !strings.Contains(why, "matches 2 capabilities") {
		t.Errorf("ambiguous: %v %s", f, why)
	}
	if f, why := inv.Resolve("OpenSSH"); f != nil || !strings.Contains(why, "Get-WindowsCapability") {
		t.Errorf("unknown: %v %s", f, why)
	}
	features, _ := NewInventory(facts.InterfaceOptionalFeature, []Feature{{Name: "OpenSSH.Server~x", State: "Disabled"}})
	if f, _ := features.Resolve("OpenSSH.Server"); f != nil {
		t.Error("prefix matching must only apply to capabilities")
	}
}

// TestClosures checks what an install adds and a removal takes away,
// including transitive parents and dependents.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestClosures(t *testing.T) {
	inv := serverInventory(t)
	if got := strings.Join(inv.InstallClosure("Web-Mgmt-Console"), ","); got != "Web-Mgmt-Tools,Web-Server" {
		t.Errorf("install closure: %s", got)
	}
	if got := inv.InstallClosure("PowerShell"); len(got) != 0 {
		t.Errorf("installed dependencies must not be listed: %v", got)
	}
	if got := strings.Join(inv.RemovalClosure("NET-Framework-45-Features"), ","); got != "NET-Framework-45-Core,PowerShell" {
		t.Errorf("removal closure: %s", got)
	}
	if got := inv.RemovalClosure("Web-Server"); len(got) != 0 {
		t.Errorf("features that are not installed must not be listed: %v", got)
	}
}

// TestBuild checks the initial status of each kind of request.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestBuild(t *testing.T) {
	p := Build([]Request{
		{"Web-Server", KindFeature, ActionInstall},
		{"powershellroot", KindFeature, ActionInstall},
		{"Hyper-V", KindFeature, ActionInstall},
		{"Telnet-Client", KindFeature, ActionRemove},
		{"Print-Services", KindFeature, ActionRemove},
		{"NET-Framework-45-Features", KindFeature, ActionRemove},
		{"Web-WebServer", KindFeature, ActionRemove},
		{"Shell-Base", KindFeature, ActionRemove},
		{"Odd", KindFeature, ActionInstall},
		{"Nope", KindFeature, ActionInstall},
		{"OpenSSH.Server", KindCapability, ActionInstall},
	}, facts.InterfaceServerManager, Inventories{facts.InterfaceServerManager: serverInventory(t)})
	want := map[string]Status{
		"Web-Server":                StatusPlanned,
		"powershellroot":            StatusCompliant,
		"Hyper-V":                   StatusPendingReboot,
		"Telnet-Client":             StatusPendingReboot,
		"Print-Services":            StatusPlanned,
		"NET-Framework-45-Features": StatusProtected,
		"Web-WebServer":             StatusCompliant,
		"Shell-Base":                StatusProtected,
		"Odd":                       StatusFailed,
		"Nope":                      StatusUnavailable,
		"OpenSSH.Server":            StatusUnavailable,
	}
	for name, status := range want {
		if it := item(t, p, name); it.Status != status {
			t.Errorf("%s: %s (%s), want %s", name, it.Status, it.Reason, status)
		}
	}
	if it := item(t, p, "powershellroot"); it.Name != "PowerShellRoot" {
		t.Errorf("canonical name: %s", it.Name)
	}
	if !strings.Contains(item(t, p, "Shell-Base").Reason, "also remove PowerShell") {
		t.Errorf("protection reason: %s", item(t, p, "Shell-Base").Reason)
	}
	if p.Items[0].Action != ActionRemove || p.Items[len(p.Items)-1].Action != ActionInstall {
		t.Error("removals must come before installs")
	}
	if len(p.Blocking(false)) != 5 || len(p.Blocking(true)) != 3 {
		t.Errorf("blocking: %d, ignoring unavailable %d", len(p.Blocking(false)), len(p.Blocking(true)))
	}
	if !p.RebootRequired() {
		t.Error("pending items must require a restart")
	}
	if got := p.Interfaces(); len(got) != 1 || got[0] != facts.InterfaceServerManager {
		t.Errorf("interfaces: %v", got)
	}
}

// TestConflicts checks that contradictory requests are both marked.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestConflicts(t *testing.T) {
	inv := Inventories{facts.InterfaceServerManager: serverInventory(t)}
	p := Build([]Request{{"Web-Mgmt-Console", KindFeature, ActionInstall}, {"Web-Server", KindFeature, ActionRemove}}, facts.InterfaceServerManager, inv)
	for _, it := range p.Items {
		if it.Status != StatusConflict {
			t.Errorf("%s: %s", it.Request.Name, it.Status)
		}
	}
	p = Build([]Request{{"PowerShell", KindFeature, ActionInstall}, {"NET-Framework-45-Core", KindFeature, ActionRemove}}, facts.InterfaceServerManager, inv)
	if item(t, p, "PowerShell").Status != StatusConflict {
		t.Errorf("install of a feature whose dependency is removed: %+v", item(t, p, "PowerShell"))
	}
	caps, _ := NewInventory(facts.InterfaceCapability, []Feature{{Name: "OpenSSH.Server~~~~0.0.1.0", State: "NotPresent"}})
	p = Build([]Request{{"OpenSSH.Server", KindCapability, ActionInstall}, {"OpenSSH.Server~~~~0.0.1.0", KindCapability, ActionInstall}}, facts.InterfaceServerManager, Inventories{facts.InterfaceCapability: caps})
	if p.Count(StatusConflict) != 2 {
		t.Errorf("two names for one capability: %+v %+v", p.Items[0], p.Items[1])
	}
}

// TestVerify checks that the outcome follows the state read back, not the
// operation's result.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestVerify(t *testing.T) {
	before := Inventories{facts.InterfaceServerManager: serverInventory(t)}
	p := Build([]Request{
		{"Web-Server", KindFeature, ActionInstall},
		{"Web-Mgmt-Tools", KindFeature, ActionInstall},
		{"Print-Services", KindFeature, ActionRemove},
		{"Print-Server", KindFeature, ActionRemove},
	}, facts.InterfaceServerManager, before)
	afterInv, _ := NewInventory(facts.InterfaceServerManager, []Feature{
		{Name: "Web-Server", State: "Installed"},
		{Name: "Web-WebServer", State: "Installed", Parent: "Web-Server"},
		{Name: "Web-Mgmt-Tools", State: "Available"},
		{Name: "Print-Services", State: "UninstallPending"},
		{Name: "Print-Server", State: "Available"},
	})
	after := Inventories{facts.InterfaceServerManager: afterInv}
	failed := "Server Manager reported exit code Failed"
	results := map[string]*OperationResult{
		item(t, p, "Web-Server").ID:     {OK: false, Error: &failed},
		item(t, p, "Web-Mgmt-Tools").ID: {OK: true},
		item(t, p, "Print-Services").ID: {OK: true, RestartNeeded: true},
	}
	p.Verify(p.Pending(), results, after)
	p.RecordSideEffects(before, after)
	checks := map[string]Status{"Web-Server": StatusInstalled, "Web-Mgmt-Tools": StatusFailed, "Print-Services": StatusPendingReboot, "Print-Server": StatusNotAttempted}
	for name, status := range checks {
		if it := item(t, p, name); it.Status != status {
			t.Errorf("%s: %s (%s), want %s", name, it.Status, it.Reason, status)
		}
	}
	if !strings.Contains(item(t, p, "Web-Server").Reason, "reported an error") {
		t.Errorf("reason: %s", item(t, p, "Web-Server").Reason)
	}
	if len(p.SideEffects) != 1 || p.SideEffects[0].Name != "Web-WebServer" {
		t.Errorf("side effects: %+v", p.SideEffects)
	}
	if got := len(p.Unsuccessful(false)); got != 2 {
		t.Errorf("unsuccessful: %d", got)
	}

	completed, _ := NewInventory(facts.InterfaceServerManager, []Feature{
		{Name: "Web-Server", State: "Available"},
		{Name: "Web-Mgmt-Tools", State: "Available"},
		{Name: "Print-Services", State: "Available"},
		{Name: "Print-Server", State: "Available"},
	})
	p.Reverify(Inventories{facts.InterfaceServerManager: completed})
	if it := item(t, p, "Print-Services"); it.Status != StatusRemoved || it.RestartNeeded {
		t.Errorf("after restart: %+v", it)
	}
	if it := item(t, p, "Web-Server"); it.Status != StatusFailed || !strings.Contains(it.Reason, "after the restart") {
		t.Errorf("lost after restart: %+v", it)
	}
	p.Reverify(Inventories{})
	if p.Count(StatusPendingReboot) != 0 || item(t, p, "Print-Services").Status != StatusFailed {
		t.Error("a missing inventory must fail the items it should verify")
	}
}

// TestFailPending checks that pending items fail after the restarts.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestFailPending(t *testing.T) {
	p := Build([]Request{{"Hyper-V", KindFeature, ActionInstall}}, facts.InterfaceServerManager, Inventories{facts.InterfaceServerManager: serverInventory(t)})
	p.Items[0].After = StateInstallPending
	p.FailPending(3)
	if p.Items[0].Status != StatusFailed || !strings.Contains(p.Items[0].Reason, "after 3 restarts") {
		t.Errorf("%+v", p.Items[0])
	}
}

// TestProtect checks every session-critical name and that other names pass.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestProtect(t *testing.T) {
	for _, iface := range []facts.Interface{facts.InterfaceServerManager, facts.InterfaceOptionalFeature} {
		names := SessionCritical(iface)
		if len(names) == 0 {
			t.Errorf("%s has no protected names", iface)
		}
		for _, name := range names {
			if Protect(iface, strings.ToUpper(name), nil) == "" {
				t.Errorf("%s %s is not protected", iface, name)
			}
		}
	}
	if Protect(facts.InterfaceServerManager, "Web-Server", []string{"Web-WebServer"}) != "" {
		t.Error("IIS removal was protected")
	}
	if Protect(facts.InterfaceOptionalFeature, "MicrosoftWindowsPowerShellV2Root", nil) != "" {
		t.Error("removing the PowerShell 2.0 engine must stay possible")
	}
}

// TestParseOperationResult checks decoding of the guest's operation result.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestParseOperationResult(t *testing.T) {
	r, err := ParseOperationResult([]byte(`{"id":"x","ok":false,"restart_needed":true,"exit_code":"Failed","changed":[],"error":"boom"}`))
	if err != nil || r.OK || !r.RestartNeeded || r.Error == nil || *r.Error != "boom" {
		t.Errorf("%+v %v", r, err)
	}
	if _, err := ParseOperationResult([]byte(`nope`)); err == nil {
		t.Error("invalid JSON was accepted")
	}
}
