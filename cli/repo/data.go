package repo

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	"pig/internal/config"
)

// LoadRpmRepo loads RPM repository configurations
func LoadRpmRepo(data []byte) error {
	if data == nil {
		data = embedRpmRepo
	}
	var tmpRepos []Repository
	if err := yaml.Unmarshal(data, &tmpRepos); err != nil {
		return fmt.Errorf("failed to parse rpm repo: %v", err)
	}

	// Filter available repos and build maps
	Repos = make([]*Repository, 0)
	RepoMap = make(map[string]*Repository)
	ModuleMap = make(map[string][]string)

	for i := range tmpRepos {
		repo := &tmpRepos[i]
		repo.Distro = config.DistroEL
		repo.Meta = map[string]string{"enabled": "1", "gpgcheck": "0", "module_hotfixes": "1"}
		if repo.Available(config.OSCode, config.OSArch) {
			Repos = append(Repos, repo)
			RepoMap[repo.Name] = repo
			if repo.Module != "" {
				if _, exists := ModuleMap[repo.Module]; !exists {
					ModuleMap[repo.Module] = make([]string, 0)
				}
				ModuleMap[repo.Module] = append(ModuleMap[repo.Module], repo.Name)
			}
		}
	}

	if config.PigstyGPGCheck {
		if repo, ok := RepoMap["pigsty-pgsql"]; ok {
			repo.Meta["gpgcheck"] = "1"
			repo.Meta["gpgkey"] = "file:///etc/pki/rpm-gpg/RPM-GPG-KEY-pigsty"
		}
		if repo, ok := RepoMap["pigsty-infra"]; ok {
			repo.Meta["gpgcheck"] = "1"
			repo.Meta["gpgkey"] = "file:///etc/pki/rpm-gpg/RPM-GPG-KEY-pigsty"
		}
	}

	ModuleMap["pigsty"] = []string{"pigsty-infra", "pigsty-pgsql"}
	ModuleMap["pgdg"] = []string{"pgdg-common", "pgdg-el8fix", "pgdg-el9fix", "pgdg17", "pgdg16", "pgdg15", "pgdg14", "pgdg13"}
	ModuleMap["all"] = append(ModuleMap["pigsty"], append(ModuleMap["pgdg"], ModuleMap["node"]...)...)

	logrus.Debugf("load %d rpm repo, %d modules", len(RepoMap), len(ModuleMap))
	return nil
}

// LoadDebRepo loads DEB repository configurations
func LoadDebRepo(data []byte) error {
	if data == nil {
		data = embedDebRepo
	}
	var tmpRepos []Repository
	if err := yaml.Unmarshal(data, &tmpRepos); err != nil {
		return fmt.Errorf("failed to parse deb repo: %v", err)
	}

	// Filter available repos and build maps
	Repos = make([]*Repository, 0)
	RepoMap = make(map[string]*Repository)
	ModuleMap = make(map[string][]string)

	for i := range tmpRepos {
		repo := &tmpRepos[i]
		repo.Distro = config.DistroDEB
		repo.Meta = map[string]string{"trusted": "yes"}
		if repo.Available(config.OSCode, config.OSArch) {
			Repos = append(Repos, repo)
			RepoMap[repo.Name] = repo
			if repo.Module != "" {
				if _, exists := ModuleMap[repo.Module]; !exists {
					ModuleMap[repo.Module] = make([]string, 0)
				}
				ModuleMap[repo.Module] = append(ModuleMap[repo.Module], repo.Name)
			}
		}
	}

	if config.PigstyGPGCheck {
		if repo, ok := RepoMap["pigsty-pgsql"]; ok {
			delete(repo.Meta, "trusted")
			repo.Meta["signed-by"] = "/etc/apt/keyrings/pigsty.gpg"
		}
		if repo, ok := RepoMap["pigsty-infra"]; ok {
			delete(repo.Meta, "trusted")
			repo.Meta["signed-by"] = "/etc/apt/keyrings/pigsty.gpg"
		}
	}

	ModuleMap["pigsty"] = []string{"pigsty-infra", "pigsty-pgsql"}
	ModuleMap["pgdg"] = []string{"pgdg"}
	ModuleMap["all"] = append(ModuleMap["pigsty"], append(ModuleMap["pgdg"], ModuleMap["node"]...)...)

	logrus.Debugf("load %d deb repo, %d modules", len(RepoMap), len(ModuleMap))
	return nil
}
