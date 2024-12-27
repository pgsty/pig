package repo

import (
	_ "embed"
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
)

// ListRepo prints the repository data in a formatted manner (list available only) (invode by repo list)
func ListRepo() error {
	rm, err := NewRepoManager()
	if err != nil {
		return err
	}
	fmt.Printf("os_environment: {code: %s, arch: %s, type: %s, major: %d}\n", rm.OsDistroCode, rm.OsArch, rm.OsType, rm.OsMajorVersion)
	fmt.Printf("repo_upstream:  # Available Repo: %d\n", len(rm.List))
	for _, r := range rm.List {
		logrus.Debugf("raw: %v", r)
		fmt.Println("  " + r.ToInlineYAML())
	}

	// sort module list and print
	modules := rm.ModuleOrder()
	fmt.Printf("repo_modules:   # Available Modules: %d\n", len(modules))
	for _, module := range modules {
		fmt.Printf("  - %-10s: %s\n", module, strings.Join(rm.Module[module], ", "))
	}
	return nil
}

// ListRepoData prints the repository data in a formatted manner (invode by repo list all)
func ListRepoData() error {
	rm, err := NewRepoManager()
	if err != nil {
		return err
	}

	fmt.Printf("repo_rawdata:  # {total: %d, available: %d, source: %s}\n", len(rm.Data), len(rm.List), rm.DataSource)
	for _, r := range rm.Data {
		line := r.ToInlineYAML()
		if r.AvailableInCurrentOS() {
			logrus.Debugf("raw: %v", r)
			fmt.Println(strings.Replace(line, "- ", "  o ", 1))
		} else {
			logrus.Debugf("raw: %v", r)
			fmt.Println(strings.Replace(line, "- ", "  x ", 1))
		}
	}
	return nil
}

// MarshalRepos marshals repository data into folded YAML format
func MarshalRepos(repos []Repository) ([]byte, error) {
	seqNode := &yaml.Node{
		Kind:  yaml.SequenceNode,
		Style: yaml.FoldedStyle,
	}
	for _, r := range repos {
		mapNode := &yaml.Node{}
		if err := mapNode.Encode(r); err != nil {
			return nil, err
		}
		mapNode.Style = yaml.FlowStyle
		seqNode.Content = append(seqNode.Content, mapNode)
	}
	return yaml.Marshal(seqNode)
}
