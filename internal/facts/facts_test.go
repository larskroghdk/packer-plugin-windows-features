package facts

import (
	"strings"
	"testing"
)

// factsFor returns facts for an OS with the given product type, installation
// type and build, with every servicing interface available.
//
// Author: Lars Krogh
// Version: 1.0.0
func factsFor(productType int, installation string, build int) *Facts {
	return &Facts{
		OS:        OS{Major: 10, Build: build, ProductType: productType, InstallationType: installation, EditionID: "x", Is64BitProcess: true},
		Servicing: Servicing{ServerManager: productType != ProductTypeWorkstation, OptionalFeatures: true, Capabilities: true},
	}
}

// TestClassify checks the release name, family, installation and feature
// interface of every supported release, and of unknown newer builds.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestClassify(t *testing.T) {
	tests := []struct {
		productType  int
		installation string
		build        int
		name         string
		iface        Interface
		known        bool
	}{
		{ProductTypeServer, "Server", 14393, "Windows Server 2016", InterfaceServerManager, true},
		{ProductTypeServer, "Server Core", 17763, "Windows Server 2019 (Server Core)", InterfaceServerManager, true},
		{ProductTypeServer, "Server", 20348, "Windows Server 2022", InterfaceServerManager, true},
		{ProductTypeServer, "Server", 26100, "Windows Server 2025", InterfaceServerManager, true},
		{ProductTypeDomainController, "Server", 26100, "Windows Server 2025", InterfaceServerManager, true},
		{ProductTypeServer, "Server", 26400, "Windows Server build 26400", InterfaceServerManager, false},
		{ProductTypeWorkstation, "Client", 19045, "Windows 10 version 22H2", InterfaceOptionalFeature, true},
		{ProductTypeWorkstation, "Client", 26100, "Windows 11 version 24H2", InterfaceOptionalFeature, true},
		{ProductTypeWorkstation, "Client", 26200, "Windows 11 version 25H2", InterfaceOptionalFeature, true},
		{ProductTypeWorkstation, "Client", 22000, "Windows 11 build 22000", InterfaceOptionalFeature, false},
		{ProductTypeWorkstation, "Client", 18363, "Windows 10 build 18363", InterfaceOptionalFeature, false},
	}
	for _, tc := range tests {
		s, err := Classify(factsFor(tc.productType, tc.installation, tc.build))
		if err != nil {
			t.Errorf("%d/%d: %v", tc.productType, tc.build, err)
			continue
		}
		if s.Name != tc.name || s.FeatureInterface != tc.iface || s.Known != tc.known {
			t.Errorf("%d/%d: got %+v", tc.productType, tc.build, s)
		}
	}
	dc, _ := Classify(factsFor(ProductTypeDomainController, "Server", 20348))
	if !dc.DomainController || dc.Family != FamilyServer {
		t.Errorf("domain controller: %+v", dc)
	}
}

// TestClassifyRejects checks every reason a guest is refused.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestClassifyRejects(t *testing.T) {
	noServerManager := factsFor(ProductTypeServer, "Server", 20348)
	noServerManager.Servicing.ServerManager = false
	noDism := factsFor(ProductTypeWorkstation, "Client", 26100)
	noDism.Servicing.OptionalFeatures = false
	old := factsFor(ProductTypeServer, "Server", 9600)
	win81 := factsFor(ProductTypeWorkstation, "Client", 9600)
	win81.OS.Major, win81.OS.Minor = 6, 3
	tests := map[string]struct {
		f    *Facts
		want string
	}{
		"Windows 8.1":       {win81, "unsupported Windows version"},
		"old build":         {old, "unsupported Windows build"},
		"Nano Server":       {factsFor(ProductTypeServer, "Nano Server", 14393), "Nano Server"},
		"unknown install":   {factsFor(ProductTypeServer, "Server Foo", 20348), "installation type"},
		"unknown client":    {factsFor(ProductTypeWorkstation, "Server", 26100), "installation type"},
		"unknown product":   {factsFor(9, "Server", 20348), "product type"},
		"no ServerManager":  {noServerManager, "Get-WindowsFeature"},
		"no DISM on client": {noDism, "Get-WindowsOptionalFeature"},
	}
	for name, tc := range tests {
		if _, err := Classify(tc.f); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error %v, want %q", name, err, tc.want)
		}
	}
}

// TestParse checks decoding of a Detect result and rejection of incomplete
// facts.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestParse(t *testing.T) {
	data := []byte(`{"os":{"product_name":"Windows Server 2022 Datacenter","major":10,"build":20348,"product_type":3,"installation_type":"Server","is_64bit_process":true},
		"session":{"user_name":"BUILD\\packer","is_elevated":true},
		"servicing":{"server_manager":true,"optional_features":true,"capabilities":true},
		"reboot":{"component_based_servicing":true,"windows_update":false,"pending_file_rename":true,"computer_rename":false},
		"winrm":null,"errors":["winrm: access denied"]}`)
	f, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if !f.Servicing.ServerManager || f.WinRM != nil || len(f.Errors) != 1 || !f.Reboot.ComponentBasedServicing {
		t.Errorf("parsed: %+v", f)
	}
	if _, err := Parse([]byte(`{"os":{"build":0}}`)); err == nil {
		t.Error("incomplete facts were accepted")
	}
	if _, err := Parse([]byte(`{`)); err == nil {
		t.Error("invalid JSON was accepted")
	}
}

