package repo

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"os"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/utils"
	"sort"
	"strconv"
	"time"
)

// AddModule adds a module to the system
func AddModule(module string, region string) error {
	modulePath := ModuleRepoPath(module)
	moduleContent := ModuleRepoConfig(module, region)

	randomFile := filepath.Join(os.TempDir(), fmt.Sprintf("%s-%s.repo", module, strconv.FormatInt(time.Now().UnixNano(), 36)))
	logrus.Debugf("write module %s to %s, content: %s", module, randomFile, moduleContent)
	if err := os.WriteFile(randomFile, []byte(moduleContent), 0644); err != nil {
		return err
	}
	defer os.Remove(randomFile)
	logrus.Debugf("sudo move %s to %s", randomFile, modulePath)
	return utils.SudoCommand([]string{"mv", "-f", randomFile, modulePath})
}

func GetModuleList() []string {
	modules := make([]string, 0, len(ModuleMap))
	for module := range ModuleMap {
		modules = append(modules, module)
	}
	sort.Strings(modules)
	return modules
}

// ModuleRepoPath returns the path to the repository configuration file for a given module
func ModuleRepoPath(moduleName string) string {
	if config.OSType == config.DistroEL {
		return fmt.Sprintf("/etc/yum.repos.d/%s.repo", moduleName)
	} else if config.OSType == config.DistroDEB {
		return fmt.Sprintf("/etc/apt/sources.list.d/%s.list", moduleName)
	}
	return fmt.Sprintf("/tmp/%s.repo", moduleName)
}

// ModuleRepoConfig generates the repository configuration for a given module
func ModuleRepoConfig(moduleName string, region string) (content string) {
	var repoContent string
	if module, ok := ModuleMap[moduleName]; ok {
		for _, repoName := range module {
			if repo, ok := RepoMap[repoName]; ok {
				if RepoMap[repoName].Available(config.OSCode, config.OSArch) {
					repoContent += repo.Content(region) + "\n"
				}
			}
		}
	}
	return repoContent
}
