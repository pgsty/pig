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
	Distro      string            `yaml:"-"` // el|deb
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
	desc := fmt.Sprintf("'%s'", r.Description) // description 里加上单引号
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
	if slices.Contains(r.Releases, 11) || slices.Contains(r.Releases, 12) || slices.Contains(r.Releases, 20) || slices.Contains(r.Releases, 22) || slices.Contains(r.Releases, 24) {
		return config.DistroDEB
	}
	if slices.Contains(r.Releases, 7) || slices.Contains(r.Releases, 8) || slices.Contains(r.Releases, 9) {
		return config.DistroEL
	}

	// Infer from base URL if releases do not provide enough information
	for _, url := range r.BaseURL {
		if strings.Contains(url, "debian") || strings.Contains(url, "ubuntu") || strings.Contains(url, "/deb/") || strings.Contains(url, "/apt/") {
			return config.DistroDEB
		}
		if strings.Contains(url, "centos") || strings.Contains(url, "redhat") || strings.Contains(url, "fedora") || strings.Contains(url, "/yum/") || strings.Contains(url, "/rpm/") {
			return config.DistroEL
		}
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
		return fmt.Sprintf("[%s]\nname=%s\nbaseurl=%s\n%s", r.Name, r.Name, r.GetBaseURL(regionStr), rpmMeta)

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
