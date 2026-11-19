package provisioner

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/larskroghdk/packer-plugin-windows-features/internal/facts"
	"github.com/larskroghdk/packer-plugin-windows-features/internal/plan"
	"github.com/larskroghdk/packer-plugin-windows-features/internal/report"
)

type outcome struct {
	report *report.Report
	err    error
	output string
}

// TestProvisionSkipsDependencyAlreadyInstalled checks that a role's default
// child is not serviced again when a previous request already installed it.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestProvisionSkipsDependencyAlreadyInstalled(t *testing.T) {
	g := newServerGuest(t)
	o := provision(t, g, map[string]any{"install": []string{"Web-Server", "Web-WebServer"}})
	if o.err != nil || g.count("Invoke") != 1 || !o.report.Validation.Passed || o.report.Summary.Installed != 2 {
		t.Fatalf("redundant child operation: error %v, operations %d, summary %+v", o.err, g.count("Invoke"), o.report.Summary)
	}
}

// provision prepares a provisioner with cfg, lets tune adjust it, and runs
// it against g with short restart waits.
//
// Returns:
//   - outcome: The report the run wrote, the error Provision returned and
//     the UI output.
//
// Author: Lars Krogh
// Version: 1.0.0
func provision(t *testing.T, g *fakeGuest, cfg map[string]any, tune ...func(*Provisioner)) outcome {
	t.Helper()
	path := filepath.Join(t.TempDir(), "report.json")
	raw := map[string]any{"report_path": path}
	for k, v := range cfg {
		raw[k] = v
	}
	p := Provisioner{timing: timing{probeInterval: time.Millisecond, settle: time.Millisecond}}
	if err := p.Prepare(raw); err != nil {
		t.Fatalf("Prepare: %v", err)
	}
	for _, f := range tune {
		f(&p)
	}
	var out bytes.Buffer
	ui := &packer.BasicUi{Reader: strings.NewReader(""), Writer: &out, ErrorWriter: &out}
	runErr := p.Provision(context.Background(), ui, g, map[string]any{"ConnType": "winrm"})
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the report was not written: %v\n%s", err, out.String())
	}
	var r report.Report
	if err := json.Unmarshal(data, &r); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	return outcome{report: &r, err: runErr, output: out.String()}
}

// request returns the report entry for the requested name.
//
// Author: Lars Krogh
// Version: 1.0.0
func request(t *testing.T, r *report.Report, name string) report.Request {
	t.Helper()
	for _, q := range r.Requests {
		if q.Requested == name {
			return q
		}
	}
	t.Fatalf("%s is not in the report", name)
	return report.Request{}
}

// TestProvisionServerInstallAndRemove installs IIS and removes the print
// role on Windows Server, then checks the verified states, the side effects,
// the connectivity check and the order of operations.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestProvisionServerInstallAndRemove(t *testing.T) {
	g := newServerGuest(t)
	o := provision(t, g, map[string]any{
		"install": []string{"web-server", "Web-Mgmt-Console"},
		"remove":  []string{"Print-Services"},
	})
	if o.err != nil {
		t.Fatalf("%v\n%s", o.err, o.output)
	}
	r := o.report
	if q := request(t, r, "web-server"); q.Status != "installed" || q.Name != "Web-Server" || q.Before != "available" || q.After != "installed" {
		t.Errorf("web-server: %+v", q)
	}
	if q := request(t, r, "Print-Services"); q.Status != "removed" || len(q.Related) != 1 || q.Related[0] != "Print-Server" {
		t.Errorf("Print-Services: %+v", q)
	}
	if g.state(facts.InterfaceServerManager, "Print-Server") != "Available" {
		t.Error("the print server role service was not removed with its role")
	}
	if len(g.ops) != 3 || g.ops[0].Action != plan.ActionRemove {
		t.Errorf("operations: %+v", g.ops)
	}
	names := map[string]bool{}
	for _, e := range r.SideEffects {
		names[e.Name] = true
	}
	if !names["Web-WebServer"] || !names["Web-Mgmt-Tools"] || !names["Print-Server"] {
		t.Errorf("side effects: %+v", r.SideEffects)
	}
	if !r.Validation.Passed || len(r.Connectivity) != 1 || !r.Connectivity[0].Passed {
		t.Errorf("validation %+v, connectivity %+v", r.Validation, r.Connectivity)
	}
	if r.System.Release != "Windows Server 2022" || r.System.FeatureInterface != "server_manager" {
		t.Errorf("system: %+v", r.System)
	}
}

