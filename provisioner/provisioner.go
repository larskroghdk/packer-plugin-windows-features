// Package provisioner implements the windows-features Packer provisioner: it
// uploads the guest scripts, detects the OS and its servicing interfaces,
// plans the requested installs and removals against the guest's inventory,
// applies only what is needed, verifies the result by reading the inventory
// again, checks WinRM, optionally restarts, and writes the report.
package provisioner

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/hashicorp/hcl/v2/hcldec"
	"github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/uuid"
	"github.com/larskroghdk/packer-plugin-windows-features/internal/facts"
	"github.com/larskroghdk/packer-plugin-windows-features/internal/plan"
	"github.com/larskroghdk/packer-plugin-windows-features/internal/report"
)

var Version = "dev"

// Provisioner is the windows-features provisioner registered with Packer.
type Provisioner struct {
	config Config
	now    func() time.Time
	timing timing
}

// ConfigSpec returns the HCL2 specification of Config.
//
// Author: Lars Krogh
// Version: 1.0.0
func (p *Provisioner) ConfigSpec() hcldec.ObjectSpec { return p.config.FlatMapstructure().HCL2Spec() }

// Prepare validates the configuration.
//
// Returns:
//   - error: The configuration is invalid.
//
// Author: Lars Krogh
// Version: 1.0.0
func (p *Provisioner) Prepare(raws ...any) error {
	if p.now == nil {
		p.now = time.Now
	}
	if p.timing == (timing{}) {
		p.timing = defaultTiming
	}
	return p.config.prepare(raws...)
}

// Provision brings the guest's features into the requested state over comm.
// The report is written whether or not the run succeeds, and the guest
// working directory is removed.
//
// Parameters:
//   - generatedData: Builder data. ConnType must be winrm when it is set.
//
// Returns:
//   - error: The run failed, or the report could not be written.
//
// Author: Lars Krogh
// Version: 1.0.0
func (p *Provisioner) Provision(ctx context.Context, ui packer.Ui, comm packer.Communicator, generatedData map[string]any) error {
	if connType, _ := generatedData["ConnType"].(string); connType != "" && connType != "winrm" {
		return fmt.Errorf("windows-features requires the WinRM communicator, the build uses %q", connType)
	}
	r := &run{
		cfg:     &p.config,
		ui:      ui,
		remote:  newRemote(comm, ui, uuid.TimeOrderedUUID()),
		now:     p.now,
		timing:  p.timing,
		started: p.now(),
	}
	runErr := r.execute(ctx)
	cleanupCtx, cancelCleanup := context.WithTimeout(context.WithoutCancel(ctx), time.Minute)
	defer cancelCleanup()
	if err := r.remote.cleanup(cleanupCtx); err != nil {
		r.warn("could not remove the guest working directory %s: %v", r.remote.dir, err)
	}
	result := r.report(runErr)
	for _, line := range result.SummaryLines() {
		ui.Say(line)
	}
	if err := writeReport(p.config.ReportPath, result); err != nil {
		ui.Error(fmt.Sprintf("Could not write the feature report: %v", err))
		if runErr == nil {
			runErr = err
		}
	} else {
		ui.Say("Report:      " + p.config.ReportPath)
	}
	return runErr
}

// writeReport writes the report as JSON to path, creating missing
// directories.
//
// Returns:
//   - error: The report cannot be encoded or written.
//
// Author: Lars Krogh
// Version: 1.0.0
func writeReport(path string, result *report.Report) error {
	data, err := result.JSON()
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0o644)
}

type run struct {
	cfg       *Config
	ui        packer.Ui
	remote    *remote
	now       func() time.Time
	timing    timing
	started   time.Time
	facts     *facts.Facts
	system    *facts.System
	plan      *plan.Plan
	lastProbe *facts.Probe
	checks    []report.Check
	reboot    report.Reboot
	warnings  []string
}

type operationRequest struct {
	ID                     string          `json:"id"`
	Interface              facts.Interface `json:"interface"`
	Action                 plan.Action     `json:"action"`
	Name                   string          `json:"name"`
	IncludeManagementTools bool            `json:"include_management_tools"`
	IncludeAllSubFeatures  bool            `json:"include_all_subfeatures"`
	Source                 string          `json:"source"`
	LimitAccess            bool            `json:"limit_access"`
	RemovePayload          bool            `json:"remove_payload"`
}