// TestPendingReboot checks the reasons and which indicators block
// servicing, including a nil indicator set.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestPendingReboot(t *testing.T) {
	var unknown *PendingReboot
	if unknown.BlocksServicing() || unknown.Reasons() != nil {
		t.Error("unknown restart state must neither block nor have reasons")
	}
	renames := &PendingReboot{PendingFileRename: true, ComputerRename: true}
	if renames.BlocksServicing() || len(renames.Reasons()) != 2 {
		t.Errorf("file renames: %v", renames.Reasons())
	}
	for _, p := range []*PendingReboot{{ComponentBasedServicing: true}, {WindowsUpdate: true}} {
		if !p.BlocksServicing() || len(p.Reasons()) != 1 {
			t.Errorf("%+v must block servicing", *p)
		}
	}
}

// TestParseProbe checks decoding of a Probe result and that a missing boot
// time is rejected.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestParseProbe(t *testing.T) {
	p, err := ParseProbe([]byte(`{"winrm":null,"session":{"is_elevated":true},"boot_time":"2026-01-01T00:00:00Z","reboot":{"component_based_servicing":true},"error":"x"}`))
	if err != nil || p.Error == nil || !p.Reboot.ComponentBasedServicing {
		t.Errorf("probe: %+v, %v", p, err)
	}
	if _, err := ParseProbe([]byte(`{"session":{}}`)); err == nil {
		t.Error("a probe without boot time was accepted")
	}
}

// healthyWinRM returns a typical Packer WinRM configuration.
//
// Author: Lars Krogh
// Version: 1.0.0
func healthyWinRM() *WinRM {
	return &WinRM{
		ServiceStatus:    "Running",
		ServiceStart:     "Automatic",
		Dependencies:     []string{"HTTP", "RPCSS"},
		Listeners:        []Listener{{Transport: "HTTP", Address: "*", Port: 5985, Enabled: true}},
		Auth:             WinRMAuth{Basic: true, Negotiate: true, Kerberos: true},
		AllowUnencrypted: true,
	}
}

// TestWinRMRegressions checks that every kind of regression is reported and
// that additions are not.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestWinRMRegressions(t *testing.T) {
	probe := func(mutate func(*WinRM)) *Probe {
		w := healthyWinRM()
		if mutate != nil {
			mutate(w)
		}
		return &Probe{WinRM: w, Session: Session{IsElevated: true}, BootTime: "x"}
	}
	if got := WinRMRegressions(healthyWinRM(), probe(nil)); len(got) != 0 {
		t.Errorf("unchanged WinRM reported regressions: %v", got)
	}
	added := probe(func(w *WinRM) {
		w.Listeners = append(w.Listeners, Listener{Transport: "HTTPS", Address: "*", Port: 5986, Enabled: true})
		w.Auth.CredSSP = true
	})
	if got := WinRMRegressions(healthyWinRM(), added); len(got) != 0 {
		t.Errorf("additions reported as regressions: %v", got)
	}
	probeError := "boom"
	regressions := map[string]*Probe{
		"stopped":          probe(func(w *WinRM) { w.ServiceStatus = "Stopped" }),
		"start type":       probe(func(w *WinRM) { w.ServiceStart = "Manual" }),
		"listener gone":    probe(func(w *WinRM) { w.Listeners = nil }),
		"listener off":     probe(func(w *WinRM) { w.Listeners[0].Enabled = false }),
		"listener address": probe(func(w *WinRM) { w.Listeners[0].Address = "127.0.0.1" }),
		"basic disabled":   probe(func(w *WinRM) { w.Auth.Basic = false }),
		"not elevated":     {WinRM: healthyWinRM(), Session: Session{IsElevated: false}, BootTime: "x"},
		"unreadable":       {Session: Session{IsElevated: true}, BootTime: "x"},
		"probe error":      {WinRM: healthyWinRM(), Session: Session{IsElevated: true}, BootTime: "x", Error: &probeError},
		"unencrypted off":  probe(func(w *WinRM) { w.AllowUnencrypted = false }),
	}
	for name, p := range regressions {
		if got := WinRMRegressions(healthyWinRM(), p); len(got) == 0 {
			t.Errorf("%s: no regression reported", name)
		}
	}
	if got := WinRMRegressions(nil, &Probe{Session: Session{IsElevated: true}, BootTime: "x"}); len(got) != 0 {
		t.Errorf("unknown baseline must only check elevation and errors, got %v", got)
	}
}
