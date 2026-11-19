// Package report builds the JSON feature report and the console summary.
//
// Increment SchemaVersion for any incompatible change to the JSON.
package report

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/larskroghdk/packer-plugin-windows-features/internal/facts"
	"github.com/larskroghdk/packer-plugin-windows-features/internal/plan"
)

const SchemaVersion = 1

const (
	ModeApply  = "apply"
	ModeDryRun = "dry_run"
)

// Report is the JSON document written at the end of every run.
type Report struct {
	SchemaVersion   int               `json:"schema_version"`
	Plugin          string            `json:"plugin"`
	StartedAt       time.Time         `json:"started_at"`
	FinishedAt      time.Time         `json:"finished_at"`
	DurationSeconds float64           `json:"duration_seconds"`
	Mode            string            `json:"mode"`
	System          System            `json:"system"`
	Summary         Summary           `json:"summary"`
	Validation      Validation        `json:"validation"`
	Requests        []Request         `json:"requests"`
	SideEffects     []plan.SideEffect `json:"side_effects"`
	Connectivity    []Check           `json:"connectivity"`
	Reboot          Reboot            `json:"reboot"`
	Warnings        []string          `json:"warnings"`
	Error           string            `json:"error,omitempty"`
}

// System describes the guest as detected.
type System struct {
	ProductName      string   `json:"product_name"`
	Release          string   `json:"release"`
	KnownRelease     bool     `json:"known_release"`
	Family           string   `json:"family"`
	Installation     string   `json:"installation"`
	DomainController bool     `json:"domain_controller"`
	EditionID        string   `json:"edition_id"`
	Version          string   `json:"version"`
	DisplayVersion   string   `json:"display_version"`
	Architecture     string   `json:"architecture"`
	FeatureInterface string   `json:"feature_interface"`
	Capabilities     bool     `json:"capabilities_available"`
	WinRMUser        string   `json:"winrm_user"`
	DetectionErrors  []string `json:"detection_errors"`
}

// Summary counts the requests by status.
type Summary struct {
	Requested     int `json:"requested"`
	Installed     int `json:"installed"`
	Removed       int `json:"removed"`
	Compliant     int `json:"compliant"`
	PendingReboot int `json:"pending_reboot"`
	Planned       int `json:"planned"`
	Unavailable   int `json:"unavailable"`
	Protected     int `json:"protected"`
	Conflict      int `json:"conflict"`
	Failed        int `json:"failed"`
	NotAttempted  int `json:"not_attempted"`
}

// Validation is the final verdict: Passed is true when every request is in,
// or after the pending restart will be in, the requested state, as read back
// from the guest.
type Validation struct {
	Passed  bool     `json:"passed"`
	Failing []string `json:"failing"`
}

// Request is the report entry for one requested name.
type Request struct {
	Requested     string   `json:"requested"`
	Name          string   `json:"name"`
	DisplayName   string   `json:"display_name,omitempty"`
	FeatureType   string   `json:"feature_type,omitempty"`
	Kind          string   `json:"kind"`
	Action        string   `json:"action"`
	Interface     string   `json:"interface"`
	Status        string   `json:"status"`
	Before        string   `json:"before,omitempty"`
	After         string   `json:"after,omitempty"`
	Related       []string `json:"related"`
	RestartNeeded bool     `json:"restart_needed"`
	Reason        string   `json:"reason,omitempty"`
}

// Check is one WinRM connectivity check made during the run.
type Check struct {
	Name    string    `json:"name"`
	At      time.Time `json:"at"`
	Passed  bool      `json:"passed"`
	Details string    `json:"details"`
}

// Reboot records the restart state: indicators found before any change and
// at the end, whether a restart is needed, and how many the plugin made.
type Reboot struct {
	PendingBefore []string `json:"pending_before"`
	PendingAfter  []string `json:"pending_after"`
	Required      bool     `json:"required"`
	Restarts      int      `json:"restarts"`
	Reason        string   `json:"reason,omitempty"`
}

