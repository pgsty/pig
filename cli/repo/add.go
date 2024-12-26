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
					moduleContent += repo.Content(rm.Region) + "\n"
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

// // AddModule adds a module to the system
// func AddModule(module string, region string) error {
// 	modulePath := ModuleRepoPath(module)
// 	moduleContent := ModuleRepoConfig(module, region)

// 	randomFile := filepath.Join(os.TempDir(), fmt.Sprintf("%s-%s.repo", module, strconv.FormatInt(time.Now().UnixNano(), 36)))
// 	logrus.Debugf("write module %s to %s, content: %s", module, randomFile, moduleContent)
// 	if err := os.WriteFile(randomFile, []byte(moduleContent), 0644); err != nil {
// 		return err
// 	}
// 	defer os.Remove(randomFile)
// 	logrus.Debugf("sudo move %s to %s", randomFile, modulePath)
// 	return utils.SudoCommand([]string{"mv", "-f", randomFile, modulePath})
// }

// // AddRepo adds the Pigsty repository to the system
// func AddRepo(region string, modules ...string) error {
// 	// if "all" in modules, replace it with node, pgsql
// 	if slices.Contains(modules, "all") {
// 		modules = append(modules, "node", "pigsty", "pgdg")
// 		modules = slices.DeleteFunc(modules, func(module string) bool {
// 			return module == "all"
// 		})
// 	}
// 	// if "pgsql" in modules, remove "pgdg"
// 	if slices.Contains(modules, "pgsql") {
// 		modules = slices.DeleteFunc(modules, func(module string) bool {
// 			return module == "pgdg"
// 		})
// 	}
// 	modules = slices.Compact(modules)
// 	slices.Sort(modules)

// 	// Global Reference
// 	rm, err := NewRepoManager()
// 	if err != nil {
// 		return err
// 	}

// 	// check module availability
// 	for _, module := range modules {
// 		if _, ok := rm.Module[module]; !ok {
// 			logrus.Warnf("available modules: %v", strings.Join(GetModuleList(), ", "))
// 			return fmt.Errorf("module %s not found", module)
// 		}
// 	}

// 	// infer region if not set
// 	if region == "" {
// 		get.Timeout = time.Second
// 		get.NetworkCondition()
// 		if !get.InternetAccess {
// 			logrus.Warn("no internet access, assume region = default")
// 			region = "default"
// 		} else {
// 			logrus.Infof("infer region %s from network condition", get.Region)
// 			region = get.Region
// 		}
// 	}

// 	logrus.Infof("add repo for %s.%s , region = %s", config.OSCode, config.OSArch, region)

// 	// we don't check gpg key by default (since you may have to sudo to add keys)
// 	// if config.PigstyGPGCheck && (slices.Contains(modules, "pgsql") || slices.Contains(modules, "infra") || slices.Contains(modules, "pigsty") || slices.Contains(modules, "all")) {
// 	// 	switch config.OSType {
// 	// 	case config.DistroDEB:
// 	// 		err = AddDebGPGKey()
// 	// 	case config.DistroEL:
// 	// 		err = AddRpmGPGKey()
// 	// 	}
// 	// 	if err != nil {
// 	// 		return err
// 	// 	}
// 	// }

// 	for _, module := range modules {
// 		if err := AddModule(module, region); err != nil {
// 			return err
// 		}
// 		logrus.Infof("add repo module: %s", module)
// 	}
// 	return nil
// }

func GetModuleList() []string {
	modules := make([]string, 0, len(ModuleMap))
	for module := range ModuleMap {
		modules = append(modules, module)
	}
	sort.Strings(modules)
	return modules
}
