// Package plan decides what the provisioner changes: it resolves each
// requested name against the guest's inventory, skips what is already in the
// requested state, rejects unavailable, conflicting and session-breaking
// requests before anything changes, and verifies every change against the
// inventory read afterwards. It does no I/O.
package plan

import (
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/larskroghdk/packer-plugin-windows-features/internal/facts"
)

// Action is what a request asks for.
type Action string

const (
	ActionInstall Action = "install"
	ActionRemove  Action = "remove"
)

// Kind separates features, managed by the OS's feature interface, from
// capabilities (Features on Demand), which have their own interface and
// names.
type Kind string

const (
	KindFeature    Kind = "feature"
	KindCapability Kind = "capability"
)

// Status is the outcome of one request.
type Status string

const (
	StatusPlanned       Status = "planned"
	StatusCompliant     Status = "compliant"
	StatusInstalled     Status = "installed"
	StatusRemoved       Status = "removed"
	StatusPendingReboot Status = "pending_reboot"
	StatusUnavailable   Status = "unavailable"
	StatusProtected     Status = "protected"
	StatusConflict      Status = "conflict"
	StatusFailed        Status = "failed"
	StatusNotAttempted  Status = "not_attempted"
)

// Request is one name from the configuration.
type Request struct {
	Name   string
	Kind   Kind
	Action Action
}

// Item is the plan entry for one request. Name is the name the inventory
// uses, which may differ in case or, for capabilities, in version suffix
// from the requested name. Related lists the features that Server Manager
// installs or removes along with this one.
type Item struct {
	Request
	ID            string
	Interface     facts.Interface
	Name          string
	DisplayName   string
	FeatureType   string
	Before        State
	After         State
	Status        Status
	Reason        string
	Related       []string
	RestartNeeded bool
}

// SideEffect is a feature that was not requested but changed state during
// the run, usually a dependency or a management tool.
type SideEffect struct {
	Interface facts.Interface `json:"interface"`
	Name      string          `json:"name"`
	Before    State           `json:"before"`
	After     State           `json:"after"`
}

// OperationResult is the guest's result for one install or remove. It is
// only evidence: the outcome is decided by the state read afterwards.
type OperationResult struct {
	ID            string   `json:"id"`
	OK            bool     `json:"ok"`
	Error         *string  `json:"error"`
	RestartNeeded bool     `json:"restart_needed"`
	ExitCode      string   `json:"exit_code"`
	Changed       []string `json:"changed"`
}

// Plan is the ordered list of items and the side effects seen after
// applying them.
type Plan struct {
	Items       []*Item
	SideEffects []SideEffect
}

// Inventories holds one inventory per servicing interface that was read. An
// interface the guest lacks has no entry.
type Inventories map[facts.Interface]*Inventory

// Build resolves every request against inv and decides its initial status.
// Feature requests use featureIface, capability requests the capability
// interface. Removals are ordered before installs, so a feature freed by a
// removal never blocks an install.
//
// Author: Lars Krogh
// Version: 1.0.0
func Build(requests []Request, featureIface facts.Interface, inv Inventories) *Plan {
	p := &Plan{}
	for _, req := range requests {
		it := &Item{Request: req, ID: fmt.Sprintf("%s:%s:%s", req.Kind, req.Action, req.Name), Interface: featureIface}
		if req.Kind == KindCapability {
			it.Interface = facts.InterfaceCapability
		}
		it.resolve(inv[it.Interface])
		p.Items = append(p.Items, it)
	}
	p.findConflicts(inv)
	sort.SliceStable(p.Items, func(i, j int) bool {
		return p.Items[i].Action == ActionRemove && p.Items[j].Action == ActionInstall
	})
	return p
}

// resolve looks the item up in inv and sets its name, state, related
// features and status.
//
// Author: Lars Krogh
// Version: 1.0.0
func (it *Item) resolve(inv *Inventory) {
	it.Name = it.Request.Name
	if inv == nil {
		it.Status = StatusUnavailable
		it.Reason = fmt.Sprintf("this system has no %s servicing interface", it.Interface)
		return
	}
	f, why := inv.Resolve(it.Request.Name)
	if f == nil {
		it.Status = StatusUnavailable
		it.Reason = why
		return
	}
	it.Name, it.DisplayName, it.FeatureType = f.Name, f.DisplayName, f.Type
	it.Before = Normalize(inv.Interface, f.State)
	if it.Before == StateUnknown {
		it.Status = StatusFailed
		it.Reason = fmt.Sprintf("%s reports the unrecognized state %q; nothing is changed", it.Name, f.State)
		return
	}
	switch it.Action {
	case ActionInstall:
		it.Related = inv.InstallClosure(it.Name)
		switch it.Before {
		case StateInstalled:
			it.Status = StatusCompliant
		case StateInstallPending:
			it.Status, it.RestartNeeded, it.Reason = StatusPendingReboot, true, "already installed; the installation completes at the next restart"
		default:
			it.Status = StatusPlanned
		}
	case ActionRemove:
		it.Related = inv.RemovalClosure(it.Name)
		switch it.Before {
		case StateAvailable, StateRemoved:
			it.Status = StatusCompliant
		case StateRemovePending:
			it.Status, it.RestartNeeded, it.Reason = StatusPendingReboot, true, "already removed; the removal completes at the next restart"
		default:
			it.Status = StatusPlanned
			if why := Protect(it.Interface, it.Name, it.Related); why != "" {
				it.Status, it.Reason = StatusProtected, why
			}
		}
	}
}

