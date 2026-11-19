package provisioner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/larskroghdk/packer-plugin-windows-features/internal/facts"
	"github.com/larskroghdk/packer-plugin-windows-features/internal/plan"
)

var actionPattern = regexp.MustCompile(`-Action (\w+) -OutputPath "([^"]+)"(?: -InputPath "([^"]+)")?`)

// fakeGuest is an in-memory Windows guest behind a packer.Communicator. It
// implements the guest actions of Invoke-WindowsFeatures.ps1 over an
// inventory per servicing interface, with Server Manager's dependency
// behavior, and fails the test when the plugin removes a session-critical
// feature.
type fakeGuest struct {
	t        *testing.T
	mu       sync.Mutex
	files    map[string][]byte
	facts    facts.Facts
	inv      map[facts.Interface][]plan.Feature
	bootTime int
	actions  []string
	commands []string
	ops      []operationRequest

	failOp      map[string]string
	ignoreOp    map[string]bool
	hangOp      map[string]bool
	pendingOn   map[string]bool
	stuck       map[string]bool
	defaultSubs map[string][]string
	mgmtTools   map[string]string
	breakWinRM  func(*facts.WinRM)
}

// newServerGuest returns a Windows Server 2022 Desktop Experience guest
// with a small Server Manager inventory and a capability inventory.
//
// Author: Lars Krogh
// Version: 1.0.0
func newServerGuest(t *testing.T) *fakeGuest {
	g := newGuest(t, facts.OS{
		ProductName: "Windows Server 2022 Datacenter", EditionID: "ServerDatacenter", InstallationType: "Server",
		Major: 10, Build: 20348, UBR: 2700, ProductType: facts.ProductTypeServer, Architecture: "AMD64", Is64BitProcess: true,
	}, facts.Servicing{ServerManager: true, OptionalFeatures: true, Capabilities: true})
	g.inv[facts.InterfaceServerManager] = []plan.Feature{
		{Name: "Web-Server", DisplayName: "Web Server (IIS)", State: "Available", Type: "Role", SubFeatures: []string{"Web-WebServer", "Web-Mgmt-Tools"}},
		{Name: "Web-WebServer", DisplayName: "Web Server", State: "Available", Type: "Role Service", Parent: "Web-Server"},
		{Name: "Web-Mgmt-Tools", DisplayName: "Management Tools", State: "Available", Type: "Role Service", Parent: "Web-Server", SubFeatures: []string{"Web-Mgmt-Console"}},
		{Name: "Web-Mgmt-Console", DisplayName: "IIS Management Console", State: "Available", Type: "Role Service", Parent: "Web-Mgmt-Tools"},
		{Name: "NET-Framework-Features", DisplayName: ".NET Framework 3.5 Features", State: "Available", Type: "Feature", SubFeatures: []string{"NET-Framework-Core"}},
		{Name: "NET-Framework-Core", DisplayName: ".NET Framework 3.5", State: "Removed", Type: "Feature", Parent: "NET-Framework-Features"},
		{Name: "NET-Framework-45-Features", DisplayName: ".NET Framework 4.8 Features", State: "Installed", Type: "Feature", SubFeatures: []string{"NET-Framework-45-Core"}},
		{Name: "NET-Framework-45-Core", DisplayName: ".NET Framework 4.8", State: "Installed", Type: "Feature", Parent: "NET-Framework-45-Features"},
		{Name: "PowerShellRoot", DisplayName: "Windows PowerShell", State: "Installed", Type: "Feature", SubFeatures: []string{"PowerShell"}},
		{Name: "PowerShell", DisplayName: "Windows PowerShell 5.1", State: "Installed", Type: "Feature", Parent: "PowerShellRoot", DependsOn: []string{"NET-Framework-45-Core"}},
		{Name: "Print-Services", DisplayName: "Print and Document Services", State: "Installed", Type: "Role", SubFeatures: []string{"Print-Server"}},
		{Name: "Print-Server", DisplayName: "Print Server", State: "Installed", Type: "Role Service", Parent: "Print-Services"},
		{Name: "Hyper-V", DisplayName: "Hyper-V", State: "Available", Type: "Role"},
		{Name: "RSAT-Hyper-V-Tools", DisplayName: "Hyper-V Management Tools", State: "Available", Type: "Feature"},
		{Name: "Telnet-Client", DisplayName: "Telnet Client", State: "Available", Type: "Feature"},
		{Name: "XPS-Viewer", DisplayName: "XPS Viewer", State: "Installed", Type: "Feature"},
	}
	g.inv[facts.InterfaceCapability] = []plan.Feature{
		{Name: "OpenSSH.Server~~~~0.0.1.0", State: "NotPresent"},
		{Name: "OpenSSH.Client~~~~0.0.1.0", State: "Installed"},
		{Name: "Language.Basic~~~en-US~0.0.1.0", State: "Installed"},
		{Name: "Language.Basic~~~de-DE~0.0.1.0", State: "NotPresent"},
	}
	g.defaultSubs["web-server"] = []string{"Web-WebServer"}
	g.mgmtTools["hyper-v"] = "RSAT-Hyper-V-Tools"
	return g
}