// TestProvisionIsIdempotent runs the same configuration twice and checks
// that the second run changes nothing and reports every request compliant.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestProvisionIsIdempotent(t *testing.T) {
	g := newServerGuest(t)
	cfg := map[string]any{"install": []string{"Web-Server", "Telnet-Client"}, "remove": []string{"XPS-Viewer"}, "install_capabilities": []string{"OpenSSH.Server"}}
	if o := provision(t, g, cfg); o.err != nil {
		t.Fatal(o.err)
	}
	ops := len(g.ops)
	o := provision(t, g, cfg)
	if o.err != nil {
		t.Fatal(o.err)
	}
	if len(g.ops) != ops {
		t.Errorf("the second run invoked %d operations", len(g.ops)-ops)
	}
	if o.report.Summary.Compliant != 4 || len(o.report.Connectivity) != 0 || o.report.Reboot.Required {
		t.Errorf("second run: %+v, connectivity %+v", o.report.Summary, o.report.Connectivity)
	}
}

// TestProvisionDryRunChangesNothing checks that a dry run inventories and
// plans, including the dependencies an install adds, but invokes nothing.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestProvisionDryRunChangesNothing(t *testing.T) {
	g := newServerGuest(t)
	o := provision(t, g, map[string]any{"install": []string{"Web-Mgmt-Console"}, "dry_run": true})
	if o.err != nil {
		t.Fatal(o.err)
	}
	if g.count("Invoke") != 0 {
		t.Error("a dry run invoked an operation")
	}
	q := request(t, o.report, "Web-Mgmt-Console")
	if q.Status != "planned" || strings.Join(q.Related, ",") != "Web-Mgmt-Tools,Web-Server" {
		t.Errorf("planned request: %+v", q)
	}
	if o.report.Mode != report.ModeDryRun || o.report.Validation.Passed {
		t.Errorf("dry run report: %s %+v", o.report.Mode, o.report.Validation)
	}
}

// TestProvisionBlocksBeforeChanges checks that unavailable, protected and
// conflicting requests fail the build before any operation, even with
// fail_on_error false, and that ignore_unavailable only skips unavailable
// names.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestProvisionBlocksBeforeChanges(t *testing.T) {
	tests := map[string]struct {
		cfg    map[string]any
		status string
	}{
		"unknown feature":       {map[string]any{"install": []string{"Web-Server", "Web-Servr"}}, "unavailable"},
		"protected":             {map[string]any{"remove": []string{"PowerShell"}, "fail_on_error": false}, "protected"},
		"protected dependency":  {map[string]any{"remove": []string{"NET-Framework-45-Features"}}, "protected"},
		"conflict":              {map[string]any{"install": []string{"Web-Mgmt-Console"}, "remove": []string{"Web-Server"}}, "conflict"},
		"ambiguous capability":  {map[string]any{"install_capabilities": []string{"Language.Basic"}}, "unavailable"},
		"optional feature name": {map[string]any{"install": []string{"IIS-WebServerRole"}}, "unavailable"},
	}
	for name, tc := range tests {
		g := newServerGuest(t)
		o := provision(t, g, tc.cfg)
		if o.err == nil || !strings.Contains(o.err.Error(), "nothing was changed") {
			t.Errorf("%s: error %v", name, o.err)
		}
		if g.count("Invoke") != 0 {
			t.Errorf("%s: an operation ran", name)
		}
		found := false
		for _, q := range o.report.Requests {
			found = found || q.Status == tc.status
		}
		if !found {
			t.Errorf("%s: no request is %s: %+v", name, tc.status, o.report.Requests)
		}
	}
	g := newServerGuest(t)
	o := provision(t, g, map[string]any{"install": []string{"Web-Server", "Web-Servr"}, "ignore_unavailable": true})
	if o.err != nil || g.count("Invoke") != 1 || len(o.report.Warnings) == 0 || !o.report.Validation.Passed {
		t.Errorf("ignore_unavailable: %v, %d operations, warnings %v, validation %+v", o.err, g.count("Invoke"), o.report.Warnings, o.report.Validation)
	}
}

