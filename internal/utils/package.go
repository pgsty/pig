package utils

import "fmt"

// LocalPackageUpgradeCommand returns a non-destructive package transaction for
// installing, upgrading, downgrading, or reinstalling one local package.
func LocalPackageUpgradeCommand(osType, packagePath string) ([]string, error) {
	switch osType {
	case "rpm":
		return []string{"rpm", "-U", "--replacepkgs", "--oldpackage", packagePath}, nil
	case "deb":
		return []string{"apt-get", "install", "-y", "--reinstall", "--allow-downgrades", packagePath}, nil
	default:
		return nil, fmt.Errorf("unsupported package type: %s", osType)
	}
}
