package repo

import (
	_ "embed"
	"fmt"
	"pig/internal/config"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

// Repository represents a package repository configuration
type Repository struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Module      string            `yaml:"module"`
	Releases    []int             `yaml:"releases"`
	Arch        []string          `yaml:"arch"`
	BaseURL     map[string]string `yaml:"baseurl"`
	Meta        map[string]string `yaml:"meta"`
	Minor       bool              `yaml:"minor"` // if true, use full version (e.g. 9.6) instead of major (e.g. 9) in $releasever
	Distro      string            `yaml:"-"`     // el|deb
}

// SupportAmd64 checks if the repository supports amd64 architecture
func (r *Repository) SupportAmd64() bool {
	return slices.Contains(r.Arch, "x86_64")
}

// SupportArm64 checks if the repository supports arm64 architecture
func (r *Repository) SupportArm64() bool {
	return slices.Contains(r.Arch, "aarch64")
}

// ToInlineYAML Will output a single line yaml string
func (r *Repository) ToInlineYAML() string {
	name := r.Name
	desc := fmt.Sprintf("'%s'", r.Description) // add single quotes to description
	module := r.Module
	releases := compactIntArray(r.Releases)
	arch := compactStrArray(r.Arch)
	return fmt.Sprintf("- { name: %-14s ,description: %-20s ,module: %-8s ,releases: %-16s ,arch: %-18s ,baseurl: '%s' }",
		name, desc, module, releases, arch, r.BaseURL["default"])
}

// InferOS infers the OS type from the repository releases fields and base URL
func (r *Repository) InferOS() string {
	if len(r.Releases) == 0 {
		return ""
	}

	for _, rel := range r.Releases {
		switch rel {
		case 11, 12, 13, 20, 22, 24:
			return config.DistroDEB
		case 7, 8, 9, 10:
			return config.DistroEL
		}
	}

	// Infer from base URL if releases do not provide enough information
	for _, url := range r.BaseURL {
		if distro := inferOSFromURL(url); distro != "" {
			return distro
		}
	}
	return ""
}

func inferOSFromURL(url string) string {
	u := strings.ToLower(url)
	if strings.Contains(u, "debian") || strings.Contains(u, "ubuntu") || strings.Contains(u, "/deb/") || strings.Contains(u, "/apt/") {
		return config.DistroDEB
	}
	if strings.Contains(u, "centos") || strings.Contains(u, "redhat") || strings.Contains(u, "fedora") || strings.Contains(u, "/yum/") || strings.Contains(u, "/rpm/") {
		return config.DistroEL
	}
	return ""
}

// GetBaseURL returns the base URL for given regions, tries one by one and falls back to default
func (r *Repository) GetBaseURL(regions ...string) string {
	for _, region := range regions {
		if url, ok := r.BaseURL[region]; ok {
			return url
		}
	}
	return r.BaseURL["default"]
}

// AvailableInCurrentOS checks if the repository is available for the current OS
func (r *Repository) AvailableInCurrentOS() bool {
	return r.Available(config.OSCode, config.OSArch)
}

// Available checks if the repository is available for a given distribution and architecture
func (r *Repository) Available(code string, arch string) bool {
	code = strings.ToLower(code)
	arch = strings.ToLower(arch)
	switch arch {
	case "amd64", "x86_64":
		arch = "x86_64" // convert arch to x86_64 or aarch64
	case "arm64", "aarch64", "arm64v8":
		arch = "aarch64" // convert arm64 arch to aarch64
	}
	if config.OSType == config.DistroMAC {
		return false
	}
	major := GetMajorVersionFromCode(code)
	if code != "" && (major == -1 || !slices.Contains(r.Releases, major)) {
		return false
	}
	if arch != "" && !slices.Contains(r.Arch, arch) {
		return false
	}
	return true
}

// useMinorVersion checks if this repo should use full minor version (e.g. 9.6) instead of major (e.g. 9) in $releasever
// This is required for:
// 1. Repos with Minor=true explicitly set
// 2. EPEL repos on EL10+ where EPEL started building for specific minor versions
// 3. PGDG repos on specific EL versions (9.6, 9.7, 10.0, 10.1) where PGDG builds for minor versions
func (r *Repository) useMinorVersion() bool {
	// Only applies to EL (RPM) systems
	if config.OSType != config.DistroEL {
		return false
	}
	// Explicit minor flag takes precedence
	if r.Minor {
		return true
	}
	// Auto-enable for EPEL on EL10+
	if config.OSMajor >= 10 && strings.HasPrefix(strings.ToLower(r.Name), "epel") {
		return true
	}
	// Auto-enable for PGDG repos on specific EL versions that require minor version
	if strings.HasPrefix(strings.ToLower(r.Name), "pgdg") {
		switch config.OSVersionFull {
		case "9.6", "9.7", "10.0", "10.1":
			return true
		}
	}
	return false
}

