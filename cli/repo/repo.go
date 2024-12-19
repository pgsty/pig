package repo

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"pig/cli/get"
	"pig/internal/config"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/openpgp/armor"
)

const (
	EL           = "rpm"
	Debian       = "deb"
	PigstyGPGKey = `-----BEGIN PGP PUBLIC KEY BLOCK-----

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
/c7w
=4rRi
-----END PGP PUBLIC KEY BLOCK-----
`
	pigstyRpmRepoPath = "/etc/yum.repos.d/pigsty.repo"
	pigstyRpmRepo     = `# Pigsty EL Repo
[pigsty-infra]
name=Pigsty Infra for $basearch
baseurl=https://repo.pigsty.io/yum/infra/$basearch
skip_if_unavailable = 1
enabled = 1
priority = 1
gpgcheck = 1
gpgkey=file:///etc/pki/rpm-gpg/RPM-GPG-KEY-pigsty
module_hotfixes=1

[pigsty-pgsql]
name=Pigsty PGSQL For el$releasever.$basearch
baseurl=https://repo.pigsty.io/yum/pgsql/el$releasever.$basearch
skip_if_unavailable = 1
enabled = 1
priority = 1
gpgcheck = 1
gpgkey=file:///etc/pki/rpm-gpg/RPM-GPG-KEY-pigsty
module_hotfixes=1
`
	pigstyDebRepoPath = "/etc/apt/sources.list.d/pigsty.list"
	pigstyDebRepo     = `# Pigsty Debian Repo
deb [signed-by=/etc/apt/keyrings/pigsty.gpg] https://repo.pigsty.io/apt/infra generic main 
deb [signed-by=/etc/apt/keyrings/pigsty.gpg] https://repo.pigsty.io/apt/pgsql/${distro_codename} ${distro_codename} main
`

	rpmRepoTmpl = ``
	debRepoTmpl = ``
)

var (
	RpmRepos []Repository
	DebRepos []Repository

	RpmRepoMap map[string]*Repository
	DebRepoMap map[string]*Repository

	RpmModuleMap map[string][]string
	DebModuleMap map[string][]string
)

// Repository represents a package repository configuration
type Repository struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Module      string            `yaml:"module"`
	Releases    []string          `yaml:"releases"`
	Arch        []string          `yaml:"arch"`
	BaseURL     map[string]string `yaml:"baseurl"`
	Distro      string            `yaml:"-"` // el|deb
}

func (r *Repository) SupportAmd64() bool {
	return slices.Contains(r.Arch, "x86_64")
}

func (r *Repository) SupportArm64() bool {
	return slices.Contains(r.Arch, "aarch64")
}

func (r *Repository) String() string {
	// dump this  to one-line json
	json, err := json.Marshal(r)
	if err != nil {
		return fmt.Sprintf("%s: %s", r.Name, r.Description)
	}
	return string(json)
}

func (r *Repository) FilePath() string {
	if r.Distro == Debian {
		return fmt.Sprintf("/etc/apt/sources.list.d/%s.list", r.Name)
	}
	if r.Distro == EL {
		return fmt.Sprintf("/etc/yum.repos.d/%s.repo", r.Name)
	}
	return ""
}

// Repositories holds all repository configurations
var Repositories = []Repository{
	{
		Name: "pigsty-pgsql", Description: "Pigsty PgSQL", Module: "pgsql", Distro: Debian,
		Releases: []string{"d11", "d12", "u20", "u22", "u24"}, Arch: []string{"x86_64", "aarch64"},
		BaseURL: map[string]string{
			"default": "https://repo.pigsty.io/apt/pgsql/${distro_codename} ${distro_codename} main",
			"china":   "https://repo.pigsty.cc/apt/pgsql/${distro_codename} ${distro_codename} main",
		},
	},
	{
		Name: "pigsty-infra", Description: "Pigsty Infra", Module: "infra", Distro: Debian,
		Releases: []string{"d11", "d12", "u20", "u22", "u24"}, Arch: []string{"x86_64", "aarch64"},
		BaseURL: map[string]string{
			"default": "https://repo.pigsty.io/apt/infra/ generic main",
			"china":   "https://repo.pigsty.cc/apt/infra/ generic main",
		},
	},
	{
		Name: "pigsty-infra", Description: "Pigsty INFRA", Module: "infra", Distro: EL,
		Releases: []string{"el7", "el8", "el9"}, Arch: []string{"x86_64", "aarch64"},
		BaseURL: map[string]string{
			"default": "https://repo.pigsty.io/yum/infra/$basearch",
			"china":   "https://repo.pigsty.cc/yum/infra/$basearch",
		},
	},
	{
		Name: "pigsty-pgsql", Description: "Pigsty PGSQL", Module: "pgsql", Distro: EL,
		Releases: []string{"el7", "el8", "el9"}, Arch: []string{"x86_64", "aarch64"},
		BaseURL: map[string]string{
			"default": "https://repo.pigsty.io/yum/pgsql/el$releasever.$basearch",
			"china":   "https://repo.pigsty.cc/yum/pgsql/el$releasever.$basearch",
		},
	},
}

