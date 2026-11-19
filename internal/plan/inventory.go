package plan

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/larskroghdk/packer-plugin-windows-features/internal/facts"
)

// State is the normalized installation state of a feature or capability.
type State string

const (
	StateInstalled      State = "installed"
	StateAvailable      State = "available"
	StateRemoved        State = "removed"
	StateInstallPending State = "install_pending"
	StateRemovePending  State = "remove_pending"
	StateUnknown        State = "unknown"
)

var rawStates = map[facts.Interface]map[string]State{
	facts.InterfaceServerManager: {
		"installed":        StateInstalled,
		"available":        StateAvailable,
		"removed":          StateRemoved,
		"installpending":   StateInstallPending,
		"uninstallpending": StateRemovePending,
	},
	facts.InterfaceOptionalFeature: {
		"enabled":                    StateInstalled,
		"disabled":                   StateAvailable,
		"disabledwithpayloadremoved": StateRemoved,
		"enablepending":              StateInstallPending,
		"disablepending":             StateRemovePending,
	},
	facts.InterfaceCapability: {
		"installed":        StateInstalled,
		"notpresent":       StateAvailable,
		"staged":           StateAvailable,
		"removed":          StateRemoved,
		"installpending":   StateInstallPending,
		"uninstallpending": StateRemovePending,
	},
}

// Normalize maps a state as the servicing interface reports it to a State.
// A state the plugin does not know, such as PartiallyInstalled or
// Superseded, is StateUnknown and is never treated as compliant.
//
// Author: Lars Krogh
// Version: 1.0.0
func Normalize(iface facts.Interface, raw string) State {
	if s, ok := rawStates[iface][strings.ToLower(strings.TrimSpace(raw))]; ok {
		return s
	}
	return StateUnknown
}

// Feature is one entry of an inventory as the guest reports it. Type,
// DependsOn, Parent and SubFeatures are only set by Server Manager.
type Feature struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name"`
	State       string   `json:"state"`
	Type        string   `json:"type"`
	DependsOn   []string `json:"depends_on"`
	Parent      string   `json:"parent"`
	SubFeatures []string `json:"sub_features"`
}

// Inventory is every feature or capability one servicing interface lists.
type Inventory struct {
	Interface facts.Interface `json:"interface"`
	Features  []Feature       `json:"features"`
	index     map[string]int
}

// ParseInventory decodes the Inventory action result and indexes it by
// lower-case name.
//
// Returns:
//   - *Inventory: The decoded inventory.
//   - error: The data is not valid JSON, names no interface, or has
//     duplicate or empty names.
//
// Author: Lars Krogh
// Version: 1.0.0
func ParseInventory(data []byte) (*Inventory, error) {
	var inv Inventory
	if err := json.Unmarshal(data, &inv); err != nil {
		return nil, fmt.Errorf("decode inventory: %w", err)
	}
	if _, ok := rawStates[inv.Interface]; !ok {
		return nil, fmt.Errorf("inventory has unknown interface %q", inv.Interface)
	}
	if err := inv.reindex(); err != nil {
		return nil, err
	}
	return &inv, nil
}

// NewInventory builds an indexed inventory, for tests and the fake guest.
//
// Returns:
//   - *Inventory: The inventory.
//   - error: A name is empty or appears twice.
//
// Author: Lars Krogh
// Version: 1.0.0
func NewInventory(iface facts.Interface, features []Feature) (*Inventory, error) {
	inv := &Inventory{Interface: iface, Features: features}
	if err := inv.reindex(); err != nil {
		return nil, err
	}
	return inv, nil
}

// reindex rebuilds the name index.
//
// Returns:
//   - error: A name is empty or appears twice.
//
// Author: Lars Krogh
// Version: 1.0.0
func (inv *Inventory) reindex() error {
	inv.index = make(map[string]int, len(inv.Features))
	for i, f := range inv.Features {
		key := strings.ToLower(f.Name)
		if key == "" {
			return fmt.Errorf("%s inventory has an entry without a name", inv.Interface)
		}
		if _, dup := inv.index[key]; dup {
			return fmt.Errorf("%s inventory lists %s twice", inv.Interface, f.Name)
		}
		inv.index[key] = i
	}
	return nil
}

// Get returns the feature with exactly this name, ignoring case.
//
// Author: Lars Krogh
// Version: 1.0.0
func (inv *Inventory) Get(name string) *Feature {
	if i, ok := inv.index[strings.ToLower(name)]; ok {
		return &inv.Features[i]
	}
	return nil
}

