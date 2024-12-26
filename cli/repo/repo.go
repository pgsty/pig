package repo

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"pig/internal/config"
	"slices"
	"sort"
	"strings"
)

var (
	//go:embed assets/rpm.yml
	embedRpmRepo []byte

	//go:embed assets/deb.yml
	embedDebRepo []byte

	//go:embed assets/key.gpg
	embedGPGKey []byte
)

const (
	pigstyRpmGPGPath = "/etc/pki/rpm-gpg/RPM-GPG-KEY-pigsty"
	pigstyDebGPGPath = "/etc/apt/keyrings/pigsty.gpg"
)

/********************
* Global Vars
********************/

var (
	Repos     []*Repository
	RepoMap   map[string]*Repository
	ModuleMap map[string][]string = make(map[string][]string)
)

/********************
* Repo Data Type
********************/

// Repository represents a package repository configuration
type Repository struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Module      string            `yaml:"module"`
	Releases    []int             `yaml:"releases"`
	Arch        []string          `yaml:"arch"`
	BaseURL     map[string]string `yaml:"baseurl"`
	Meta        map[string]string `yaml:"-"`
	Distro      string            `yaml:"-"` // el|deb
}

func (r *Repository) SupportAmd64() bool {
	return slices.Contains(r.Arch, "x86_64")
}

func (r *Repository) SupportArm64() bool {
	return slices.Contains(r.Arch, "aarch64")
}

func (r *Repository) GetBaseURL(region string) string {
	if url, ok := r.BaseURL[region]; ok {
		return url
	}
	return r.BaseURL["default"]
}

// Available checks if the repository is available for a given distribution and architecture
func (r *Repository) Available(code string, arch string) bool {
	code = strings.ToLower(code)
	arch = strings.ToLower(arch)
	if arch == "amd64" || arch == "x86_64" {
		// convert arch to x86_64 or aarch64
		arch = "x86_64"
	} else if arch == "arm64" || arch == "aarch64" {
		arch = "aarch64"
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

func (r *Repository) String() string {
	json, err := json.Marshal(r)
	if err != nil {
		return fmt.Sprintf("%s: %s", r.Name, r.Description)
	}
	return string(json)
}

func (r *Repository) Content(region ...string) string {
	regionStr := "default"
	if len(region) > 0 {
		regionStr = region[0]
	}
	if r.Distro == config.DistroEL {
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
	}
	if r.Distro == config.DistroDEB {
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