func (r *Repository) Available(distro string, arch string) bool {
	// convert arch to x86_64 or aarch64
	distro = strings.ToLower(distro)
	arch = strings.ToLower(arch)
	if arch == "amd64" {
		arch = "x86_64"
	} else if arch == "arm64" {
		arch = "aarch64"
	}
	if distro != "" && !slices.Contains(r.Releases, distro) {
		return false
	}
	if arch != "" && !slices.Contains(r.Arch, arch) {
		return false
	}
	return true
}

// ListRepo lists all available repositories for a given distribution and architecture
func ListRepo(distro string, arch string) {
	if distro == "" {
		distro = config.OSCode
	}
	if arch == "" {
		arch = config.OSArch
	}

	logrus.Infof("Available repositories for distro = %s, arch = %s:", distro, arch)
	for _, repo := range Repositories {
		if repo.Available(distro, arch) {
			fmt.Println(repo.String())
		}
	}
	return
}

func AddDebGPGKey() error {
	if err := os.MkdirAll("/etc/apt/keyrings", 0755); err != nil {
		return fmt.Errorf("failed to create keyrings directory: %v", err)
	}
	block, _ := armor.Decode(strings.NewReader(PigstyGPGKey))
	keyBytes, err := io.ReadAll(block.Body)
	if err != nil {
		return fmt.Errorf("failed to read GPG key: %v", err)
	}

	if err := os.WriteFile("/etc/apt/keyrings/pigsty.gpg", keyBytes, 0644); err != nil {
		return fmt.Errorf("failed to write GPG key: %v", err)
	}
	return nil
}

func AddRpmGPGKey() error {
	if err := os.WriteFile("/etc/pki/rpm-gpg/RPM-GPG-KEY-pigsty", []byte(PigstyGPGKey), 0644); err != nil {
		return fmt.Errorf("failed to write GPG key: %v", err)
	}
	return nil
}

// logrus.Infof("Import pigsty GPG key 0xB9BD8B20 to %s", keyPath)

func AddPigstyRepo() error {
	if runtime.GOOS != "linux" { // check if linux
		return fmt.Errorf("pigsty works on linux, unsupported os: %s", runtime.GOOS)
	}
	if os.Geteuid() != 0 { // check if root
		return fmt.Errorf("insufficient privileges, try again with: sudo pig repo add")
	}
	logrus.Infof("add pigsty repo for %s.%s", config.OSCode, config.OSArch)

	// check network condition
	get.Timeout = time.Second
	get.NetworkCondition() // check network condition
	if !get.InternetAccess {
		return fmt.Errorf("no internet access")
	}

	if config.OSType == "rpm" {
		if err := AddRpmGPGKey(); err != nil {
			return err
		}

		rpmRepo := pigstyRpmRepo
		if get.Source == get.ViaCC {
			logrus.Infof("use china mirror instead of default repo")
			rpmRepo = strings.ReplaceAll(rpmRepo, "repo.pigsty.io", "repo.pigsty.cc")
		}
		if err := os.WriteFile(pigstyRpmRepoPath, []byte(rpmRepo), 0644); err != nil {
			return fmt.Errorf("failed to write repo file: %v", err)
		}
		logrus.Infof("add pigsty repo file to %s, now run yum makecache ...", pigstyRpmRepoPath)

		// run yum makecache and print output
		cmd := exec.Command("sudo", "yum", "makecache")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to run yum makecache: %v", err)
		}
		return nil

	}
	if config.OSType == "deb" {
		if err := AddDebGPGKey(); err != nil {
			return err
		}
		debRepo := strings.ReplaceAll(pigstyDebRepo, "${distro_codename}", config.OSVersionCode)
		if get.Source == get.ViaCC {
			logrus.Infof("use china mirror instead of default repo")
			debRepo = strings.ReplaceAll(debRepo, "repo.pigsty.io", "repo.pigsty.cc")
		}
		if err := os.WriteFile(pigstyDebRepoPath, []byte(debRepo), 0644); err != nil {
			return fmt.Errorf("failed to write repo file: %v", err)
		}
		logrus.Infof("add pigsty repo file to %s, now run apt-get update ...", pigstyDebRepoPath)
		// run apt update and print output
		cmd := exec.Command("sudo", "apt-get", "update")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("failed to run apt update: %v", err)
		}
		return nil
	}
	return nil
}

func AddRpmModule(module ...string) error {
	fmt.Println(module)
	return nil
}

func AddDebRepo(name string, url string) error {
	return nil
}