// warn records a warning for the report and shows it in the Packer UI.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *run) warn(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	r.warnings = append(r.warnings, msg)
	r.ui.Error("Warning: " + msg)
}

// report builds the report from the run's current state and includes runErr
// in final validation when orchestration failed.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *run) report(runErr error) *report.Report {
	mode := report.ModeApply
	if r.cfg.DryRun {
		mode = report.ModeDryRun
	}
	return report.Build(report.Input{
		Plugin:            "packer-plugin-windows-features " + Version,
		StartedAt:         r.started,
		FinishedAt:        r.now(),
		Mode:              mode,
		Facts:             r.facts,
		System:            r.system,
		Plan:              r.plan,
		IgnoreUnavailable: r.cfg.IgnoreUnavailable,
		Connectivity:      r.checks,
		Reboot:            r.reboot,
		Warnings:          r.warnings,
		RunError:          runErr,
	})
}

// execute runs the steps in order: upload and detect, inventory and plan,
// stop on blocking requests or in dry-run mode, restart first when a pending
// restart blocks servicing, apply each operation, inventory again and verify,
// check WinRM, and restart as needed.
//
// Returns:
//   - error: A step failed, a request is unavailable, protected or
//     conflicting, WinRM regressed, or requests failed while fail_on_error is
//     true.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *run) execute(ctx context.Context) error {
	if err := r.prepareGuest(ctx); err != nil {
		return err
	}
	before, err := r.planRequests(ctx)
	if err != nil {
		return err
	}
	if r.cfg.DryRun {
		r.reboot.Required = r.plan.RebootRequired()
		r.ui.Say("Dry run: no changes were made.")
		return nil
	}
	if len(r.plan.Pending()) > 0 && r.facts.Reboot.BlocksServicing() {
		if !r.cfg.RestartIfRequired {
			r.warn("a restart is pending (%s); servicing may fail until the machine restarts", strings.Join(r.facts.Reboot.Reasons(), "; "))
		} else {
			r.ui.Say("A restart is pending and can block servicing; restarting before any change...")
			r.reboot.Reason = "a restart was pending before any change"
			if err := r.restartAndCheck(ctx); err != nil {
				return err
			}
			if before, err = r.planRequests(ctx); err != nil {
				return err
			}
		}
	}
	if pending := r.plan.Pending(); len(pending) > 0 {
		if r.facts.WinRM == nil && slices.ContainsFunc(pending, func(it *plan.Item) bool { return it.Action == plan.ActionRemove }) {
			return errors.New("the WinRM configuration could not be read, so the effect of a removal on the Packer session cannot be checked; nothing was changed")
		}
		results, operationErr := r.operate(ctx, pending)
		if err := ctx.Err(); err != nil {
			return err
		}
		after, err := r.inventories(ctx, r.plan.Interfaces())
		if err != nil {
			return err
		}
		r.plan.Verify(pending, results, after)
		for _, it := range r.plan.Items {
			if it.Status == plan.StatusCompliant {
				r.plan.Verify([]*plan.Item{it}, map[string]*plan.OperationResult{it.ID: {ID: it.ID}}, after)
				if it.Status == plan.StatusInstalled || it.Status == plan.StatusRemoved {
					it.Status = plan.StatusCompliant
				}
			}
		}
		r.plan.RecordSideEffects(before, after)
		if err := r.checkConnectivity(ctx, "after the feature changes"); err != nil {
			return err
		}
		if operationErr != nil {
			return operationErr
		}
	} else {
		r.ui.Say("Every request is already in the requested state; nothing to change.")
	}
	if err := r.completeRestart(ctx); err != nil {
		return err
	}
	if bad := r.plan.Unsuccessful(r.cfg.IgnoreUnavailable); len(bad) > 0 {
		return r.failure("%d requests did not reach the requested state: %s", len(bad), describeItems(bad))
	}
	return nil
}