// StateOf returns the normalized state of the named feature, and false when
// the inventory does not list it.
//
// Returns:
//   - State: The normalized state.
//   - bool: The feature is listed.
//
// Author: Lars Krogh
// Version: 1.0.0
func (inv *Inventory) StateOf(name string) (State, bool) {
	f := inv.Get(name)
	if f == nil {
		return "", false
	}
	return Normalize(inv.Interface, f.State), true
}

// Resolve finds the feature a requested name refers to. Names match exactly,
// ignoring case. A capability name without a version, such as
// OpenSSH.Server, also matches the single capability whose identity before
// the first '~' equals it, so OpenSSH.Server resolves to
// OpenSSH.Server~~~~0.0.1.0.
//
// Returns:
//   - *Feature: The feature, or nil.
//   - string: Why nothing was found, when the feature is nil.
//
// Author: Lars Krogh
// Version: 1.0.0
func (inv *Inventory) Resolve(name string) (*Feature, string) {
	if f := inv.Get(name); f != nil {
		return f, ""
	}
	if inv.Interface == facts.InterfaceCapability && !strings.Contains(name, "~") {
		var matches []string
		for _, f := range inv.Features {
			if identity, _, _ := strings.Cut(f.Name, "~"); strings.EqualFold(identity, name) {
				matches = append(matches, f.Name)
			}
		}
		switch len(matches) {
		case 1:
			return inv.Get(matches[0]), ""
		case 0:
		default:
			sort.Strings(matches)
			return nil, fmt.Sprintf("%s matches %d capabilities (%s); use the full capability name", name, len(matches), strings.Join(matches, ", "))
		}
	}
	return nil, fmt.Sprintf("%s is not a %s on this system", name, inv.Interface.Describe())
}

// InstallClosure returns the features that installing name also installs:
// every feature it depends on and every parent, transitively, that is not
// installed yet. Only Server Manager reports dependencies, so the result is
// empty for the other interfaces.
//
// Author: Lars Krogh
// Version: 1.0.0
func (inv *Inventory) InstallClosure(name string) []string {
	var out []string
	for _, dep := range inv.RequiredClosure(name) {
		if st, ok := inv.StateOf(dep); ok && st != StateInstalled && st != StateInstallPending {
			out = append(out, dep)
		}
	}
	return out
}

// RequiredClosure returns all transitive dependencies and parents, including
// those already installed, so a conflicting removal cannot invalidate them.
//
// Author: Lars Krogh
// Version: 1.0.0
func (inv *Inventory) RequiredClosure(name string) []string {
	seen := map[string]bool{strings.ToLower(name): true}
	queue := []string{name}
	var out []string
	for len(queue) > 0 {
		f := inv.Get(queue[0])
		queue = queue[1:]
		if f == nil {
			continue
		}
		next := append([]string{}, f.DependsOn...)
		if f.Parent != "" {
			next = append(next, f.Parent)
		}
		for _, dep := range next {
			key := strings.ToLower(dep)
			if seen[key] {
				continue
			}
			seen[key] = true
			queue = append(queue, dep)
			if _, ok := inv.StateOf(dep); ok {
				out = append(out, inv.Get(dep).Name)
			}
		}
	}
	sort.Strings(out)
	return out
}

// RemovalClosure returns the installed features that removing name also
// removes: its sub-features, and every feature that depends on, or is a
// child of, a feature in the set, until nothing is added. It is a superset of
// what Uninstall-WindowsFeature removes, so a protection decision based on it
// errs on the safe side.
//
// Author: Lars Krogh
// Version: 1.0.0
func (inv *Inventory) RemovalClosure(name string) []string {
	set := map[string]bool{strings.ToLower(name): true}
	for changed := true; changed; {
		changed = false
		for _, f := range inv.Features {
			key := strings.ToLower(f.Name)
			if set[key] {
				continue
			}
			st := Normalize(inv.Interface, f.State)
			if st != StateInstalled && st != StateInstallPending {
				continue
			}
			if set[strings.ToLower(f.Parent)] || containsFold(f.DependsOn, set) {
				set[key] = true
				changed = true
			}
		}
	}
	var out []string
	for _, f := range inv.Features {
		if key := strings.ToLower(f.Name); set[key] && key != strings.ToLower(name) {
			out = append(out, f.Name)
		}
	}
	sort.Strings(out)
	return out
}

// containsFold reports whether any of names is in set, which has lower-case
// keys.
//
// Author: Lars Krogh
// Version: 1.0.0
func containsFold(names []string, set map[string]bool) bool {
	for _, n := range names {
		if set[strings.ToLower(n)] {
			return true
		}
	}
	return false
}
