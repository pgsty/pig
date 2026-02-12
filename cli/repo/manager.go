package repo

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"pig/cli/get"
	"pig/internal/config"
	"slices"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"

	"gopkg.in/yaml.v3"
)

//go:embed assets/repo.yml
var embedRepoData []byte

// Manager represents a package repository manager
type Manager struct {
	Data           []*Repository
	List           []*Repository
	Map            map[string]*Repository
	Module         map[string][]string
	Region         string
	OsDistroCode   string
	OsType         string
	OsArch         string
	OsMajorVersion int
	RepoDir        string
	RepoPattern    string
	BackupDir      string
	UpdateCmd      []string
	DataSource     string
}

// NewManager creates a new repo Manager
func NewManager(paths ...string) (m *Manager, err error) {
	m = &Manager{
		List:       make([]*Repository, 0),
		Map:        make(map[string]*Repository),
		Module:     make(map[string][]string),
		DataSource: "embedded",
	}
	m.OsDistroCode = strings.ToLower(config.OSCode)
	m.OsMajorVersion = GetMajorVersionFromCode(m.OsDistroCode)
	m.OsType = config.OSType
	m.OsArch = config.OSArch
	m.Region = "default"

	switch config.OSType {
	case config.DistroEL:
		m.RepoDir = "/etc/yum.repos.d"
		m.BackupDir = "/etc/yum.repos.d/backup"
		m.RepoPattern = "/etc/yum.repos.d/*.repo"
		m.UpdateCmd = []string{"yum", "makecache"}
	case config.DistroDEB:
		m.RepoDir = "/etc/apt/sources.list.d"
		m.BackupDir = "/etc/apt/sources.list.d/backup"
		m.RepoPattern = "/etc/apt/sources.list.d/*.list"
		m.UpdateCmd = []string{"apt-get", "update"}
	default:
		m.RepoDir = "/tmp/"
		m.BackupDir = "/tmp/repo-backup"
	}

	var data []byte
	var defaultCsvPath string
	if config.ConfigDir != "" {
		defaultCsvPath = filepath.Join(config.ConfigDir, "repo.yml")
		if !slices.Contains(paths, defaultCsvPath) {
			paths = append(paths, defaultCsvPath)
		}
	}
	for _, path := range paths {
		if fileData, err := os.ReadFile(path); err == nil {
			data = fileData
			m.DataSource = path
			break
		}
	}
	if err := m.LoadData(data); err != nil {
		if m.DataSource != defaultCsvPath {
			logrus.Debugf("failed to load repo data from %s, using embedded", m.DataSource)
		} else {
			logrus.Debugf("repo data not found at default path, using embedded")
		}
		if err = m.LoadData(nil); err != nil {
			logrus.Errorf("failed to parse embedded repo data: %v", err)
			return nil, fmt.Errorf("failed to load repo configuration: %w", err)
		}
		m.DataSource = "embedded"
	}
	logrus.Debugf("repo data loaded: %s", m.DataSource)
	return m, nil
}

// LoadData loads repository configurations for a given OS type
func (m *Manager) LoadData(data []byte) error {
	if data == nil {
		logrus.Debugf("load repo with nil data, fallback to embedded repo.yml")
		data = embedRepoData
	}

	var tmpData []Repository
	if err := yaml.Unmarshal(data, &tmpData); err != nil {
		return fmt.Errorf("failed to parse %s repo: %v", m.OsType, err)
	}

	// Filter available repos and build maps
	m.Data = make([]*Repository, 0)
	m.List = make([]*Repository, 0)
	m.Map = make(map[string]*Repository)
	m.Module = make(map[string][]string)
	for i := range tmpData {
		repoPointer := &tmpData[i]
		repoPointer.Distro = repoPointer.InferOS()
		m.Data = append(m.Data, repoPointer)
	}

	for _, repo := range m.Data {
		repo.Distro = repo.InferOS()
		switch repo.Distro {
		case config.DistroEL:
			// Intentionally permissive default for compatibility with offline/mirror repos.
			// See config.PigstyGPGCheck for opt-in signature enforcement.
			meta := map[string]string{"enabled": "1", "gpgcheck": "0", "module_hotfixes": "1"}
			if repo.Meta != nil {
				for k, v := range repo.Meta {
					meta[k] = v
				}
			}
			repo.Meta = meta
		case config.DistroDEB:
			// Intentionally permissive default for compatibility with offline/mirror repos.
			// See config.PigstyGPGCheck for opt-in signature enforcement.
			meta := map[string]string{"trusted": "yes"}
			if repo.Meta != nil {
				for k, v := range repo.Meta {
					meta[k] = v
				}
			}
			repo.Meta = meta
		default:
			logrus.Debugf("found unsupported distro in repo %s: %v", repo.Name, repo)
		}

		// It's user's responsibility to ensure the repo name is unique for all the combinations of os, arch
		if repo.Available(m.OsDistroCode, m.OsArch) {
			m.List = append(m.List, repo)
			m.Map[repo.Name] = repo
		}
		if repo.Module != "" {
			if _, exists := m.Module[repo.Module]; !exists {
				m.Module[repo.Module] = make([]string, 0)
			}
			// append repo name to module list if not already present
			if !slices.Contains(m.Module[repo.Module], repo.Name) {
				m.Module[repo.Module] = append(m.Module[repo.Module], repo.Name)
			}
		}
	}

	m.addDefaultModules()
	m.adjustPigstyRepoMeta()

	logrus.Debugf("load %d %s repo, %d modules", len(m.Map), m.OsType, len(m.Module))
	return nil
}

