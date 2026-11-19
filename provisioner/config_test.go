package provisioner

import (
	"strings"
	"testing"
	"time"

	"github.com/larskroghdk/packer-plugin-windows-features/internal/plan"
)

// prepareConfig runs Config.prepare on raw.
//
// Returns:
//   - Config: The prepared configuration.
//   - error: The validation errors.
//
// Author: Lars Krogh
// Version: 1.0.0
func prepareConfig(raw map[string]any) (Config, error) {
	var c Config
	err := c.prepare(raw)
	return c, err
}

// TestConfigDefaults checks the defaults of a minimal configuration.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestConfigDefaults(t *testing.T) {
	c, err := prepareConfig(map[string]any{"packer_build_name": "hyperv.server 2022", "install": []string{"Web-Server"}})
	if err != nil {
		t.Fatal(err)
	}
	if c.DryRun || c.RestartIfRequired || c.IncludeManagementTools || c.IgnoreUnavailable || !c.failOnError() {
		t.Errorf("defaults: %+v", c)
	}
	if c.RestartTimeout != 30*time.Minute || c.OperationTimeout != 60*time.Minute {
		t.Errorf("timeouts: %s %s", c.RestartTimeout, c.OperationTimeout)
	}
	if c.ReportPath != "windows-features-report-hyperv.server_2022.json" {
		t.Errorf("report_path default %q", c.ReportPath)
	}
	c, err = prepareConfig(map[string]any{"install": []string{"x"}, "fail_on_error": false, "restart_timeout": "5m", "operation_timeout": "2h"})
	if err != nil || c.failOnError() || c.RestartTimeout != 5*time.Minute || c.OperationTimeout != 2*time.Hour {
		t.Errorf("explicit values: %+v, %v", c, err)
	}
}

// TestConfigRequests checks that every list becomes requests of the right
// kind and action, removals first.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestConfigRequests(t *testing.T) {
	c, err := prepareConfig(map[string]any{
		"install":              []string{"Web-Server"},
		"remove":               []string{"Print-Server"},
		"install_capabilities": []string{"OpenSSH.Server~~~~0.0.1.0"},
		"remove_capabilities":  []string{"Browser.InternetExplorer"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []plan.Request{
		{Name: "Print-Server", Kind: plan.KindFeature, Action: plan.ActionRemove},
		{Name: "Web-Server", Kind: plan.KindFeature, Action: plan.ActionInstall},
		{Name: "Browser.InternetExplorer", Kind: plan.KindCapability, Action: plan.ActionRemove},
		{Name: "OpenSSH.Server~~~~0.0.1.0", Kind: plan.KindCapability, Action: plan.ActionInstall},
	}
	got := c.requests()
	if len(got) != len(want) {
		t.Fatalf("requests: %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("request %d: %+v, want %+v", i, got[i], want[i])
		}
	}
	if !c.hasKind(plan.KindCapability) || !c.hasKind(plan.KindFeature) {
		t.Error("hasKind")
	}
}

// TestConfigRejects checks that invalid and contradictory configurations
// fail Prepare with a message naming the option.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestConfigRejects(t *testing.T) {
	tests := map[string]struct {
		raw  map[string]any
		want string
	}{
		"nothing to do":         {map[string]any{}, "nothing to do"},
		"install and remove":    {map[string]any{"install": []string{"Web-Server"}, "remove": []string{"web-server"}}, "remove[0]"},
		"duplicate":             {map[string]any{"install": []string{"Web-Server", "WEB-SERVER"}}, "install[1]"},
		"capability twice":      {map[string]any{"install_capabilities": []string{"A"}, "remove_capabilities": []string{"a"}}, "remove_capabilities[0]"},
		"empty name":            {map[string]any{"install": []string{""}}, "name is empty"},
		"spaces":                {map[string]any{"install": []string{" Web-Server"}}, "spaces"},
		"wildcard":              {map[string]any{"install": []string{"Web-*"}}, "wildcard"},
		"quote":                 {map[string]any{"install": []string{"Web'Server"}}, "quote"},
		"capability as feature": {map[string]any{"install": []string{"OpenSSH.Server~~~~0.0.1.0"}}, "install_capabilities"},
		"limit without source":  {map[string]any{"install": []string{"NetFx3"}, "limit_access": true}, "limit_access"},
		"payload without list":  {map[string]any{"install": []string{"NetFx3"}, "remove_payload": true}, "remove_payload"},
		"subfeatures no list":   {map[string]any{"remove": []string{"NetFx3"}, "include_all_subfeatures": true}, "include_all_subfeatures"},
		"multi-line source":     {map[string]any{"install": []string{"NetFx3"}, "source": "a\nb"}, "source"},
		"short restart":         {map[string]any{"install": []string{"x"}, "restart_timeout": "30s"}, "restart_timeout"},
		"short operation":       {map[string]any{"install": []string{"x"}, "operation_timeout": "10s"}, "operation_timeout"},
		"unknown option":        {map[string]any{"install": []string{"x"}, "reboot_if_required": true}, "reboot_if_required"},
	}
	for name, tc := range tests {
		_, err := prepareConfig(tc.raw)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: error %v, want it to mention %q", name, err, tc.want)
		}
	}
}

// TestConfigSpec checks that the generated HCL2 spec covers every option.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestConfigSpec(t *testing.T) {
	spec := new(Provisioner).ConfigSpec()
	for _, name := range []string{"install", "remove", "install_capabilities", "remove_capabilities", "include_management_tools", "include_all_subfeatures",
		"source", "limit_access", "remove_payload", "ignore_unavailable", "dry_run", "fail_on_error", "restart_if_required", "restart_timeout",
		"operation_timeout", "report_path"} {
		if _, ok := spec[name]; !ok {
			t.Errorf("the HCL2 spec has no %s; run make generate", name)
		}
	}
}

// TestWithHint checks that known servicing errors get a hint and others are
// unchanged.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestWithHint(t *testing.T) {
	if got := withHint("The source files could not be found. (HRESULT 0x800F081F)"); !strings.Contains(got, "set source") {
		t.Errorf("source hint: %s", got)
	}
	if got := withHint("Error: 0x800f0954"); !strings.Contains(got, "WSUS") {
		t.Errorf("WSUS hint: %s", got)
	}
	if got := withHint("something else"); got != "something else" {
		t.Errorf("unknown error changed: %s", got)
	}
}
