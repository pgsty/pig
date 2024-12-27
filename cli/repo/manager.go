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

	_ "embed"

	"gopkg.in/yaml.v3"
)

//go:embed assets/repo.yml
var embedRepoData []byte

var Manager *RepoManager

// RepoManager represents a package repository configuration
type RepoManager struct {
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
	DataSource     string
}

// NewRepoManager creates a new RepoManager
func NewRepoManager(paths ...string) (rm *RepoManager, err error) {
	rm = &RepoManager{
		List:   make([]*Repository, 0),
		Map:    make(map[string]*Repository),
		Module: make(map[string][]string),
	}
	rm.OsDistroCode = strings.ToLower(config.OSCode)
	rm.OsMajorVersion = GetMajorVersionFromCode(rm.OsDistroCode)
	rm.OsType = config.OSType
	rm.OsArch = config.OSArch
	rm.Region = "default"

	switch config.OSType {
	case config.DistroEL:
		rm.RepoDir = "/etc/yum.repos.d"
		rm.BackupDir = "/etc/yum.repos.d/backup"
		rm.RepoPattern = "/etc/yum.repos.d/*.repo"
	case config.DistroDEB:
		rm.RepoDir = "/etc/apt/sources.list.d"
		rm.BackupDir = "/etc/apt/sources.list.d/backup"
		rm.RepoPattern = "/etc/apt/sources.list.d/*.list"
	default:
		rm.RepoDir = "/tmp/"
		rm.RepoDir = "/tmp/repo-backup"
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
			rm.DataSource = path
			break
		}
	}
	if err := rm.LoadRepo(data); err != nil {
		if rm.DataSource != defaultCsvPath {
			logrus.Debugf("failed to load extension data from %s: %v, fallback to embedded data", rm.DataSource, err)
		} else {
			logrus.Debugf("failed to load extension data from default path: %s, fallback to embedded data", defaultCsvPath)
		}
		err = rm.LoadRepo(nil)
		rm.DataSource = "embedded"
		if err != nil {
			logrus.Debugf("not likely to happen: failed on parsing embedded data: %v", err)
		}
		return nil, err

	}
	logrus.Debugf("load extension data from %s", rm.DataSource)
	return rm, nil
}

// LoadRepo loads repository configurations for a given OS type
func (rm *RepoManager) LoadRepo(data []byte) error {
	if data == nil {
		logrus.Debugf("load repo with nil data, fallback to embedded repo.yml")
		data = embedRepoData
	}

	var tmpData []Repository
	if err := yaml.Unmarshal(data, &tmpData); err != nil {
		return fmt.Errorf("failed to parse %s repo: %v", rm.OsType, err)
	}

	// Filter available repos and build maps
	rm.Data = make([]*Repository, 0)
	rm.List = make([]*Repository, 0)
	rm.Map = make(map[string]*Repository)
	rm.Module = make(map[string][]string)
	for i := range tmpData {
		repoPointer := &tmpData[i]
		repoPointer.Distro = repoPointer.InferOS()
		rm.Data = append(rm.Data, repoPointer)
	}

	for _, repo := range rm.Data {
		repo.Distro = repo.InferOS()
		switch repo.Distro {
		case config.DistroEL:
			meta := map[string]string{"enabled": "1", "gpgcheck": "0", "module_hotfixes": "1"}
			if repo.Meta != nil {
				for k, v := range repo.Meta {
					meta[k] = v
				}
			}
			repo.Meta = meta
		case config.DistroDEB:
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
		if repo.Available(rm.OsDistroCode, rm.OsArch) {
			rm.List = append(rm.List, repo)
			rm.Map[repo.Name] = repo
			if repo.Module != "" {
				if _, exists := rm.Module[repo.Module]; !exists {
					rm.Module[repo.Module] = make([]string, 0)
				}
				rm.Module[repo.Module] = append(rm.Module[repo.Module], repo.Name)
			}
		}
	}

	rm.addDefaultModules()
	rm.adjustPigstyRepoMeta()

	logrus.Debugf("load %d %s repo, %d modules", len(rm.Map), rm.OsType, len(rm.Module))
	return nil
}

// adjustRepoMeta adjusts the repository metadata
func (rm *RepoManager) addDefaultModules() {
	if rm.OsType == config.DistroEL {
		rm.Module["pigsty"] = []string{"pigsty-infra", "pigsty-pgsql"}
		rm.Module["pgdg"] = []string{"pgdg-common", "pgdg-el8fix", "pgdg-el9fix", "pgdg17", "pgdg16", "pgdg15", "pgdg14", "pgdg13"}
		rm.Module["all"] = append(rm.Module["pigsty"], append(rm.Module["pgdg"], rm.Module["node"]...)...)
	} else if rm.OsType == config.DistroDEB {
		rm.Module["pigsty"] = []string{"pigsty-infra", "pigsty-pgsql"}
		rm.Module["pgdg"] = []string{"pgdg"}
		rm.Module["all"] = append(rm.Module["pigsty"], append(rm.Module["pgdg"], rm.Module["node"]...)...)
	}
}

// adjustPigstyRepoMeta adjusts the Pigsty repository metadata if use GPG flag is set
func (rm *RepoManager) adjustPigstyRepoMeta() {
	if !config.PigstyGPGCheck {
		return
	}
	if repo, ok := rm.Map["pigsty-pgsql"]; ok {
		if rm.OsType == config.DistroEL {
			repo.Meta["gpgcheck"] = "1"
			repo.Meta["gpgkey"] = "file:///etc/pki/rpm-gpg/RPM-GPG-KEY-pigsty"
		} else if rm.OsType == config.DistroDEB {
			delete(repo.Meta, "trusted")
			repo.Meta["signed-by"] = "/etc/apt/keyrings/pigsty.gpg"
		}
	}
	if repo, ok := rm.Map["pigsty-infra"]; ok {
		if rm.OsType == config.DistroEL {
			repo.Meta["gpgcheck"] = "1"
			repo.Meta["gpgkey"] = "file:///etc/pki/rpm-gpg/RPM-GPG-KEY-pigsty"
		} else if rm.OsType == config.DistroDEB {
			delete(repo.Meta, "trusted")
			repo.Meta["signed-by"] = "/etc/apt/keyrings/pigsty.gpg"
		}
	}
}

// ModuleOrder returns the order of modules in given precedence
func (rm *RepoManager) ModuleOrder() []string {
	// Define the desired order of specific modules
	desiredOrder := []string{"all", "pigsty", "pgdg", "node", "infra", "pgsql", "extra", "mssql", "mysql", "docker", "kube", "grafana", "pgml"}

	// Create a map to store the index of each module in the desired order
	orderMap := make(map[string]int)
	for i, module := range desiredOrder {
		orderMap[module] = i
	}

	// Collect all modules from rm.Module
	modules := make([]string, 0, len(rm.Module))
	for module := range rm.Module {
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

// if region is given, use it, otherwise detect from network condition
func (rm *RepoManager) DetectRegion(region string) {
	if region != "" {
		rm.Region = region
		return
	}
	get.NetworkCondition()
	if !get.InternetAccess {
		logrus.Warn("no internet access, assume region = default")
		rm.Region = "default"
	} else {
		rm.Region = get.Region
	}
}
