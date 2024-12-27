package repo

import (
	"fmt"
	"os"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/utils"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// AddModules adds multiple modules to the system
func (rm *RepoManager) AddModules(modules ...string) error {
	modules = rm.normalizeModules(modules...)

	// check module availability
	for _, module := range modules {
		if _, ok := rm.Module[module]; !ok {
			logrus.Warnf("available modules: %v", strings.Join(GetModuleList(), ", "))
			return fmt.Errorf("module %s not found", module)
		}
	}

	logrus.Infof("add repo for %s.%s , region = %s", config.OSCode, config.OSArch, rm.Region)
	for _, module := range modules {
		if err := rm.AddModule(module); err != nil {
			logrus.Errorf("failed to add repo module: %s", module)
			return err
		}
		logrus.Infof("add repo module: %s", module)
	}
	return nil
}

// addModule adds a module to the system (require sudo/root privilege to move)
func (rm *RepoManager) AddModule(module string) error {
	modulePath := rm.getModulePath(module)
	moduleContent := rm.getModuleContent(module)
	randomFile := filepath.Join(os.TempDir(), fmt.Sprintf("%s-%s.repo", module, strconv.FormatInt(time.Now().UnixNano(), 36)))

	logrus.Debugf("write module %s to %s, content: %s", module, randomFile, moduleContent)
	if err := os.WriteFile(randomFile, []byte(moduleContent), 0644); err != nil {
		return err
	}
	defer os.Remove(randomFile)
	logrus.Debugf("sudo move %s to %s", randomFile, modulePath)
	return utils.SudoCommand([]string{"mv", "-f", randomFile, modulePath})
}

// getModulePath returns the path to the repository configuration file for a given module
func (rm *RepoManager) getModulePath(module string) string {
	return filepath.Join(rm.RepoDir, fmt.Sprintf("%s.repo", module))
}

// getModuleContent returns the multiple repo content together
func (rm *RepoManager) getModuleContent(module string) string {
	var moduleContent string
	if module, ok := rm.Module[module]; ok {
		for _, repoName := range module {
			if repo, ok := rm.Map[repoName]; ok {
				if repo.Available(config.OSCode, config.OSArch) {
					logrus.Debugf("repo %s is available for %s.%s: %v", repoName, config.OSCode, config.OSArch, repo)
					// fmt.Println(repo)
					fmt.Println(repo.Meta)
					fmt.Println(repo.Content())
					moduleContent += repo.Content(rm.Region) + "\n"
				} else {
					logrus.Debugf("repo %s is not available for %s.%s: %v", repoName, config.OSCode, config.OSArch, repo)
				}
			}
		}
	}
	return moduleContent
}

// normalizeModules normalizes the module list, deduplicates and sorts
func (rm *RepoManager) normalizeModules(modules ...string) []string {
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

func GetModuleList() []string {
	modules := make([]string, 0, len(ModuleMap))
	for module := range ModuleMap {
		modules = append(modules, module)
	}
	sort.Strings(modules)
	return modules
}
