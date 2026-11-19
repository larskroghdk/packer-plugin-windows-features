//go:generate packer-sdc mapstructure-to-hcl2 -type Config

package provisioner

import (
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/packer-plugin-sdk/common"
	"github.com/hashicorp/packer-plugin-sdk/packer"
	"github.com/hashicorp/packer-plugin-sdk/template/config"
	"github.com/hashicorp/packer-plugin-sdk/template/interpolate"
	"github.com/larskroghdk/packer-plugin-windows-features/internal/plan"
)

const (
	defaultRestartTimeout   = 30 * time.Minute
	defaultOperationTimeout = 60 * time.Minute
	maxNameLength           = 256
)

// Config is the provisioner's HCL configuration. See docs/configuration.md for
// each option.
type Config struct {
	common.PackerConfig    `mapstructure:",squash"`
	Install                []string       `mapstructure:"install"`
	Remove                 []string       `mapstructure:"remove"`
	InstallCapabilities    []string       `mapstructure:"install_capabilities"`
	RemoveCapabilities     []string       `mapstructure:"remove_capabilities"`
	IncludeManagementTools bool           `mapstructure:"include_management_tools"`
	IncludeAllSubFeatures  bool           `mapstructure:"include_all_subfeatures"`
	Source                 string         `mapstructure:"source"`
	LimitAccess            bool           `mapstructure:"limit_access"`
	RemovePayload          bool           `mapstructure:"remove_payload"`
	IgnoreUnavailable      bool           `mapstructure:"ignore_unavailable"`
	DryRun                 bool           `mapstructure:"dry_run"`
	FailOnError            config.Trilean `mapstructure:"fail_on_error"`
	RestartIfRequired      bool           `mapstructure:"restart_if_required"`
	RestartTimeout         time.Duration  `mapstructure:"restart_timeout"`
	OperationTimeout       time.Duration  `mapstructure:"operation_timeout"`
	ReportPath             string         `mapstructure:"report_path"`
	ctx                    interpolate.Context
}

type nameList struct {
	option string
	kind   plan.Kind
	action plan.Action
	names  []string
}

// lists returns the four name lists with the option, kind and action each
// stands for.
//
// Author: Lars Krogh
// Version: 1.0.0
func (c *Config) lists() []nameList {
	return []nameList{
		{"remove", plan.KindFeature, plan.ActionRemove, c.Remove},
		{"install", plan.KindFeature, plan.ActionInstall, c.Install},
		{"remove_capabilities", plan.KindCapability, plan.ActionRemove, c.RemoveCapabilities},
		{"install_capabilities", plan.KindCapability, plan.ActionInstall, c.InstallCapabilities},
	}
}

// prepare decodes and interpolates the raw configuration, applies defaults and
// validates every option. A name that appears twice, or in both an install
// and a remove list, is an error, so contradictory requests never reach the
// guest.
//
// Returns:
//   - error: All validation errors, combined.
//
// Author: Lars Krogh
// Version: 1.0.0
func (c *Config) prepare(raws ...any) error {
	err := config.Decode(c, &config.DecodeOpts{
		PluginType:         "windows-features",
		Interpolate:        true,
		InterpolateContext: &c.ctx,
	}, raws...)
	if err != nil {
		return err
	}
	var errs error
	add := func(format string, args ...any) { errs = packer.MultiErrorAppend(errs, fmt.Errorf(format, args...)) }
	if c.RestartTimeout == 0 {
		c.RestartTimeout = defaultRestartTimeout
	}
	if c.OperationTimeout == 0 {
		c.OperationTimeout = defaultOperationTimeout
	}
	if c.ReportPath == "" {
		name := c.PackerBuildName
		if name == "" {
			name = "build"
		}
		c.ReportPath = fmt.Sprintf("windows-features-report-%s.json", sanitizeFileName(name))
	}
	total := 0
	seen := map[string]string{}
	for _, l := range c.lists() {
		total += len(l.names)
		for i, name := range l.names {
			where := fmt.Sprintf("%s[%d]", l.option, i)
			if why := checkName(name, l.kind); why != "" {
				add("%s: %s", where, why)
				continue
			}
			key := string(l.kind) + ":" + strings.ToLower(name)
			if first, dup := seen[key]; dup {
				add("%s: %q is already listed in %s; a name can be requested only once", where, name, first)
				continue
			}
			seen[key] = where
		}
	}
	if total == 0 {
		add("nothing to do: list at least one name in install, remove, install_capabilities or remove_capabilities")
	}
	if c.LimitAccess && c.Source == "" {
		add("limit_access: without source, Windows has no payload to install from; set source or remove limit_access")
	}
	if c.RemovePayload && len(c.Remove) == 0 {
		add("remove_payload: applies to the remove list, which is empty")
	}
	if c.IncludeAllSubFeatures && len(c.Install) == 0 {
		add("include_all_subfeatures: applies to the install list, which is empty")
	}
	if strings.ContainsAny(c.Source, "\r\n\x00") {
		add("source: must be a single path")
	}
	if c.RestartTimeout < time.Minute {
		add("restart_timeout: must be at least 1m")
	}
	if c.OperationTimeout < time.Minute {
		add("operation_timeout: must be at least 1m")
	}
	return errs
}

// checkName returns why name cannot be requested, or "".
//
// Author: Lars Krogh
// Version: 1.0.0
func checkName(name string, kind plan.Kind) string {
	switch {
	case strings.TrimSpace(name) == "":
		return "name is empty"
	case strings.TrimSpace(name) != name:
		return fmt.Sprintf("%q has leading or trailing spaces", name)
	case len(name) > maxNameLength:
		return fmt.Sprintf("name is longer than %d characters", maxNameLength)
	case strings.ContainsAny(name, "*?[]"):
		return fmt.Sprintf("%q contains a wildcard; list every name explicitly so the plan is predictable", name)
	case strings.IndexFunc(name, func(r rune) bool { return r < 0x20 || r == '"' || r == '\'' || r == '`' }) >= 0:
		return fmt.Sprintf("%q contains a control character or quote", name)
	case kind == plan.KindFeature && strings.Contains(name, "~"):
		return fmt.Sprintf("%q is a capability name; list it in install_capabilities or remove_capabilities", name)
	}
	return ""
}

// requests returns every requested name as a plan request, removals first.
//
// Author: Lars Krogh
// Version: 1.0.0
func (c *Config) requests() []plan.Request {
	var out []plan.Request
	for _, l := range c.lists() {
		for _, name := range l.names {
			out = append(out, plan.Request{Name: name, Kind: l.kind, Action: l.action})
		}
	}
	return out
}

// failOnError reports whether unsuccessful requests fail the build. It is true
// unless fail_on_error is explicitly false.
//
// Author: Lars Krogh
// Version: 1.0.0
func (c *Config) failOnError() bool { return !c.FailOnError.False() }

// hasKind reports whether any request is of kind k.
//
// Author: Lars Krogh
// Version: 1.0.0
func (c *Config) hasKind(k plan.Kind) bool {
	for _, l := range c.lists() {
		if l.kind == k && len(l.names) > 0 {
			return true
		}
	}
	return false
}

// sanitizeFileName replaces every character other than ASCII letters, digits,
// '-', '_' and '.' with '_'.
//
// Author: Lars Krogh
// Version: 1.0.0
func sanitizeFileName(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		}
		return '_'
	}, s)
}
