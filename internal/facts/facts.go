// Package facts decodes what the guest reports about itself: the Windows
// release, edition, installation type and architecture, the servicing
// interfaces it offers, its pending restart state, the Packer session and the
// WinRM configuration.
package facts

import (
	"encoding/json"
	"fmt"
)

const (
	ProductTypeWorkstation      = 1
	ProductTypeDomainController = 2
	ProductTypeServer           = 3
)

// Facts is the result of the guest Detect action. Pointer fields are nil when
// the guest could not collect that part. Errors says why, and every decision
// that needs a missing part must fail closed.
type Facts struct {
	OS        OS             `json:"os"`
	Session   Session        `json:"session"`
	Servicing Servicing      `json:"servicing"`
	Reboot    *PendingReboot `json:"reboot"`
	WinRM     *WinRM         `json:"winrm"`
	Errors    []string       `json:"errors"`
}

// OS identifies the Windows release, edition, installation type and process
// architecture.
type OS struct {
	ProductName      string `json:"product_name"`
	EditionID        string `json:"edition_id"`
	InstallationType string `json:"installation_type"`
	DisplayVersion   string `json:"display_version"`
	Major            int    `json:"major"`
	Minor            int    `json:"minor"`
	Build            int    `json:"build"`
	UBR              int    `json:"ubr"`
	ProductType      int    `json:"product_type"`
	Architecture     string `json:"architecture"`
	Is64BitProcess   bool   `json:"is_64bit_process"`
}

// Servicing lists which feature servicing cmdlets the guest has. The guest
// checks for the cmdlets themselves, so a missing module is detected rather
// than assumed from the product name.
type Servicing struct {
	ServerManager    bool `json:"server_manager"`
	OptionalFeatures bool `json:"optional_features"`
	Capabilities     bool `json:"capabilities"`
}

// Session describes the account and token of the Packer WinRM session.
type Session struct {
	UserName   string `json:"user_name"`
	IsElevated bool   `json:"is_elevated"`
}

// PendingReboot is the set of restart indicators Windows keeps in the
// registry.
type PendingReboot struct {
	ComponentBasedServicing bool `json:"component_based_servicing"`
	WindowsUpdate           bool `json:"windows_update"`
	PendingFileRename       bool `json:"pending_file_rename"`
	ComputerRename          bool `json:"computer_rename"`
}

// Reasons returns a description of every indicator that is set, in a fixed
// order.
//
// Author: Lars Krogh
// Version: 1.0.0
func (p *PendingReboot) Reasons() []string {
	if p == nil {
		return nil
	}
	var out []string
	if p.ComponentBasedServicing {
		out = append(out, "component based servicing has a restart pending")
	}
	if p.WindowsUpdate {
		out = append(out, "Windows Update has a restart pending")
	}
	if p.PendingFileRename {
		out = append(out, "file rename operations are pending")
	}
	if p.ComputerRename {
		out = append(out, "a computer rename is pending")
	}
	return out
}

// BlocksServicing reports whether a pending restart can make feature
// servicing fail. Only component based servicing and Windows Update pending
// restarts do. Pending file renames are set by many installers and do not
// block servicing.
//
// Author: Lars Krogh
// Version: 1.0.0
func (p *PendingReboot) BlocksServicing() bool {
	return p != nil && (p.ComponentBasedServicing || p.WindowsUpdate)
}

// Parse decodes the Detect result.
//
// Returns:
//   - *Facts: The decoded facts.
//   - error: The data is not valid JSON, or the build or product type is
//     missing, which means the guest could not identify the OS.
//
// Author: Lars Krogh
// Version: 1.0.0
func Parse(data []byte) (*Facts, error) {
	var f Facts
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("decode system facts: %w", err)
	}
	if f.OS.Build == 0 || f.OS.ProductType == 0 {
		return nil, fmt.Errorf("system facts are incomplete: build %d, product type %d", f.OS.Build, f.OS.ProductType)
	}
	return &f, nil
}

// Version returns the full version as major.minor.build.revision.
//
// Author: Lars Krogh
// Version: 1.0.0
func (o OS) Version() string { return fmt.Sprintf("%d.%d.%d.%d", o.Major, o.Minor, o.Build, o.UBR) }
