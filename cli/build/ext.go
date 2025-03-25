package build

import (
	"fmt"
	"os"
	"pig/internal/config"
	"pig/internal/utils"
	"strings"

	"github.com/sirupsen/logrus"
)

func BuildExtension(args []string) error {
	switch config.OSType {
	case config.DistroEL:
		return buildRpmExtension(args)
	case config.DistroDEB:
		return buildDebExtension(args)
	default:
		return fmt.Errorf("unsupported operating system")
	}
}

func buildRpmExtension(extlist []string) error {
	workDir := config.HomeDir + "/rpmbuild/"
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		return fmt.Errorf("rpmbuild directory not found, please run `pig build spec` first")
	}
	os.Chdir(workDir)

	logrus.Infof("building extensions: %s in %s", strings.Join(extlist, ","), workDir)
	for _, ext := range extlist {
		logrus.Infof("################ %s build begin in %s", ext, workDir)
		err := utils.Command([]string{"make", ext})
		if err != nil {
			logrus.Errorf("################  %s build failed: %v", ext, err)
			return err
		} else {
			logrus.Infof("################  %s build success", ext)
		}
	}

	return nil
}

func buildDebExtension(extlist []string) error {
	workDir := config.HomeDir + "/deb/"
	if _, err := os.Stat(workDir); os.IsNotExist(err) {
		return fmt.Errorf("deb directory not found, please run `pig build spec` first")
	}
	os.Chdir(workDir)

	logrus.Infof("building extensions %s in %s", strings.Join(extlist, ","), workDir)

	for _, ext := range extlist {
		err := utils.Command([]string{"make", ext})
		if err != nil {
			logrus.Error("=========================================")
			logrus.Errorf("failed to build extension %s: %v", ext, err)
			return err
		} else {
			logrus.Info("=========================================")
			logrus.Infof("build extension %s success", ext)
		}
	}
	return nil
}