// planRequests reads the inventories the requests need and builds the plan.
// Unavailable requests are warnings when ignore_unavailable is set. Any other
// blocking request stops the run before anything is changed, whatever
// fail_on_error says.
//
// Returns:
//   - plan.Inventories: The inventories the plan was built from.
//   - error: An inventory cannot be read, or a request blocks the run.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *run) planRequests(ctx context.Context) (plan.Inventories, error) {
	var needed []facts.Interface
	if r.cfg.hasKind(plan.KindFeature) {
		needed = append(needed, r.system.FeatureInterface)
	}
	if r.cfg.hasKind(plan.KindCapability) {
		if r.system.Capabilities {
			needed = append(needed, facts.InterfaceCapability)
		} else {
			r.warn("this system has no Get-WindowsCapability cmdlet; capability requests are unavailable")
		}
	}
	inv, err := r.inventories(ctx, needed)
	if err != nil {
		return nil, err
	}
	r.plan = plan.Build(r.cfg.requests(), r.system.FeatureInterface, inv)
	r.ui.Say(fmt.Sprintf("Plan: %d requests: %d to change, %d already compliant, %d pending restart, %d unavailable, %d protected, %d conflicting",
		len(r.plan.Items), r.plan.Count(plan.StatusPlanned), r.plan.Count(plan.StatusCompliant), r.plan.Count(plan.StatusPendingReboot),
		r.plan.Count(plan.StatusUnavailable), r.plan.Count(plan.StatusProtected), r.plan.Count(plan.StatusConflict)))
	if r.system.Family == facts.FamilyClient && r.cfg.IncludeManagementTools && r.cfg.hasKind(plan.KindFeature) {
		r.warn("include_management_tools applies to Windows Server roles only; it is ignored on %s", r.system.Name)
	}
	if blocking := r.plan.Blocking(r.cfg.IgnoreUnavailable); len(blocking) > 0 {
		return nil, fmt.Errorf("%d requests cannot be applied, so nothing was changed: %s", len(blocking), describeItems(blocking))
	}
	for _, it := range r.plan.Items {
		if it.Status == plan.StatusPlanned && it.Action == plan.ActionInstall && slices.Contains([]string{
			"hyper-v", "microsoft-hyper-v-all", "microsoft-hyper-v", "microsoft-hyper-v-hypervisor",
			"remoteaccess", "routing", "directaccess-vpn", "dns", "dhcp", "ad-domain-services",
		}, strings.ToLower(it.Name)) {
			r.warn("installing %s can change networking or authentication; verify the builder network and nested virtualization settings and retain out-of-band access", it.Name)
		}
		if it.Status == plan.StatusUnavailable {
			r.warn("skipped %s %s: %s", it.Action, it.Request.Name, it.Reason)
		}
	}
	return inv, nil
}

// describeItems lists items as "action name (status: reason)".
//
// Author: Lars Krogh
// Version: 1.0.0
func describeItems(items []*plan.Item) string {
	parts := make([]string, len(items))
	for i, it := range items {
		parts[i] = fmt.Sprintf("%s %s (%s: %s)", it.Action, it.Request.Name, it.Status, it.Reason)
	}
	return strings.Join(parts, "; ")
}

// failure returns an error built from format, or records it as a warning and
// returns nil when fail_on_error is false.
//
// Returns:
//   - error: The failure, or nil when failures do not fail the build.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *run) failure(format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	if r.cfg.failOnError() {
		return errors.New(msg)
	}
	r.warn("%s; continuing because fail_on_error is false", msg)
	return nil
}

// prepareGuest uploads the scripts, detects the guest and classifies it.
//
// Returns:
//   - error: The upload or detection failed, the session is not elevated,
//     PowerShell is not 64-bit, or the OS is not supported.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *run) prepareGuest(ctx context.Context) error {
	r.ui.Say("Uploading the windows-features guest scripts...")
	if err := r.remote.install(ctx); err != nil {
		return err
	}
	r.ui.Say("Detecting the operating system, servicing interfaces, restart state and WinRM...")
	data, err := r.remote.action(ctx, "Detect", nil)
	if err != nil {
		return err
	}
	f, err := facts.Parse(data)
	if err != nil {
		return err
	}
	r.facts = f
	for _, e := range f.Errors {
		r.warn("detection: %s", e)
	}
	if !f.Session.IsElevated {
		return fmt.Errorf("the WinRM session (%s) is not elevated; servicing features requires an administrator session", f.Session.UserName)
	}
	if !f.OS.Is64BitProcess {
		return errors.New("the guest PowerShell process is not 64-bit; DISM cannot service a 64-bit Windows from a 32-bit process")
	}
	system, err := facts.Classify(f)
	if err != nil {
		return fmt.Errorf("cannot manage features on %s: %w", f.OS.ProductName, err)
	}
	r.system = &system
	r.reboot.PendingBefore = f.Reboot.Reasons()
	r.ui.Say(fmt.Sprintf("Detected %s (%s, %s, build %s, %s); features through %s, capabilities %s",
		system.Name, f.OS.EditionID, system.Installation, f.OS.Version(), f.OS.Architecture, system.FeatureInterface, availability(system.Capabilities)))
	if !system.Known {
		r.warn("%s is newer than the releases this plugin version was tested with", system.Name)
	}
	return nil
}