// newClientGuest returns a Windows 11 24H2 Enterprise guest with a small
// optional feature inventory and a capability inventory.
//
// Author: Lars Krogh
// Version: 1.0.0
func newClientGuest(t *testing.T) *fakeGuest {
	g := newGuest(t, facts.OS{
		ProductName: "Windows 10 Enterprise", EditionID: "Enterprise", InstallationType: "Client", DisplayVersion: "24H2",
		Major: 10, Build: 26100, UBR: 4061, ProductType: facts.ProductTypeWorkstation, Architecture: "AMD64", Is64BitProcess: true,
	}, facts.Servicing{OptionalFeatures: true, Capabilities: true})
	g.inv[facts.InterfaceOptionalFeature] = []plan.Feature{
		{Name: "Microsoft-Hyper-V-All", State: "Disabled"},
		{Name: "IIS-WebServerRole", State: "Disabled"},
		{Name: "NetFx3", State: "DisabledWithPayloadRemoved"},
		{Name: "NetFx4-AdvSrvs", State: "Enabled"},
		{Name: "MicrosoftWindowsPowerShellV2Root", State: "Enabled"},
		{Name: "TelnetClient", State: "Disabled"},
		{Name: "SMB1Protocol", State: "Disabled"},
	}
	g.inv[facts.InterfaceCapability] = []plan.Feature{
		{Name: "OpenSSH.Server~~~~0.0.1.0", State: "NotPresent"},
		{Name: "Rsat.ActiveDirectory.DS-LDS.Tools~~~~0.0.1.0", State: "NotPresent"},
	}
	return g
}

// newGuest returns a guest with os, servicing support, an elevated session
// and an HTTP WinRM listener, and no inventories.
//
// Author: Lars Krogh
// Version: 1.0.0
func newGuest(t *testing.T, os facts.OS, servicing facts.Servicing) *fakeGuest {
	return &fakeGuest{
		t:     t,
		files: map[string][]byte{},
		facts: facts.Facts{
			OS:        os,
			Session:   facts.Session{UserName: `BUILD\packer`, IsElevated: true},
			Servicing: servicing,
			Reboot:    &facts.PendingReboot{},
			WinRM: &facts.WinRM{
				ServiceStatus: "Running", ServiceStart: "Automatic",
				Dependencies:     []string{"HTTP", "RPCSS"},
				Listeners:        []facts.Listener{{Transport: "HTTP", Address: "*", Port: 5985, Enabled: true}},
				AllowUnencrypted: true,
				Auth:             facts.WinRMAuth{Basic: true, Negotiate: true, Kerberos: true},
			},
			Errors: []string{},
		},
		inv:         map[facts.Interface][]plan.Feature{},
		failOp:      map[string]string{},
		ignoreOp:    map[string]bool{},
		hangOp:      map[string]bool{},
		pendingOn:   map[string]bool{},
		stuck:       map[string]bool{},
		defaultSubs: map[string][]string{},
		mgmtTools:   map[string]string{},
	}
}

// feature returns the inventory entry of name in iface, or nil.
//
// Author: Lars Krogh
// Version: 1.0.0
func (g *fakeGuest) feature(iface facts.Interface, name string) *plan.Feature {
	for i := range g.inv[iface] {
		if strings.EqualFold(g.inv[iface][i].Name, name) {
			return &g.inv[iface][i]
		}
	}
	return nil
}

// state returns the raw state of name in iface, or "" when it is not listed.
//
// Author: Lars Krogh
// Version: 1.0.0
func (g *fakeGuest) state(iface facts.Interface, name string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	if f := g.feature(iface, name); f != nil {
		return f.State
	}
	return ""
}

// Start runs cmd against the fake guest and reports its exit status. A
// command for a hanging operation exits only when ctx is done.
//
// Returns:
//   - error: Always nil; failures are reported through the exit status.
//
// Author: Lars Krogh
// Version: 1.0.0
func (g *fakeGuest) Start(ctx context.Context, cmd *packer.RemoteCmd) error {
	g.mu.Lock()
	g.commands = append(g.commands, cmd.Command)
	code, out, hang := g.run(cmd.Command)
	g.mu.Unlock()
	go func() {
		if hang {
			<-ctx.Done()
			cmd.SetExited(1)
			return
		}
		if out != "" {
			fmt.Fprintln(cmd.Stdout, out)
		}
		cmd.SetExited(code)
	}()
	return nil
}

