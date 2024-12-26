package repo

import (
	"fmt"
	"os"
	"path/filepath"
	"pig/internal/config"
	"runtime"
	"strings"

	"github.com/sirupsen/logrus"
)

// RpmPrecheck checks if the system is an EL distro
func RpmPrecheck() error {
	if runtime.GOOS != "linux" { // check if linux
		return fmt.Errorf("pigsty works on linux, unsupported os: %s", runtime.GOOS)
	}
	if config.OSType != config.DistroEL { // check if EL distro
		return fmt.Errorf("can not add rpm repo to %s distro", config.OSType)
	}
	return nil
}

// DebPrecheck checks if the system is a DEB distro
func DebPrecheck() error {
	if runtime.GOOS != "linux" { // check if linux
		return fmt.Errorf("pigsty works on linux, unsupported os: %s", runtime.GOOS)
	}
	if config.OSType != config.DistroDEB { // check if DEB distro
		return fmt.Errorf("can not add deb repo to %s distro", config.OSType)
	}
	return nil
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

	if strings.HasPrefix(code, "a") {
		var major int
		if _, err := fmt.Sscanf(code, "a%d", &major); err == nil {
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

// // AddPigstyDebRepo adds the Pigsty DEB repository to the system
// func AddPigstyDebRepo(region string) error {
// 	if err := DebPrecheck(); err != nil {
// 		return err
// 	}
// 	LoadDebRepo(embedDebRepo)

// 	if region == "" { // check network condition (if region is not set)
// 		get.Timeout = time.Second
// 		get.NetworkCondition()
// 		if !get.InternetAccess {
// 			logrus.Warn("no internet access, assume region = default")
// 			region = "default"
// 		}
// 	}

// 	// write gpg key
// 	err := AddDebGPGKey()
// 	if err != nil {
// 		return err
// 	}
// 	logrus.Infof("import gpg key B9BD8B20 to %s", pigstyDebGPGPath)

// 	// write repo file
// 	repoContent := ModuleRepoConfig("pigsty", region)
// 	err = TryReadMkdirWrite(ModuleRepoPath("pigsty"), []byte(repoContent))
// 	if err != nil {
// 		return err
// 	}
// 	logrus.Infof("repo added: %s", ModuleRepoPath("pigsty"))
// 	return nil
// }

// // RemovePigstyDebRepo removes the Pigsty DEB repository from the system
// func RemovePigstyDebRepo() error {
// 	if err := DebPrecheck(); err != nil {
// 		return err
// 	}

// 	// wipe pigsty repo file
// 	err := WipeFile(ModuleRepoPath("pigsty"))
// 	if err != nil {
// 		return err
// 	}
// 	logrus.Infof("remove %s", ModuleRepoPath("pigsty"))

// 	// wipe pigsty gpg file
// 	err = WipeFile(pigstyDebGPGPath)
// 	if err != nil {
// 		return err
// 	}
// 	logrus.Infof("remove gpg key B9BD8B20 from %s", pigstyDebGPGPath)
// 	return nil
// }

// TryReadMkdirWrite will try to read file and compare content, if same, return nil, otherwise write content to file, and make sure the directory exists
func TryReadMkdirWrite(filePath string, content []byte) error {
	// if it is permission denied, don't try to write
	if _, err := os.Stat(filePath); err == nil {
		if target, err := os.ReadFile(filePath); err == nil {
			if string(target) == string(content) {
				return nil
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return err //fmt.Errorf("failed to create parent directories: %v", err)
	}
	return os.WriteFile(filePath, content, 0644)
}

// WipeFile removes a file, if permission denied, try to remove with sudo
func WipeFile(filePath string) error {
	// if not exists, just return
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		logrus.Debugf("file not exists, do nothing : %s %v", filePath, err)
		return nil
	}
	return os.Remove(filePath)
}