// adjustRepoMeta adjusts the repository metadata
func (m *Manager) addDefaultModules() {
	switch m.OsType {
	case config.DistroEL:
		m.Module["pigsty"] = []string{"pigsty-infra", "pigsty-pgsql"}
		m.Module["pgdg"] = []string{"pgdg-common", "pgdg-el8fix", "pgdg-el9fix", "pgdg18", "pgdg17", "pgdg16", "pgdg15", "pgdg14", "pgdg13"}
		m.Module["all"] = append(m.Module["pigsty"], append(m.Module["pgdg"], m.Module["node"]...)...)
	case config.DistroDEB:
		m.Module["pigsty"] = []string{"pigsty-infra", "pigsty-pgsql"}
		m.Module["pgdg"] = []string{"pgdg"}
		m.Module["all"] = append(m.Module["pigsty"], append(m.Module["pgdg"], m.Module["node"]...)...)
	default:
		m.Module["pigsty"] = []string{"pigsty-infra", "pigsty-pgsql"}
		m.Module["pgdg"] = m.Module["pgsql"]
		m.Module["all"] = append([]string{"pigsty-infra"}, append(m.Module["pgsql"], m.Module["node"]...)...)
	}
}

// adjustPigstyRepoMeta adjusts the Pigsty repository metadata if use GPG flag is set
func (m *Manager) adjustPigstyRepoMeta() {
	if !config.PigstyGPGCheck {
		return
	}
	if repo, ok := m.Map["pigsty-pgsql"]; ok {
		if m.OsType == config.DistroEL {
			repo.Meta["gpgcheck"] = "1"
			repo.Meta["gpgkey"] = "file:///etc/pki/rpm-gpg/RPM-GPG-KEY-pigsty"
		} else if m.OsType == config.DistroDEB {
			delete(repo.Meta, "trusted")
			repo.Meta["signed-by"] = "/etc/apt/keyrings/pigsty.gpg"
		}
	}
	if repo, ok := m.Map["pigsty-infra"]; ok {
		if m.OsType == config.DistroEL {
			repo.Meta["gpgcheck"] = "1"
			repo.Meta["gpgkey"] = "file:///etc/pki/rpm-gpg/RPM-GPG-KEY-pigsty"
		} else if m.OsType == config.DistroDEB {
			delete(repo.Meta, "trusted")
			repo.Meta["signed-by"] = "/etc/apt/keyrings/pigsty.gpg"
		}
	}
}

// ModuleOrder returns the order of modules in given precedence
func (m *Manager) ModuleOrder() []string {
	// Define the desired order of specific modules
	desiredOrder := []string{"all", "pigsty", "pgdg", "node", "infra", "pgsql", "extra", "mssql", "mysql", "kube", "grafana", "pgml"}

	// Create a map to store the index of each module in the desired order
	orderMap := make(map[string]int)
	for i, module := range desiredOrder {
		orderMap[module] = i
	}

	// Collect all modules from m.Module
	modules := make([]string, 0, len(m.Module))
	for module := range m.Module {
		modules = append(modules, module)
	}

	// Sort the modules based on the desired order
	sort.Slice(modules, func(i, j int) bool {
		indexI, okI := orderMap[modules[i]]
		indexJ, okJ := orderMap[modules[j]]
		if okI && okJ {
			return indexI < indexJ
		}
		if okI {
			return true
		}
		if okJ {
			return false
		}
		return modules[i] < modules[j]
	})

	return modules
}

// DetectRegion if region is given, use it, otherwise detect from network condition
func (m *Manager) DetectRegion(region string) {
	if region != "" {
		m.Region = region
		logrus.Debugf("using specified region: %s", region)
		return
	}
	get.NetworkCondition()
	if !get.InternetAccess {
		logrus.Warnf("no internet access, using default region")
		m.Region = "default"
	} else {
		m.Region = get.Region
		logrus.Debugf("detected region: %s", m.Region)
	}
}
