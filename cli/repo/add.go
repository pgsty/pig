package repo

import (
	"fmt"
	"pig/cli/get"
	"pig/internal/config"
	"slices"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// AddRepo adds the Pigsty repository to the system
func AddRepo(region string, modules ...string) error {
	// if "all" in modules, replace it with node, pgsql
	if slices.Contains(modules, "all") {
		modules = append(modules, "node", "pigsty", "pgdg")
		modules = slices.DeleteFunc(modules, func(module string) bool {
			return module == "all"
		})
	}
	// if "pgsql" in modules, remove "pgdg"
	if slices.Contains(modules, "pgsql") {
		modules = slices.DeleteFunc(modules, func(module string) bool {
			return module == "pgdg"
		})
	}
	modules = slices.Compact(modules)
	slices.Sort(modules)

	// Global Reference
	rm, err := NewRepoManager()
	if err != nil {
		return err
	}

	// check module availability
	for _, module := range modules {
		if _, ok := rm.Module[module]; !ok {
			logrus.Warnf("available modules: %v", strings.Join(GetModuleList(), ", "))
			return fmt.Errorf("module %s not found", module)
		}
	}

	// infer region if not set
	if region == "" {
		get.Timeout = time.Second
		get.NetworkCondition()
		if !get.InternetAccess {
			logrus.Warn("no internet access, assume region = default")
			region = "default"
		} else {
			logrus.Infof("infer region %s from network condition", get.Region)
			region = get.Region
		}
	}

	logrus.Infof("add repo for %s.%s , region = %s", config.OSCode, config.OSArch, region)

	// we don't check gpg key by default (since you may have to sudo to add keys)
	// if config.PigstyGPGCheck && (slices.Contains(modules, "pgsql") || slices.Contains(modules, "infra") || slices.Contains(modules, "pigsty") || slices.Contains(modules, "all")) {
	// 	switch config.OSType {
	// 	case config.DistroDEB:
	// 		err = AddDebGPGKey()
	// 	case config.DistroEL:
	// 		err = AddRpmGPGKey()
	// 	}
	// 	if err != nil {
	// 		return err
	// 	}
	// }

	for _, module := range modules {
		if err := AddModule(module, region); err != nil {
			return err
		}
		logrus.Infof("add repo module: %s", module)
	}
	return nil
}