// findConflicts marks requests that contradict each other: two requests that
// resolve to the same name, and an install whose dependencies a removal in
// the same run would remove, or the reverse.
//
// Author: Lars Krogh
// Version: 1.0.0
func (p *Plan) findConflicts(inv Inventories) {
	type key struct {
		iface facts.Interface
		name  string
	}
	owner := map[key]*Item{}
	for _, it := range p.Items {
		if it.Status == StatusUnavailable {
			continue
		}
		k := key{it.Interface, strings.ToLower(it.Name)}
		if other, dup := owner[k]; dup {
			conflict(it, other, fmt.Sprintf("%s and %s both resolve to %s", other.Request.Name, it.Request.Name, it.Name))
			continue
		}
		owner[k] = it
	}
	for _, in := range p.Items {
		if in.Action != ActionInstall || in.Status == StatusUnavailable {
			continue
		}
		needs := append([]string{in.Name}, in.Related...)
		if inventory := inv[in.Interface]; inventory != nil {
			needs = append(needs, inventory.RequiredClosure(in.Name)...)
		}
		for _, out := range p.Items {
			if out.Action != ActionRemove || out.Interface != in.Interface || out.Status == StatusUnavailable {
				continue
			}
			removes := append([]string{out.Name}, out.Related...)
			if shared := intersectFold(needs, removes); shared != "" {
				conflict(in, out, fmt.Sprintf("installing %s needs %s, which removing %s removes", in.Name, shared, out.Name))
			}
		}
	}
}

// conflict marks both items as conflicting with reason.
//
// Author: Lars Krogh
// Version: 1.0.0
func conflict(a, b *Item, reason string) {
	for _, it := range []*Item{a, b} {
		it.Status, it.Reason = StatusConflict, reason
	}
}

// intersectFold returns the first name of a that is also in b, ignoring
// case, or "".
//
// Author: Lars Krogh
// Version: 1.0.0
func intersectFold(a, b []string) string {
	for _, x := range a {
		if slices.ContainsFunc(b, func(y string) bool { return strings.EqualFold(x, y) }) {
			return x
		}
	}
	return ""
}

// Blocking returns the items that stop the run before any change: protected,
// conflicting and failed items, and unavailable ones unless ignoreUnavailable
// is set.
//
// Author: Lars Krogh
// Version: 1.0.0
func (p *Plan) Blocking(ignoreUnavailable bool) []*Item {
	var out []*Item
	for _, it := range p.Items {
		switch it.Status {
		case StatusProtected, StatusConflict, StatusFailed:
			out = append(out, it)
		case StatusUnavailable:
			if !ignoreUnavailable {
				out = append(out, it)
			}
		}
	}
	return out
}

// Pending returns the items that need an operation, removals first.
//
// Author: Lars Krogh
// Version: 1.0.0
func (p *Plan) Pending() []*Item {
	var out []*Item
	for _, it := range p.Items {
		if it.Status == StatusPlanned {
			out = append(out, it)
		}
	}
	return out
}

// Interfaces returns the interfaces of items that are not unavailable, in a
// fixed order.
//
// Author: Lars Krogh
// Version: 1.0.0
func (p *Plan) Interfaces() []facts.Interface {
	var out []facts.Interface
	for _, iface := range []facts.Interface{facts.InterfaceServerManager, facts.InterfaceOptionalFeature, facts.InterfaceCapability} {
		if slices.ContainsFunc(p.Items, func(it *Item) bool { return it.Interface == iface && it.Status != StatusUnavailable }) {
			out = append(out, iface)
		}
	}
	return out
}

// Verify sets the outcome of each operated item from the state in after.
// The guest's result only adds the restart flag and the error text. An item
// whose operation reported success but whose feature is not in the requested
// state fails, and an item whose operation reported an error but whose
// feature reached the requested state succeeds with a note. Items without a
// result were not attempted.
//
// Parameters:
//   - items: The items that were due for an operation.
//   - results: The guest results by item ID.
//
// Author: Lars Krogh
// Version: 1.0.0
func (p *Plan) Verify(items []*Item, results map[string]*OperationResult, after Inventories) {
	for _, it := range items {
		res := results[it.ID]
		if res == nil {
			it.Status, it.Reason = StatusNotAttempted, "not attempted because an earlier operation timed out or the run was cancelled"
			continue
		}
		opErr := ""
		if res.Error != nil {
			opErr = *res.Error
		}
		it.RestartNeeded = res.RestartNeeded
		it.settle(after[it.Interface], "after the operation", opErr)
	}
}