// availability returns "available" or "unavailable".
//
// Author: Lars Krogh
// Version: 1.0.0
func availability(ok bool) string {
	if ok {
		return "available"
	}
	return "unavailable"
}

// inventories runs the Inventory action once per interface.
//
// Returns:
//   - plan.Inventories: The inventory of each interface.
//   - error: An action failed or its result cannot be decoded.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *run) inventories(ctx context.Context, ifaces []facts.Interface) (plan.Inventories, error) {
	out := plan.Inventories{}
	for _, iface := range ifaces {
		r.ui.Say(fmt.Sprintf("Reading the %s inventory...", iface))
		data, err := r.remote.action(ctx, "Inventory", struct {
			Interface facts.Interface `json:"interface"`
		}{iface})
		if err != nil {
			return nil, fmt.Errorf("read the %s inventory: %w", iface, err)
		}
		inv, err := plan.ParseInventory(data)
		if err != nil {
			return nil, err
		}
		if inv.Interface != iface {
			return nil, fmt.Errorf("asked for the %s inventory, the guest returned %s", iface, inv.Interface)
		}
		out[iface] = inv
	}
	return out, nil
}

// operate runs one Invoke action per item, each limited by
// operation_timeout. A failed operation does not stop the others. A timeout
// or cancellation does, because the guest may still be servicing and later
// operations would queue behind it.
//
// Returns:
//   - map[string]*plan.OperationResult: The guest result by item ID. Items
//     that were not attempted have no entry.
//   - error: An intermediate inventory failed, preventing further operations.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *run) operate(ctx context.Context, items []*plan.Item) (map[string]*plan.OperationResult, error) {
	results := map[string]*plan.OperationResult{}
	for i, it := range items {
		if i > 0 {
			inspectCtx, cancelInspect := context.WithTimeout(ctx, r.cfg.OperationTimeout)
			current, err := r.inventories(inspectCtx, []facts.Interface{it.Interface})
			cancelInspect()
			if err != nil {
				return results, fmt.Errorf("cannot check the state after the previous operation: %w", err)
			}
			state, _ := current[it.Interface].StateOf(it.Name)
			if (it.Action == plan.ActionInstall && (state == plan.StateInstalled || state == plan.StateInstallPending)) ||
				(it.Action == plan.ActionRemove && (state == plan.StateAvailable || state == plan.StateRemoved || state == plan.StateRemovePending)) {
				r.ui.Say(fmt.Sprintf("%s reached the requested state through an earlier operation; skipping its servicing command", it.Name))
				results[it.ID] = &plan.OperationResult{ID: it.ID, OK: true}
				continue
			}
		}
		verb := "Installing"
		if it.Action == plan.ActionRemove {
			verb = "Removing"
		}
		r.ui.Say(fmt.Sprintf("[%d/%d] %s %s (%s)...", i+1, len(items), verb, it.Name, it.Interface))
		opCtx, cancel := context.WithTimeout(ctx, r.cfg.OperationTimeout)
		data, err := r.remote.action(opCtx, "Invoke", r.operation(it))
		timedOut := errors.Is(opCtx.Err(), context.DeadlineExceeded) && ctx.Err() == nil
		cancel()
		var res *plan.OperationResult
		if err == nil {
			res, err = plan.ParseOperationResult(data)
		}
		if err != nil {
			msg := err.Error()
			if timedOut {
				msg = fmt.Sprintf("timed out after %s; the guest may still be servicing, so the remaining operations were not started", r.cfg.OperationTimeout)
			}
			res = &plan.OperationResult{ID: it.ID, Error: &msg}
		}
		if res.Error != nil {
			hinted := withHint(*res.Error)
			res.Error = &hinted
			r.ui.Error(fmt.Sprintf("%s %s reported: %s", it.Action, it.Name, hinted))
		}
		results[it.ID] = res
		if timedOut || ctx.Err() != nil {
			break
		}
	}
	return results, nil
}