// Content returns the repo file content for a given region
func (r *Repository) Content(region ...string) string {
	regionStr := "default"
	if len(region) > 0 {
		regionStr = region[0]
	}
	if r.Distro == "" {
		return ""
	}
	switch r.Distro {
	case config.DistroEL:
		logrus.Debugf("generate EL repo content for %s.%s", r.Name, r.Distro)
		rpmMeta := ""
		// Get sorted keys
		keys := make([]string, 0, len(r.Meta))
		for k := range r.Meta {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		// Add meta in sorted order
		for _, k := range keys {
			rpmMeta += fmt.Sprintf("%s=%s\n", k, r.Meta[k])
		}

		baseURL := r.GetBaseURL(regionStr)
		// Substitute $releasever with full version (e.g. 9.6, 10.0) if minor version is required
		// This is needed for EPEL on EL10+ and repos with Minor=true
		if r.useMinorVersion() {
			logrus.Debugf("substituting $releasever with %s for repo %s", config.OSVersionFull, r.Name)
			baseURL = strings.ReplaceAll(baseURL, "$releasever", config.OSVersionFull)
		}

		return fmt.Sprintf("[%s]\nname=%s\nbaseurl=%s\n%s", r.Name, r.Name, baseURL, rpmMeta)

	case config.DistroDEB:
		logrus.Debugf("generate DEB repo content for %s.%s", r.Name, r.Distro)
		// Get sorted keys
		keys := make([]string, 0, len(r.Meta))
		for k := range r.Meta {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		// Build meta string in sorted order
		debMeta := ""
		for _, k := range keys {
			debMeta += fmt.Sprintf("%s=%s ", k, r.Meta[k])
		}
		debMeta = strings.TrimSpace(debMeta)
		repoURL := r.GetBaseURL(regionStr)
		repoURL = strings.ReplaceAll(repoURL, "${distro_codename}", config.OSVersionCode)
		repoURL = strings.ReplaceAll(repoURL, "${distro_name}", config.OSVendor)
		return fmt.Sprintf("# %s %s\ndeb [%s] %s", r.Name, r.Description, debMeta, repoURL)
	}
	return ""
}

// compactArray formats the releases to a compact inline string
func compactIntArray(rs []int) string {
	if len(rs) == 0 {
		return "[]"
	}
	s := make([]string, len(rs))
	for i, v := range rs {
		s[i] = strconv.Itoa(v)
	}
	return "[" + strings.Join(s, ",") + "]"
}

// compactStrArray formats the releases to a compact inline string
func compactStrArray(a []string) string {
	if len(a) == 0 {
		return "[]"
	}
	return "[" + strings.Join(a, ", ") + "]"
}

// Info prints the information of a repository
func (r *Repository) Info() string {
	metaInfo := ""
	if r.Meta != nil {
		for key, value := range r.Meta {
			metaInfo += fmt.Sprintf("%s=%s ", key, value)
		}
	}
	availInfo := fmt.Sprintf("No (%s %s %s)", config.OSVendor, config.OSCode, config.OSArch)
	if r.AvailableInCurrentOS() {
		availInfo = fmt.Sprintf("Yes (%s %s %s)", config.OSVendor, config.OSCode, config.OSArch)
	}
	info := fmt.Sprintf(
		"Name       : %s\n"+
			"Summary    : %s\n"+
			"Available  : %s\n"+
			"Module     : %s\n"+
			"OS Arch    : %s\n"+
			"OS Distro  : %s\n"+
			"Meta       : %s\n"+
			"Base URL   : %s\n",
		r.Name, r.Description, availInfo, r.Module, compactStrArray(r.Arch), r.InferOS()+" "+compactIntArray(r.Releases), metaInfo, r.BaseURL["default"])
	if len(r.BaseURL) > 1 {
		for key, value := range r.BaseURL {
			if key != "default" {
				// replace continues space into one space of value
				if strings.Contains(value, " ") {
					value = strings.Join(strings.Fields(value), " ")
				}
				info += fmt.Sprintf("%10s : %s\n", key, value)
			}
		}
	}

	info += fmt.Sprintf("\n# default repo content\n%s\n", r.Content("default"))
	if len(r.BaseURL) > 1 {
		for region := range r.BaseURL {
			if region != "default" {
				info += fmt.Sprintf("\n# %s mirror repo content\n%s\n", region, r.Content(region))
			}
		}
	}

	return info
}