// run executes one command line.
//
// Returns:
//   - int: The exit status.
//   - string: Output for stdout.
//   - bool: The command hangs until cancelled.
//
// Author: Lars Krogh
// Version: 1.0.0
func (g *fakeGuest) run(command string) (int, string, bool) {
	switch {
	case strings.HasPrefix(command, "shutdown.exe /r"):
		g.reboot()
		return 0, "", false
	case strings.Contains(command, "New-Item -ItemType Directory"), strings.Contains(command, "Remove-Item -LiteralPath"):
		return 0, "", false
	}
	m := actionPattern.FindStringSubmatch(command)
	if m == nil {
		g.t.Errorf("unexpected guest command: %s", command)
		return 1, "", false
	}
	action, output, input := m[1], m[2], m[3]
	g.actions = append(g.actions, action)
	result, hang, err := g.action(action, g.files[input])
	if hang {
		return 0, "", true
	}
	if err != nil {
		return 1, "ERROR: " + err.Error(), false
	}
	data, err := json.Marshal(result)
	if err != nil {
		g.t.Fatal(err)
	}
	g.files[output] = data
	return 0, "", false
}

// action runs one guest action on its decoded input.
//
// Returns:
//   - any: The result to encode as JSON.
//   - bool: The action hangs.
//   - error: The input cannot be decoded or the action is unknown.
//
// Author: Lars Krogh
// Version: 1.0.0
func (g *fakeGuest) action(name string, input []byte) (any, bool, error) {
	switch name {
	case "Detect":
		f := g.facts
		r := *g.facts.Reboot
		f.Reboot = &r
		return f, false, nil
	case "Probe":
		r := *g.facts.Reboot
		probe := facts.Probe{Session: g.facts.Session, BootTime: fmt.Sprintf("2026-01-01T00:%02d:00Z", g.bootTime), Reboot: &r}
		if g.facts.WinRM == nil {
			msg := "access denied"
			probe.Error = &msg
			return probe, false, nil
		}
		w := *g.facts.WinRM
		w.Listeners = append([]facts.Listener(nil), w.Listeners...)
		probe.WinRM = &w
		return probe, false, nil
	case "Inventory":
		var req struct {
			Interface facts.Interface `json:"interface"`
		}
		if err := json.Unmarshal(input, &req); err != nil {
			return nil, false, err
		}
		features, ok := g.inv[req.Interface]
		if !ok {
			return nil, false, fmt.Errorf("the %s cmdlets are not available", req.Interface)
		}
		return map[string]any{"interface": req.Interface, "features": features}, false, nil
	case "Invoke":
		var req operationRequest
		if err := json.Unmarshal(input, &req); err != nil {
			return nil, false, err
		}
		g.ops = append(g.ops, req)
		if g.hangOp[strings.ToLower(req.Name)] {
			return nil, true, nil
		}
		return g.invoke(req), false, nil
	}
	return nil, false, fmt.Errorf("unknown action %s", name)
}

// invoke performs one operation the way the servicing interface would.
// Server Manager installs dependencies, parents and default sub-features,
// and removes dependents and children. A feature in pendingOn goes to its
// pending state and sets the servicing restart indicator.
//
// Author: Lars Krogh
// Version: 1.0.0
func (g *fakeGuest) invoke(req operationRequest) plan.OperationResult {
	res := plan.OperationResult{ID: req.ID, OK: true, ExitCode: "Success"}
	key := strings.ToLower(req.Name)
	if msg, ok := g.failOp[key]; ok {
		res.OK, res.Error, res.ExitCode = false, &msg, "Failed"
		return res
	}
	if g.ignoreOp[key] {
		return res
	}
	inv, err := plan.NewInventory(req.Interface, g.inv[req.Interface])
	if err != nil {
		g.t.Fatal(err)
	}
	target := g.feature(req.Interface, req.Name)
	if target == nil {
		msg := "feature name " + req.Name + " is unknown"
		res.OK, res.Error = false, &msg
		return res
	}
	if plan.Normalize(req.Interface, target.State) == plan.StateRemoved && req.Action == plan.ActionInstall && req.Source == "" {
		msg := "The source files could not be found. (HRESULT 0x800F081F)"
		res.OK, res.Error, res.ExitCode = false, &msg, "Failed"
		return res
	}
	names := []string{target.Name}
	if req.Action == plan.ActionInstall {
		names = append(names, inv.InstallClosure(target.Name)...)
		names = append(names, g.defaultSubs[key]...)
		if tool, ok := g.mgmtTools[key]; ok && req.IncludeManagementTools {
			names = append(names, tool)
		}
	} else {
		removed := inv.RemovalClosure(target.Name)
		for _, n := range append([]string{target.Name}, removed...) {
			if why := plan.Protect(req.Interface, n, nil); why != "" {
				g.t.Errorf("the plugin removed a session-critical feature: %s", why)
			}
		}
		names = append(names, removed...)
	}
	for _, n := range names {
		f := g.feature(req.Interface, n)
		pending := g.pendingOn[strings.ToLower(n)]
		f.State = g.rawState(req.Interface, req.Action, pending, req.RemovePayload)
		if pending {
			res.RestartNeeded = true
			g.facts.Reboot.ComponentBasedServicing = true
		}
	}
	res.Changed = names
	if g.breakWinRM != nil {
		g.breakWinRM(g.facts.WinRM)
	}
	return res
}

