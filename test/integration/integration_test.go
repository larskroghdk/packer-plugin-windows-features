//go:build integration

// Package integration exercises actual Windows servicing through Packer.
// Tests require an explicitly configured disposable Windows guest.
package integration

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/larskroghdk/packer-plugin-windows-features/internal/report"
)

// TestServicingCycle plans, installs, checks idempotency, and removes an
// initially absent feature on a disposable guest. No guest runs by default.
//
// Author: Lars Krogh
// Version: 1.0.0
func TestServicingCycle(t *testing.T) {
	if os.Getenv("WF_TEST_HOST") == "" {
		t.Skip("set WF_TEST_HOST, WF_TEST_PASSWORD and WF_TEST_FEATURE for a disposable Windows guest")
	}
	if os.Getenv("WF_TEST_PASSWORD") == "" || os.Getenv("WF_TEST_FEATURE") == "" {
		t.Fatal("WF_TEST_PASSWORD and WF_TEST_FEATURE are required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Minute)
	defer cancel()
	dir := t.TempDir()
	binary := filepath.Join(dir, "packer-plugin-windows-features")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	env := append(os.Environ(), "PACKER_PLUGIN_PATH="+filepath.Join(dir, "plugins"),
		"PKR_VAR_host="+os.Getenv("WF_TEST_HOST"), "PKR_VAR_password="+os.Getenv("WF_TEST_PASSWORD"),
		"PKR_VAR_feature="+os.Getenv("WF_TEST_FEATURE"))
	for _, option := range []struct{ key, variable string }{{"WF_TEST_USERNAME", "username"}, {"WF_TEST_USE_SSL", "use_ssl"}, {"WF_TEST_INSECURE", "insecure"}} {
		if value := os.Getenv(option.key); value != "" {
			env = append(env, "PKR_VAR_"+option.variable+"="+value)
		}
	}
	command := func(name string, args ...string) {
		t.Helper()
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.Env = env
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%s failed: %v\n%s", name, err, output)
		}
	}
	command("go", "build", "-ldflags=-X main.versionPrerelease=", "-o", binary, "../..")
	command("packer", "plugins", "install", "--path", binary, "github.com/larskroghdk/windows-features")
	command("packer", "init", ".")
	build := func(name string, dryRun, remove bool) report.Report {
		t.Helper()
		path := filepath.Join(dir, name+".json")
		args := []string{"build", "-color=false", "-var", "report_path=" + filepath.ToSlash(path)}
		if !dryRun {
			args = append(args, "-var", "dry_run=false")
		}
		if remove {
			args = append(args, "-var", "remove=true")
		}
		command("packer", append(args, ".")...)
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var result report.Report
		if err := json.Unmarshal(data, &result); err != nil {
			t.Fatal(err)
		}
		t.Logf("%s: %s build %s: %+v", name, result.System.Release, result.System.Version, result.Summary)
		return result
	}
	before := build("plan", true, false)
	if before.Summary.Planned != 1 {
		t.Fatal("choose an available, initially absent feature; the suite will not remove a pre-existing feature")
	}
	if os.Getenv("WF_TEST_ALLOW_CHANGES") != "yes" {
		t.Log("dry run passed; set WF_TEST_ALLOW_CHANGES=yes to install and remove the feature")
		return
	}
	first := build("install", false, false)
	if !first.Validation.Passed || first.Summary.Installed != 1 || first.Reboot.Required {
		t.Fatalf("install did not complete: %+v", first)
	}
	second := build("idempotent", false, false)
	if !second.Validation.Passed || second.Summary.Compliant != 1 || second.Summary.Installed != 0 {
		t.Fatalf("second run was not idempotent: %+v", second.Summary)
	}
	removed := build("remove", false, true)
	if !removed.Validation.Passed || removed.Summary.Removed != 1 || removed.Reboot.Required {
		t.Fatalf("removal did not complete: %+v", removed)
	}
}