// TestProvisionPartialFailure checks that a failing operation does not stop
// the others, fails the build, and only warns with fail_on_error false, and
// that a source-related error carries a hint.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestProvisionPartialFailure(t *testing.T) {
	for _, failOnError := range []bool{true, false} {
		g := newServerGuest(t)
		o := provision(t, g, map[string]any{"install": []string{"NET-Framework-Core", "Telnet-Client"}, "fail_on_error": failOnError})
		if failOnError != (o.err != nil) {
			t.Errorf("fail_on_error %v: error %v", failOnError, o.err)
		}
		if q := request(t, o.report, "NET-Framework-Core"); q.Status != "failed" || !strings.Contains(q.Reason, "hint: the feature payload is not on the system") {
			t.Errorf("failed request: %+v", q)
		}
		if q := request(t, o.report, "Telnet-Client"); q.Status != "installed" {
			t.Errorf("the second operation: %+v", q)
		}
		if o.report.Validation.Passed {
			t.Error("validation passed with a failed request")
		}
	}
	g := newServerGuest(t)
	o := provision(t, g, map[string]any{"install": []string{"NET-Framework-Core"}, "source": `D:\sources\sxs`})
	if o.err != nil || g.ops[0].Source != `D:\sources\sxs` {
		t.Errorf("with source: %v, %+v", o.err, g.ops)
	}
}

// TestProvisionVerifiesState checks that an operation reporting success
// without changing the feature fails, and that a reported error is outweighed
// by a feature that reached the requested state.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestProvisionVerifiesState(t *testing.T) {
	g := newServerGuest(t)
	g.ignoreOp["telnet-client"] = true
	o := provision(t, g, map[string]any{"install": []string{"Telnet-Client"}})
	if o.err == nil {
		t.Fatal("an install that changed nothing passed")
	}
	if q := request(t, o.report, "Telnet-Client"); q.Status != "failed" || q.After != "available" {
		t.Errorf("unverified install: %+v", q)
	}
}

// TestProvisionTimeout checks that a hanging operation times out, stops the
// remaining operations, and fails the build.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestProvisionTimeout(t *testing.T) {
	g := newServerGuest(t)
	g.hangOp["telnet-client"] = true
	o := provision(t, g, map[string]any{"install": []string{"Telnet-Client", "Web-Server"}},
		func(p *Provisioner) { p.config.OperationTimeout = 50 * time.Millisecond })
	if o.err == nil {
		t.Fatal("a timed-out operation passed")
	}
	if q := request(t, o.report, "Telnet-Client"); q.Status != "failed" || !strings.Contains(q.Reason, "timed out") {
		t.Errorf("timed-out request: %+v", q)
	}
	if q := request(t, o.report, "Web-Server"); q.Status != "not_attempted" {
		t.Errorf("request after the timeout: %+v", q)
	}
}

// TestProvisionRestart checks that a pending install is reported without
// restart_if_required, and completed and re-verified with it.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestProvisionRestart(t *testing.T) {
	g := newServerGuest(t)
	g.pendingOn["hyper-v"] = true
	o := provision(t, g, map[string]any{"install": []string{"Hyper-V"}, "include_management_tools": true})
	if o.err != nil {
		t.Fatal(o.err)
	}
	if q := request(t, o.report, "Hyper-V"); q.Status != "pending_reboot" || !o.report.Reboot.Required || g.restarts() != 0 {
		t.Errorf("without restart_if_required: %+v, reboot %+v", q, o.report.Reboot)
	}
	if len(o.report.Reboot.PendingAfter) == 0 || !strings.Contains(o.output, "add a windows-restart provisioner") {
		t.Errorf("the pending restart is not reported: %+v", o.report.Reboot)
	}
	if g.state(facts.InterfaceServerManager, "RSAT-Hyper-V-Tools") != "Installed" {
		t.Error("include_management_tools did not install the management tools")
	}

	g = newServerGuest(t)
	g.pendingOn["hyper-v"] = true
	o = provision(t, g, map[string]any{"install": []string{"Hyper-V"}, "restart_if_required": true})
	if o.err != nil {
		t.Fatal(o.err)
	}
	if q := request(t, o.report, "Hyper-V"); q.Status != "installed" || g.restarts() != 1 || o.report.Reboot.Restarts != 1 {
		t.Errorf("with restart_if_required: %+v, reboot %+v", q, o.report.Reboot)
	}
	if len(o.report.Connectivity) != 2 || o.report.Reboot.Required || !o.report.Validation.Passed {
		t.Errorf("after restart: connectivity %+v, reboot %+v", o.report.Connectivity, o.report.Reboot)
	}
}