// Input is everything Build needs. Facts, System and Plan are nil when the
// run failed before they were known.
type Input struct {
	Plugin            string
	StartedAt         time.Time
	FinishedAt        time.Time
	Mode              string
	Facts             *facts.Facts
	System            *facts.System
	Plan              *plan.Plan
	IgnoreUnavailable bool
	Connectivity      []Check
	Reboot            Reboot
	Warnings          []string
	RunError          error
}

// Build assembles the report from what the run produced, including when the
// run failed early.
//
// Author: Lars Krogh
// Version: 1.0.0
func Build(in Input) *Report {
	r := &Report{
		SchemaVersion:   SchemaVersion,
		Plugin:          in.Plugin,
		StartedAt:       in.StartedAt.UTC(),
		FinishedAt:      in.FinishedAt.UTC(),
		DurationSeconds: in.FinishedAt.Sub(in.StartedAt).Round(time.Millisecond).Seconds(),
		Mode:            in.Mode,
		Requests:        []Request{},
		SideEffects:     []plan.SideEffect{},
		Connectivity:    nonNil(in.Connectivity),
		Reboot:          in.Reboot,
		Warnings:        nonNil(in.Warnings),
		Validation:      Validation{Failing: []string{}},
	}
	r.Reboot.PendingBefore = nonNil(r.Reboot.PendingBefore)
	r.Reboot.PendingAfter = nonNil(r.Reboot.PendingAfter)
	if in.RunError != nil {
		r.Error = in.RunError.Error()
		r.Validation.Failing = append(r.Validation.Failing, "run failed: "+r.Error)
	}
	if in.Facts != nil {
		r.System = system(in.Facts, in.System)
	}
	if in.Plan == nil {
		r.Validation.Failing = append(r.Validation.Failing, "the run stopped before the requests were planned")
		return r
	}
	for _, it := range in.Plan.Items {
		r.Requests = append(r.Requests, request(it))
	}
	r.SideEffects = nonNil(in.Plan.SideEffects)
	p := in.Plan
	r.Summary = Summary{
		Requested:     len(p.Items),
		Installed:     p.Count(plan.StatusInstalled),
		Removed:       p.Count(plan.StatusRemoved),
		Compliant:     p.Count(plan.StatusCompliant),
		PendingReboot: p.Count(plan.StatusPendingReboot),
		Planned:       p.Count(plan.StatusPlanned),
		Unavailable:   p.Count(plan.StatusUnavailable),
		Protected:     p.Count(plan.StatusProtected),
		Conflict:      p.Count(plan.StatusConflict),
		Failed:        p.Count(plan.StatusFailed),
		NotAttempted:  p.Count(plan.StatusNotAttempted),
	}
	for _, it := range p.Unsuccessful(in.IgnoreUnavailable) {
		r.Validation.Failing = append(r.Validation.Failing, fmt.Sprintf("%s %s: %s", it.Action, it.Request.Name, it.Status))
	}
	if in.Mode == ModeDryRun && r.Summary.Planned > 0 {
		r.Validation.Failing = append(r.Validation.Failing, fmt.Sprintf("dry run: %d requests were not applied", r.Summary.Planned))
	}
	for _, c := range r.Connectivity {
		if !c.Passed {
			r.Validation.Failing = append(r.Validation.Failing, "connectivity check failed: "+c.Name)
		}
	}
	r.Validation.Passed = len(r.Validation.Failing) == 0
	return r
}

// nonNil returns s, or an empty slice when s is nil, so the JSON has [] rather
// than null.
//
// Author: Lars Krogh
// Version: 1.0.0
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// system converts the guest facts and classification to the report's system
// section.
//
// Author: Lars Krogh
// Version: 1.0.0
func system(f *facts.Facts, s *facts.System) System {
	out := System{
		ProductName:     f.OS.ProductName,
		EditionID:       f.OS.EditionID,
		Version:         f.OS.Version(),
		DisplayVersion:  f.OS.DisplayVersion,
		Architecture:    f.OS.Architecture,
		Capabilities:    f.Servicing.Capabilities,
		WinRMUser:       f.Session.UserName,
		DetectionErrors: nonNil(f.Errors),
	}
	if s != nil {
		out.Release, out.KnownRelease, out.Family = s.Name, s.Known, string(s.Family)
		out.Installation, out.DomainController = s.Installation, s.DomainController
		out.FeatureInterface = string(s.FeatureInterface)
	}
	return out
}

