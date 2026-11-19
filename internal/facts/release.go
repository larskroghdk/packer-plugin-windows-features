package facts

import (
	"errors"
	"fmt"
	"strings"
)

// Interface is a Windows servicing interface that manages one namespace of
// feature names.
type Interface string

const (
	InterfaceServerManager   Interface = "server_manager"
	InterfaceOptionalFeature Interface = "optional_feature"
	InterfaceCapability      Interface = "capability"
)

// Family is Windows Server or Windows client.
type Family string

const (
	FamilyServer Family = "server"
	FamilyClient Family = "client"
)

const (
	minimumBuild   = 14393
	firstWindows11 = 22000
)

// Describe returns the interface's name as used in messages, with the
// cmdlet that lists its names.
//
// Author: Lars Krogh
// Version: 1.0.0
func (i Interface) Describe() string {
	switch i {
	case InterfaceServerManager:
		return "Windows Server role or feature (Get-WindowsFeature)"
	case InterfaceOptionalFeature:
		return "Windows optional feature (Get-WindowsOptionalFeature -Online)"
	case InterfaceCapability:
		return "Windows capability (Get-WindowsCapability -Online)"
	}
	return string(i)
}

// System is the classified guest. FeatureInterface manages the install and
// remove lists. Capabilities is false when the guest has no capability
// cmdlets. Known is false for a build newer than the release tables, which is
// supported but untested.
type System struct {
	Name             string
	Family           Family
	Installation     string
	DomainController bool
	Build            int
	Known            bool
	FeatureInterface Interface
	Capabilities     bool
}

var serverReleases = map[int]string{
	14393: "Windows Server 2016",
	17763: "Windows Server 2019",
	20348: "Windows Server 2022",
	26100: "Windows Server 2025",
}

var clientReleases = map[int]string{
	14393: "Windows 10 version 1607",
	17763: "Windows 10 version 1809",
	19044: "Windows 10 version 21H2",
	19045: "Windows 10 version 22H2",
	22621: "Windows 11 version 22H2",
	22631: "Windows 11 version 23H2",
	26100: "Windows 11 version 24H2",
	26200: "Windows 11 version 25H2",
}

// Classify identifies the release of f and selects the servicing interface
// for features. Windows Server uses Server Manager, Windows client uses DISM
// optional features. The choice follows the product type that Windows
// reports, and the interface must exist on the guest. Builds newer than the
// tables are accepted as unknown releases of the same family.
//
// Returns:
//   - System: The classified guest.
//   - error: The OS is older than Windows 10 version 1607 or Windows Server
//     2016, is Nano Server, has an unknown product or installation type, or
//     lacks the cmdlets of its feature servicing interface.
//
// Author: Lars Krogh
// Version: 1.0.0
func Classify(f *Facts) (System, error) {
	o := f.OS
	if o.Major != 10 {
		return System{}, fmt.Errorf("unsupported Windows version %d.%d: Windows 10, Windows 11 or Windows Server 2016 and later are required", o.Major, o.Minor)
	}
	if o.Build < minimumBuild {
		return System{}, fmt.Errorf("unsupported Windows build %d: Windows 10 version 1607 or Windows Server 2016 (build %d) or later is required", o.Build, minimumBuild)
	}
	s := System{Build: o.Build, Capabilities: f.Servicing.Capabilities}
	switch o.ProductType {
	case ProductTypeServer, ProductTypeDomainController:
		s.Family = FamilyServer
		s.DomainController = o.ProductType == ProductTypeDomainController
		switch strings.ToLower(o.InstallationType) {
		case "server":
			s.Installation = "server_desktop"
		case "server core":
			s.Installation = "server_core"
		case "nano server":
			return System{}, errors.New("unsupported Nano Server: it has no Server Manager or DISM online servicing cmdlets")
		default:
			return System{}, fmt.Errorf("unknown Windows Server installation type %q", o.InstallationType)
		}
		if !f.Servicing.ServerManager {
			return System{}, errors.New("this Windows Server has no Get-WindowsFeature cmdlet (ServerManager module); roles and features cannot be managed")
		}
		s.FeatureInterface = InterfaceServerManager
		s.Name, s.Known = serverReleases[o.Build]
		if !s.Known {
			s.Name = fmt.Sprintf("Windows Server build %d", o.Build)
		}
		if s.Installation == "server_core" {
			s.Name += " (Server Core)"
		}
	case ProductTypeWorkstation:
		s.Family = FamilyClient
		if !strings.EqualFold(o.InstallationType, "client") {
			return System{}, fmt.Errorf("unknown Windows client installation type %q", o.InstallationType)
		}
		s.Installation = "client"
		if !f.Servicing.OptionalFeatures {
			return System{}, errors.New("this Windows client has no Get-WindowsOptionalFeature cmdlet (DISM module); optional features cannot be managed")
		}
		s.FeatureInterface = InterfaceOptionalFeature
		s.Name, s.Known = clientReleases[o.Build]
		if !s.Known {
			family := "Windows 10"
			if o.Build >= firstWindows11 {
				family = "Windows 11"
			}
			s.Name = fmt.Sprintf("%s build %d", family, o.Build)
		}
	default:
		return System{}, fmt.Errorf("unknown product type %d", o.ProductType)
	}
	return s, nil
}