// TestProvisionRestartLimit checks that a feature that stays pending fails
// after the maximum number of restarts.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestProvisionRestartLimit(t *testing.T) {
	g := newServerGuest(t)
	g.pendingOn["hyper-v"] = true
	g.stuck["hyper-v"] = true
	o := provision(t, g, map[string]any{"install": []string{"Hyper-V"}, "restart_if_required": true})
	if o.err == nil || g.restarts() != maxRestarts {
		t.Fatalf("error %v after %d restarts", o.err, g.restarts())
	}
	if q := request(t, o.report, "Hyper-V"); q.Status != "failed" || !strings.Contains(q.Reason, "after 3 restarts") {
		t.Errorf("stuck request: %+v", q)
	}
}

// TestProvisionRestartsBeforeBlockedServicing checks that a pending
// servicing restart found before any change is performed first when
// restart_if_required is set, and only warned about otherwise.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestProvisionRestartsBeforeBlockedServicing(t *testing.T) {
	g := newServerGuest(t)
	g.facts.Reboot.ComponentBasedServicing = true
	o := provision(t, g, map[string]any{"install": []string{"Telnet-Client"}, "restart_if_required": true})
	if o.err != nil || g.restarts() != 1 || len(o.report.Reboot.PendingBefore) != 1 {
		t.Errorf("restart first: %v, %d restarts, %+v", o.err, g.restarts(), o.report.Reboot)
	}
	g = newServerGuest(t)
	g.facts.Reboot.ComponentBasedServicing = true
	o = provision(t, g, map[string]any{"install": []string{"Telnet-Client"}})
	if o.err != nil || g.restarts() != 0 || len(o.report.Warnings) != 1 {
		t.Errorf("no restart: %v, %d restarts, warnings %v", o.err, g.restarts(), o.report.Warnings)
	}
}

// TestProvisionDetectsWinRMRegression checks that a WinRM regression after
// the changes fails the build even with fail_on_error false.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestProvisionDetectsWinRMRegression(t *testing.T) {
	g := newServerGuest(t)
	g.breakWinRM = func(w *facts.WinRM) { w.Auth.Basic = false }
	o := provision(t, g, map[string]any{"install": []string{"Telnet-Client"}, "fail_on_error": false})
	if o.err == nil || !strings.Contains(o.err.Error(), "Basic authentication was disabled") {
		t.Errorf("regression: %v", o.err)
	}
	if o.report.Validation.Passed || len(o.report.Connectivity) != 1 || o.report.Connectivity[0].Passed {
		t.Errorf("regression report: %+v %+v", o.report.Validation, o.report.Connectivity)
	}
}

// TestProvisionRefusesRemovalWithoutWinRMFacts checks that removals are
// refused when the WinRM configuration could not be detected.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestProvisionRefusesRemovalWithoutWinRMFacts(t *testing.T) {
	g := newServerGuest(t)
	g.facts.WinRM = nil
	o := provision(t, g, map[string]any{"remove": []string{"XPS-Viewer"}})
	if o.err == nil || g.count("Invoke") != 0 {
		t.Errorf("removal without WinRM facts: %v, %d operations", o.err, g.count("Invoke"))
	}
}