// request converts a plan item to its report entry.
//
// Author: Lars Krogh
// Version: 1.0.0
func request(it *plan.Item) Request {
	return Request{
		Requested:     it.Request.Name,
		Name:          it.Name,
		DisplayName:   it.DisplayName,
		FeatureType:   it.FeatureType,
		Kind:          string(it.Kind),
		Action:        string(it.Action),
		Interface:     string(it.Interface),
		Status:        string(it.Status),
		Before:        string(it.Before),
		After:         string(it.After),
		Related:       nonNil(it.Related),
		RestartNeeded: it.RestartNeeded,
		Reason:        it.Reason,
	}
}

// JSON returns the report as indented JSON with a trailing newline.
//
// Returns:
//   - []byte: The encoded report.
//   - error: The report cannot be encoded.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *Report) JSON() ([]byte, error) {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// SummaryLines returns the console summary: system, counts, one line per
// request, side effects, restart state, validation, warnings and duration.
//
// Author: Lars Krogh
// Version: 1.0.0
func (r *Report) SummaryLines() []string {
	s := r.System
	lines := []string{
		fmt.Sprintf("System:      %s (%s, %s, build %s, %s)", s.Release, s.EditionID, s.Installation, s.Version, s.Architecture),
		fmt.Sprintf("Mode:        %s, features through %s", r.Mode, s.FeatureInterface),
	}
	n := r.Summary
	lines = append(lines, fmt.Sprintf("Requests:    %d: %d installed, %d removed, %d compliant, %d pending restart, %d planned, %d unavailable, %d protected, %d conflicting, %d failed, %d not attempted",
		n.Requested, n.Installed, n.Removed, n.Compliant, n.PendingReboot, n.Planned, n.Unavailable, n.Protected, n.Conflict, n.Failed, n.NotAttempted))
	for _, q := range r.Requests {
		line := fmt.Sprintf("  %-14s %-7s %s", q.Status, q.Action, q.Name)
		if len(q.Related) > 0 && (q.Status == string(plan.StatusPlanned) || q.Status == string(plan.StatusInstalled) || q.Status == string(plan.StatusRemoved)) {
			line += " (with " + strings.Join(q.Related, ", ") + ")"
		}
		if q.Reason != "" {
			line += ": " + q.Reason
		}
		lines = append(lines, line)
	}
	for _, e := range r.SideEffects {
		lines = append(lines, fmt.Sprintf("  side effect    %s: %s -> %s", e.Name, e.Before, e.After))
	}
	switch {
	case r.Reboot.Required && r.Reboot.Restarts == 0:
		lines = append(lines, fmt.Sprintf("Restart:     required, not performed (%s); add a windows-restart provisioner or set restart_if_required", r.Reboot.Reason))
	case r.Reboot.Restarts > 0:
		lines = append(lines, fmt.Sprintf("Restart:     %d performed (%s)", r.Reboot.Restarts, r.Reboot.Reason))
	}
	if len(r.Reboot.PendingAfter) > 0 {
		lines = append(lines, "Pending:     "+strings.Join(r.Reboot.PendingAfter, "; "))
	}
	if r.Validation.Passed {
		lines = append(lines, "Validation:  passed, every request was read back in the requested state")
	} else {
		lines = append(lines, "Validation:  failed: "+strings.Join(r.Validation.Failing, "; "))
	}
	for _, w := range r.Warnings {
		lines = append(lines, "Warning:     "+w)
	}
	lines = append(lines, fmt.Sprintf("Duration:    %.1fs", r.DurationSeconds))
	return lines
}