// operation converts an item to the guest's Invoke request.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *run) operation(it *plan.Item) operationRequest {
	op := operationRequest{ID: it.ID, Interface: it.Interface, Action: it.Action, Name: it.Name}
	if it.Action == plan.ActionInstall {
		op.Source, op.LimitAccess = r.cfg.Source, r.cfg.LimitAccess
		if it.Interface == facts.InterfaceServerManager {
			op.IncludeManagementTools, op.IncludeAllSubFeatures = r.cfg.IncludeManagementTools, r.cfg.IncludeAllSubFeatures
		}
	} else {
		if it.Interface == facts.InterfaceServerManager {
			op.IncludeManagementTools = r.cfg.IncludeManagementTools
		}
		if it.Kind == plan.KindFeature {
			op.RemovePayload = r.cfg.RemovePayload
		}
	}
	return op
}

// completeRestart records whether a restart is needed and, when
// restart_if_required is set, restarts until no request is pending or
// maxRestarts restarts were made. Each restart is followed by a WinRM check
// and a new inventory. Requests still pending after the last restart fail.
//
// Returns:
//   - error: A restart, the WinRM check or an inventory failed.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *run) completeRestart(ctx context.Context) error {
	if r.plan.RebootRequired() {
		r.reboot.Required = true
		r.reboot.Reason = "feature changes complete only after a restart"
		if r.cfg.RestartIfRequired {
			made := 0
			for made < maxRestarts && r.plan.RebootRequired() {
				made++
				if err := r.restartAndCheck(ctx); err != nil {
					return err
				}
				after, err := r.inventories(ctx, r.plan.Interfaces())
				if err != nil {
					return err
				}
				r.plan.Reverify(after)
			}
			r.plan.FailPending(made)
			r.reboot.Required = r.plan.RebootRequired()
		}
	}
	if r.lastProbe != nil {
		r.reboot.PendingAfter = r.lastProbe.Reboot.Reasons()
	} else {
		r.reboot.PendingAfter = r.facts.Reboot.Reasons()
	}
	return nil
}

// restartAndCheck restarts the guest and checks WinRM afterwards.
//
// Returns:
//   - error: The restart or the WinRM check failed.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *run) restartAndCheck(ctx context.Context) error {
	if err := r.restart(ctx); err != nil {
		return err
	}
	return r.checkConnectivity(ctx, fmt.Sprintf("after restart %d", r.reboot.Restarts))
}

// checkConnectivity runs a new WinRM command and compares the WinRM
// configuration with the one found by Detect, recording the outcome as a
// report check. when describes the moment, for example "after restart 1". Its
// failure is never softened by fail_on_error.
//
// Returns:
//   - error: The probe failed or WinRM regressed.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *run) checkConnectivity(ctx context.Context, when string) error {
	name := "new WinRM command " + when
	probe, err := r.probe(ctx)
	if err != nil {
		r.addCheck(name, false, err.Error())
		return fmt.Errorf("WinRM connectivity check %s failed: %w", when, err)
	}
	r.lastProbe = probe
	if problems := facts.WinRMRegressions(r.facts.WinRM, probe); len(problems) > 0 {
		detail := strings.Join(problems, "; ")
		r.addCheck(name, false, detail)
		return fmt.Errorf("WinRM configuration regressed %s: %s", when, detail)
	}
	r.addCheck(name, true, "a new command ran and the WinRM service, listeners and authentication are unchanged")
	return nil
}

// probe runs the Probe action.
//
// Returns:
//   - *facts.Probe: The probe result.
//   - error: The action failed or its result cannot be decoded.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *run) probe(ctx context.Context) (*facts.Probe, error) {
	data, err := r.remote.action(ctx, "Probe", nil)
	if err != nil {
		return nil, err
	}
	return facts.ParseProbe(data)
}

// addCheck records a connectivity check for the report.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *run) addCheck(name string, passed bool, details string) {
	r.checks = append(r.checks, report.Check{Name: name, At: r.now().UTC(), Passed: passed, Details: details})
}
