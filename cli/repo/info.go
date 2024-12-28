package repo

import (
	"fmt"

	"github.com/sirupsen/logrus"
)

func Info(args ...string) error {
	m, err := NewManager()
	if err != nil {
		return err
	}

	if len(args) == 0 {
		return fmt.Errorf("repo or module name is required")
	}

	var repoList []string
	repoDedupe := make(map[string]bool)

	for _, arg := range args {
		if m.Module[arg] != nil {
			// treat it as module name
			for _, repoName := range m.Module[arg] {
				if !repoDedupe[repoName] {
					repoList = append(repoList, repoName)
					repoDedupe[repoName] = true
				}
			}
			logrus.Debugf("expand module %s to %s", arg, compactStrArray(m.Module[arg]))
		} else {
			// treat it as repo name
			if !repoDedupe[arg] {
				repoList = append(repoList, arg)
				repoDedupe[arg] = true
			}
		}
	}

	logrus.Debugf("repo info to be queried: %s", compactStrArray(repoList))
	for _, name := range repoList {
		for _, repo := range m.Data {
			if repo.Name == name {
				fmt.Println("#-------------------------------------------------")
				fmt.Println(repo.Info())
			}
		}
	}
	return nil
}
