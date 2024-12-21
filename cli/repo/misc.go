package repo

import (
	"fmt"
	"sort"
	"strings"
)

/********************
* Misc Repo Functions
********************/

// GetReposByModule returns all repositories for a given module
func GetReposByModule(module string) []*Repository {
	result := make([]*Repository, 0)
	if repoNames, ok := ModuleMap[module]; ok {
		for _, name := range repoNames {
			if repo, exists := RepoMap[name]; exists {
				result = append(result, repo)
			}
		}
	}
	return result
}

// GetRepo returns a repository by name
func GetRepo(name string) *Repository {
	return RepoMap[name]
}

func GetModuleList() []string {
	// get modulle keys and sort them
	modules := make([]string, 0, len(ModuleMap))
	for module := range ModuleMap {
		modules = append(modules, module)
	}
	sort.Strings(modules)
	return modules
}

// GetMajorVersionFromCode gets the major version from the code
func GetMajorVersionFromCode(code string) int {
	code = strings.ToLower(code)

	// Handle EL versions
	if strings.HasPrefix(code, "el") {
		var major int
		if _, err := fmt.Sscanf(code, "el%d", &major); err == nil {
			return major
		} else {
			return -1
		}
	}

	if strings.HasPrefix(code, "u") {
		var major int
		if _, err := fmt.Sscanf(code, "u%d", &major); err == nil {
			return major
		} else {
			return -1
		}
	}

	if strings.HasPrefix(code, "d") {
		var major int
		if _, err := fmt.Sscanf(code, "d%d", &major); err == nil {
			return major
		} else {
			return -1
		}
	}

	// Handle Ubuntu codenames
	switch code {
	case "focal":
		return 20
	case "jammy":
		return 22
	case "noble":
		return 24
	}

	// Handle Debian codenames
	switch code {
	case "bullseye":
		return 11
	case "bookworm":
		return 12
	case "trixie":
		return 13
	}

	return -1
}

func containsAny(slice []string, elems ...string) bool {
	for _, elem := range elems {
		for _, s := range slice {
			if s == elem {
				return true
			}
		}
	}
	return false
}
