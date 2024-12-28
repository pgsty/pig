package repo

import (
	"fmt"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/utils"
	"slices"
	"sort"
	"strings"

	"github.com/sirupsen/logrus"
)

// AddModules adds multiple modules to the system
func (m *Manager) AddModules(modules ...string) error {
	modules = m.normalizeModules(modules...)

	// check module availability
	for _, module := range modules {
		if _, ok := m.Module[module]; !ok {
			logrus.Warnf("available modules: %v", strings.Join(m.GetModuleList(), ", "))
			return fmt.Errorf("module %s not found", module)
		}
	}

	logrus.Infof("add repo for %s.%s , region = %s", config.OSCode, config.OSArch, m.Region)
	for _, module := range modules {
		if err := m.AddModule(module); err != nil {
			logrus.Errorf("failed to add repo module: %s", module)
			return err
		}
		logrus.Infof("add repo module: %s", module)
	}
	return nil
}

// AddModule handles adding a single module to the system
func (m *Manager) AddModule(module string) error {
	modulePath := m.getModulePath(module)
	if modulePath == "" {
		return fmt.Errorf("fail to get module path for %s", module)
	}
	moduleContent := m.getModuleContent(module)
	return utils.PutFile(modulePath, []byte(moduleContent))
}

// getModulePath returns the path to the repository configuration file for a given module
func (m *Manager) getModulePath(module string) string {
	switch config.OSType {
	case config.DistroEL:
		return filepath.Join(m.RepoDir, fmt.Sprintf("%s.repo", module))
	case config.DistroDEB:
		return filepath.Join(m.RepoDir, fmt.Sprintf("%s.list", module))
	default:
		return ""
	}
}

// getModuleContent returns the multiple repo content together
func (m *Manager) getModuleContent(module string) string {
	var moduleContent string
	if module, ok := m.Module[module]; ok {
		for _, repoName := range module {
			if repo, ok := m.Map[repoName]; ok {
				if repo.Available(config.OSCode, config.OSArch) {
					logrus.Debugf("repo %s is available for %s.%s: %v", repoName, config.OSCode, config.OSArch, repo)
					moduleContent += repo.Content(m.Region) + "\n"
				} else {
					logrus.Debugf("repo %s is not available for %s.%s: %v", repoName, config.OSCode, config.OSArch, repo)
				}
			}
		}
	}
	return moduleContent
}

// normalizeModules normalizes the module list, deduplicates and sorts
func (m *Manager) normalizeModules(modules ...string) []string {
	// if "all" in modules, replace it with node, pgsql
	if slices.Contains(modules, "all") {
		modules = append(modules, "node", "pigsty", "pgdg")
		modules = slices.DeleteFunc(modules, func(module string) bool {
			return module == "all"
		})
	}
	// if "pgsql" in modules, remove "pgdg", since pgdg is a subset of pgsql
	if slices.Contains(modules, "pgsql") {
		modules = slices.DeleteFunc(modules, func(module string) bool {
			return module == "pgdg"
		})
	}
	modules = slices.Compact(modules)
	slices.Sort(modules)
	return modules
}

// GetModuleList returns a sorted list of available modules
func (m *Manager) GetModuleList() []string {
	modules := make([]string, 0, len(m.Module))
	for module := range m.Module {
		modules = append(modules, module)
	}
	sort.Strings(modules)
	return modules
}

// ExpandModuleArgs will split the input arguments by comma if necessary
func ExpandModuleArgs(args []string) []string {
	var newArgs []string
	for _, arg := range args {
		if strings.Contains(arg, ",") {
			newArgs = append(newArgs, strings.Split(arg, ",")...)
		} else {
			newArgs = append(newArgs, arg)
		}
	}
	return newArgs
}
