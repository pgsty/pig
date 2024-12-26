package repo

import (
	_ "embed"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

func ListRepo() error {
	rm, err := NewRepoManager()
	if err != nil {
		return err
	}
	fmt.Printf("os_environment: {type: %s, major: %d, code: %s, arch: %s}\n", rm.OsType, rm.OsMajorVersion, rm.OsDistroCode, rm.OsArch)
	fmt.Printf("repo_upstream:  # Available Repo: %d\n", len(rm.List))
	for _, r := range rm.List {
		fmt.Println("  " + r.ToInlineYAML())
	}

	// sort module list and print
	modules := rm.ModuleOrder()
	fmt.Printf("repo_modules:   # Available Modules: %d\n", len(modules))
	for _, module := range modules {
		fmt.Printf("  - %s: %s\n", module, strings.Join(rm.Module[module], ", "))
	}
	return nil
}

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

// ListRepoData prints the repository data in a formatted manner
func ListRepoData() error {
	rm, err := NewRepoManager()
	if err != nil {
		return err
	}

	fmt.Printf("repo_rawdata:  # {total: %d, available: %d, source: %s}\n", len(rm.Data), len(rm.List), rm.DataSource)
	for _, r := range rm.Data {
		line := r.ToInlineYAML()
		if r.AvailableInCurrentOS() {
			fmt.Println(strings.Replace(line, "- ", "  o ", 1))
		} else {
			fmt.Println(strings.Replace(line, "- ", "  x ", 1))
		}
	}
	return nil
}