// Reverify re-reads the state of every item that changed or waits for a
// restart, after a restart. A pending item that completed takes its final
// status, one still pending stays pending, and one that lost its state
// fails. The restart flag from the operation is cleared, because the restart
// it asked for has happened.
//
// Author: Lars Krogh
// Version: 1.0.0
func (p *Plan) Reverify(after Inventories) {
	for _, it := range p.Items {
		switch it.Status {
		case StatusInstalled, StatusRemoved, StatusPendingReboot, StatusCompliant:
			wasCompliant := it.Status == StatusCompliant
			it.RestartNeeded = false
			it.settle(after[it.Interface], "after the restart", "")
			if wasCompliant && (it.Status == StatusInstalled || it.Status == StatusRemoved) {
				it.Status = StatusCompliant
			}
		}
	}
}

// settle sets the item's status from its state in inv.
//
// Parameters:
//   - when: Names the moment for the failure reason.
//   - opErr: The error the operation reported, or "".
//
// Author: Lars Krogh
// Version: 1.0.0
func (it *Item) settle(inv *Inventory, when, opErr string) {
	if inv == nil {
		it.Status, it.Reason = StatusFailed, fmt.Sprintf("the %s inventory could not be read %s", it.Interface, when)
		return
	}
	st, ok := inv.StateOf(it.Name)
	if !ok {
		it.Status, it.Reason = StatusFailed, fmt.Sprintf("%s is no longer listed %s", it.Name, when)
		return
	}
	it.After = st
	done, pending := StatusInstalled, StateInstallPending
	reached := st == StateInstalled
	if it.Action == ActionRemove {
		done, pending = StatusRemoved, StateRemovePending
		reached = st == StateAvailable || st == StateRemoved
	}
	switch {
	case reached:
		it.Status, it.Reason = done, ""
		if opErr != "" {
			it.Reason = "the operation reported an error, but the requested state was reached: " + opErr
		}
	case st == pending:
		it.Status, it.RestartNeeded, it.Reason = StatusPendingReboot, true, fmt.Sprintf("%s completes at the next restart", it.Action)
	default:
		it.Status = StatusFailed
		it.Reason = fmt.Sprintf("%s is %s %s", it.Name, st, when)
		if opErr != "" {
			it.Reason += ": " + opErr
		}
	}
}

// FailPending fails every item that is still pending after the plugin
// restarted the guest restarts times.
//
// Author: Lars Krogh
// Version: 1.0.0
func (p *Plan) FailPending(restarts int) {
	for _, it := range p.Items {
		if it.Status == StatusPendingReboot {
			it.Status = StatusFailed
			it.Reason = fmt.Sprintf("still %s after %d restarts", it.After, restarts)
		}
	}
}

// RecordSideEffects lists every feature that was not requested and whose
// state differs between before and after.
//
// Author: Lars Krogh
// Version: 1.0.0
func (p *Plan) RecordSideEffects(before, after Inventories) {
	requested := map[string]bool{}
	for _, it := range p.Items {
		requested[string(it.Interface)+":"+strings.ToLower(it.Name)] = true
	}
	p.SideEffects = nil
	for _, iface := range []facts.Interface{facts.InterfaceServerManager, facts.InterfaceOptionalFeature, facts.InterfaceCapability} {
		b, a := before[iface], after[iface]
		if b == nil || a == nil {
			continue
		}
		for _, f := range a.Features {
			if requested[string(iface)+":"+strings.ToLower(f.Name)] {
				continue
			}
			was, ok := b.StateOf(f.Name)
			now := Normalize(iface, f.State)
			if ok && was != now {
				p.SideEffects = append(p.SideEffects, SideEffect{Interface: iface, Name: f.Name, Before: was, After: now})
			}
		}
	}
}

// RebootRequired reports whether an item needs a restart to complete.
//
// Author: Lars Krogh
// Version: 1.0.0
func (p *Plan) RebootRequired() bool {
	return slices.ContainsFunc(p.Items, func(it *Item) bool { return it.Status == StatusPendingReboot || it.RestartNeeded })
}

// Count returns the number of items with status s.
//
// Author: Lars Krogh
// Version: 1.0.0
func (p *Plan) Count(s Status) int {
	n := 0
	for _, it := range p.Items {
		if it.Status == s {
			n++
		}
	}
	return n
}

// Unsuccessful returns the items that did not reach, or will not reach, the
// requested state. Unavailable items count unless ignoreUnavailable is set.
//
// Author: Lars Krogh
// Version: 1.0.0
func (p *Plan) Unsuccessful(ignoreUnavailable bool) []*Item {
	var out []*Item
	for _, it := range p.Items {
		switch it.Status {
		case StatusCompliant, StatusInstalled, StatusRemoved, StatusPendingReboot:
		case StatusUnavailable:
			if !ignoreUnavailable {
				out = append(out, it)
			}
		default:
			out = append(out, it)
		}
	}
	return out
}

// ParseOperationResult decodes the guest's result of one operation.
//
// Returns:
//   - *OperationResult: The decoded result.
//   - error: The data is not valid JSON.
//
// Author: Lars Krogh
// Version: 1.0.0
func ParseOperationResult(data []byte) (*OperationResult, error) {
	var r OperationResult
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("decode operation result: %w", err)
	}
	return &r, nil
}
