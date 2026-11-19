package report

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/larskroghdk/packer-plugin-windows-features/internal/facts"
	"github.com/larskroghdk/packer-plugin-windows-features/internal/plan"
)

// TestRunFailureInvalidatesCompliantPlan prevents successful validation when
// orchestration fails after the requested features were found compliant.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestRunFailureInvalidatesCompliantPlan(t *testing.T) {
	inv, _ := plan.NewInventory(facts.InterfaceServerManager, []plan.Feature{{Name: "Web-Server", State: "Installed"}})
	p := plan.Build([]plan.Request{{Name: "Web-Server", Kind: plan.KindFeature, Action: plan.ActionInstall}}, facts.InterfaceServerManager, plan.Inventories{facts.InterfaceServerManager: inv})
	r := Build(Input{Plan: p, Mode: ModeApply, RunError: errors.New("restart command failed")})
	if r.Validation.Passed || r.Error != "restart command failed" {
		t.Fatalf("run failure was lost: %+v", r.Validation)
	}
}

// TestBuildWithoutPlan checks that a run that failed early still produces a
// valid report with empty lists and a failed validation.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestBuildWithoutPlan(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r := Build(Input{Plugin: "p", StartedAt: start, FinishedAt: start.Add(1500 * time.Millisecond), Mode: ModeApply})
	data, err := r.JSON()
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"requests": []`, `"warnings": []`, `"pending_before": []`, `"duration_seconds": 1.5`, `"passed": false`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("report lacks %s:\n%s", want, data)
		}
	}
}

// TestBuildSummary checks counts, validation, system and summary lines for
// a plan with an installed, a pending and an unavailable request.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestBuildSummary(t *testing.T) {
	inv, _ := plan.NewInventory(facts.InterfaceServerManager, []plan.Feature{{Name: "Web-Server", State: "Installed"}, {Name: "Hyper-V", State: "InstallPending"}})
	p := plan.Build([]plan.Request{
		{Name: "Web-Server", Kind: plan.KindFeature, Action: plan.ActionInstall},
		{Name: "Hyper-V", Kind: plan.KindFeature, Action: plan.ActionInstall},
		{Name: "Nope", Kind: plan.KindFeature, Action: plan.ActionInstall},
	}, facts.InterfaceServerManager, plan.Inventories{facts.InterfaceServerManager: inv})
	f := &facts.Facts{OS: facts.OS{Major: 10, Build: 20348, EditionID: "ServerStandard", Architecture: "AMD64"}}
	sys := &facts.System{Name: "Windows Server 2022", Family: facts.FamilyServer, Installation: "server_desktop", FeatureInterface: facts.InterfaceServerManager, Known: true}
	in := Input{Mode: ModeApply, Facts: f, System: sys, Plan: p, Reboot: Reboot{Required: true, Reason: "x"}, StartedAt: time.Now(), FinishedAt: time.Now()}
	r := Build(in)
	if r.Summary.Requested != 3 || r.Summary.Compliant != 1 || r.Summary.PendingReboot != 1 || r.Summary.Unavailable != 1 {
		t.Errorf("summary: %+v", r.Summary)
	}
	if r.Validation.Passed || len(r.Validation.Failing) != 1 {
		t.Errorf("validation: %+v", r.Validation)
	}
	in.IgnoreUnavailable = true
	if r := Build(in); !r.Validation.Passed {
		t.Errorf("ignored unavailable request failed validation: %+v", r.Validation)
	}
	if r.System.Release != "Windows Server 2022" || r.System.FeatureInterface != "server_manager" || r.System.Version != "10.0.20348.0" {
		t.Errorf("system: %+v", r.System)
	}
	lines := strings.Join(r.SummaryLines(), "\n")
	for _, want := range []string{"Windows Server 2022", "pending_reboot", "add a windows-restart provisioner", "Validation:  failed", "Duration:"} {
		if !strings.Contains(lines, want) {
			t.Errorf("summary lacks %q:\n%s", want, lines)
		}
	}
	data, _ := r.JSON()
	var decoded Report
	if err := json.Unmarshal(data, &decoded); err != nil || decoded.SchemaVersion != SchemaVersion {
		t.Errorf("round trip: %v", err)
	}
}

// TestDryRunNeverValidates checks that planned changes in a dry run fail
// validation, because nothing was applied.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestDryRunNeverValidates(t *testing.T) {
	inv, _ := plan.NewInventory(facts.InterfaceOptionalFeature, []plan.Feature{{Name: "NetFx3", State: "Disabled"}})
	p := plan.Build([]plan.Request{{Name: "NetFx3", Kind: plan.KindFeature, Action: plan.ActionInstall}}, facts.InterfaceOptionalFeature, plan.Inventories{facts.InterfaceOptionalFeature: inv})
	r := Build(Input{Mode: ModeDryRun, Plan: p, StartedAt: time.Now(), FinishedAt: time.Now()})
	if r.Validation.Passed || r.Summary.Planned != 1 {
		t.Errorf("dry run: %+v %+v", r.Validation, r.Summary)
	}
}