// rawState returns the state string iface reports after action.
//
// Author: Lars Krogh
// Version: 1.0.0
func (g *fakeGuest) rawState(iface facts.Interface, action plan.Action, pending, removePayload bool) string {
	type key struct {
		install, pending bool
	}
	states := map[facts.Interface]map[key]string{
		facts.InterfaceServerManager:   {{true, false}: "Installed", {true, true}: "InstallPending", {false, false}: "Available", {false, true}: "UninstallPending"},
		facts.InterfaceOptionalFeature: {{true, false}: "Enabled", {true, true}: "EnablePending", {false, false}: "Disabled", {false, true}: "DisablePending"},
		facts.InterfaceCapability:      {{true, false}: "Installed", {true, true}: "InstallPending", {false, false}: "NotPresent", {false, true}: "UninstallPending"},
	}
	if action == plan.ActionRemove && removePayload && !pending {
		if iface == facts.InterfaceOptionalFeature {
			return "DisabledWithPayloadRemoved"
		}
		return "Removed"
	}
	return states[iface][key{action == plan.ActionInstall, pending}]
}

// reboot advances the boot time, clears the restart indicators, and
// completes every pending operation that is not stuck.
//
// Author: Lars Krogh
// Version: 1.0.0
func (g *fakeGuest) reboot() {
	g.bootTime++
	stuck := false
	for iface := range g.inv {
		for i := range g.inv[iface] {
			f := &g.inv[iface][i]
			if g.stuck[strings.ToLower(f.Name)] {
				stuck = stuck || plan.Normalize(iface, f.State) == plan.StateInstallPending
				continue
			}
			switch plan.Normalize(iface, f.State) {
			case plan.StateInstallPending:
				f.State = g.rawState(iface, plan.ActionInstall, false, false)
			case plan.StateRemovePending:
				f.State = g.rawState(iface, plan.ActionRemove, false, false)
			}
		}
	}
	*g.facts.Reboot = facts.PendingReboot{ComponentBasedServicing: stuck}
}

// Upload stores the uploaded file.
//
// Returns:
//   - error: Reading r failed.
//
// Author: Lars Krogh
// Version: 1.0.0
func (g *fakeGuest) Upload(path string, r io.Reader, _ *os.FileInfo) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.files[path] = data
	return nil
}

// Download writes a stored file to w.
//
// Returns:
//   - error: The file does not exist.
//
// Author: Lars Krogh
// Version: 1.0.0
func (g *fakeGuest) Download(path string, w io.Writer) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	data, ok := g.files[path]
	if !ok {
		return fmt.Errorf("%s does not exist", path)
	}
	_, err := io.Copy(w, bytes.NewReader(data))
	return err
}

// UploadDir is not used by the plugin.
//
// Returns:
//   - error: Always.
//
// Author: Lars Krogh
// Version: 1.0.0
func (g *fakeGuest) UploadDir(string, string, []string) error {
	return fmt.Errorf("UploadDir is not supported")
}

// DownloadDir is not used by the plugin.
//
// Returns:
//   - error: Always.
//
// Author: Lars Krogh
// Version: 1.0.0
func (g *fakeGuest) DownloadDir(string, string, []string) error {
	return fmt.Errorf("DownloadDir is not supported")
}

// count returns how often the plugin ran action.
//
// Author: Lars Krogh
// Version: 1.0.0
func (g *fakeGuest) count(action string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	n := 0
	for _, a := range g.actions {
		if a == action {
			n++
		}
	}
	return n
}

// restarts returns how many restart commands the plugin ran.
//
// Author: Lars Krogh
// Version: 1.0.0
func (g *fakeGuest) restarts() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	n := 0
	for _, c := range g.commands {
		if strings.HasPrefix(c, "shutdown.exe /r") {
			n++
		}
	}
	return n
}