// TestProvisionClient enables optional features and a capability on
// Windows 11, checks the DISM options, the protection of .NET Framework 4,
// and the warning for a server-only option.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestProvisionClient(t *testing.T) {
	g := newClientGuest(t)
	o := provision(t, g, map[string]any{
		"install":                  []string{"NetFx3", "IIS-WebServerRole"},
		"remove":                   []string{"MicrosoftWindowsPowerShellV2Root"},
		"install_capabilities":     []string{"OpenSSH.Server"},
		"source":                   `D:\sources\sxs`,
		"limit_access":             true,
		"include_management_tools": true,
	})
	if o.err != nil {
		t.Fatalf("%v\n%s", o.err, o.output)
	}
	if o.report.System.FeatureInterface != "optional_feature" || o.report.System.Family != "client" {
		t.Errorf("system: %+v", o.report.System)
	}
	if q := request(t, o.report, "OpenSSH.Server"); q.Status != "installed" || q.Name != "OpenSSH.Server~~~~0.0.1.0" || q.Interface != "capability" {
		t.Errorf("capability: %+v", q)
	}
	for _, op := range g.ops {
		if op.Action == plan.ActionInstall && (!op.LimitAccess || op.Source == "" || op.IncludeManagementTools) {
			t.Errorf("install options: %+v", op)
		}
	}
	if len(o.report.Warnings) != 1 || !strings.Contains(o.report.Warnings[0], "include_management_tools") {
		t.Errorf("warnings: %v", o.report.Warnings)
	}
	o = provision(t, newClientGuest(t), map[string]any{"remove": []string{"NetFx4-AdvSrvs"}})
	if o.err == nil || request(t, o.report, "NetFx4-AdvSrvs").Status != "protected" {
		t.Errorf(".NET Framework 4 removal: %v", o.err)
	}
	o = provision(t, newClientGuest(t), map[string]any{"install": []string{"Web-Server"}})
	if o.err == nil || request(t, o.report, "Web-Server").Status != "unavailable" {
		t.Errorf("a Server Manager name on a client: %v", o.err)
	}
}

// TestProvisionWithoutCapabilities checks that capability requests are
// unavailable on a guest without the capability cmdlets.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestProvisionWithoutCapabilities(t *testing.T) {
	g := newServerGuest(t)
	g.facts.Servicing.Capabilities = false
	o := provision(t, g, map[string]any{"install_capabilities": []string{"OpenSSH.Server"}})
	if o.err == nil || request(t, o.report, "OpenSSH.Server").Status != "unavailable" {
		t.Errorf("capabilities without cmdlets: %v", o.err)
	}
}

// TestProvisionRefusesUnsafeSessions checks that SSH, non-elevated and
// 32-bit sessions and unsupported systems are refused before any change.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestProvisionRefusesUnsafeSessions(t *testing.T) {
	cases := map[string]func(*fakeGuest){
		"not elevated":     func(g *fakeGuest) { g.facts.Session.IsElevated = false },
		"32-bit":           func(g *fakeGuest) { g.facts.OS.Is64BitProcess = false },
		"old build":        func(g *fakeGuest) { g.facts.OS.Build = 10240 },
		"no ServerManager": func(g *fakeGuest) { g.facts.Servicing.ServerManager = false },
	}
	for name, mutate := range cases {
		g := newServerGuest(t)
		mutate(g)
		o := provision(t, g, map[string]any{"install": []string{"Telnet-Client"}})
		if o.err == nil || g.count("Invoke") != 0 || g.count("Inventory") != 0 {
			t.Errorf("%s: error %v, actions %v", name, o.err, g.actions)
		}
	}
	var p Provisioner
	if err := p.Prepare(map[string]any{"install": []string{"Telnet-Client"}}); err != nil {
		t.Fatal(err)
	}
	ui := &packer.BasicUi{Reader: strings.NewReader(""), Writer: &bytes.Buffer{}, ErrorWriter: &bytes.Buffer{}}
	if err := p.Provision(context.Background(), ui, newServerGuest(t), map[string]any{"ConnType": "ssh"}); err == nil {
		t.Error("an SSH build was accepted")
	}
}

// TestProvisionUnknownRelease checks that a newer build is managed with a
// warning.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestProvisionUnknownRelease(t *testing.T) {
	g := newServerGuest(t)
	g.facts.OS.Build = 26300
	o := provision(t, g, map[string]any{"install": []string{"Telnet-Client"}})
	if o.err != nil || o.report.System.KnownRelease || len(o.report.Warnings) != 1 {
		t.Errorf("unknown release: %v, %+v, %v", o.err, o.report.System, o.report.Warnings)
	}
}

// TestProvisionRemovePayload checks that remove_payload reaches the guest
// and a payload-removed feature counts as removed.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestProvisionRemovePayload(t *testing.T) {
	g := newServerGuest(t)
	o := provision(t, g, map[string]any{"remove": []string{"XPS-Viewer"}, "remove_payload": true})
	if o.err != nil || !g.ops[0].RemovePayload || request(t, o.report, "XPS-Viewer").After != "removed" {
		t.Errorf("remove_payload: %v, %+v, %+v", o.err, g.ops, o.report.Requests)
	}
}
