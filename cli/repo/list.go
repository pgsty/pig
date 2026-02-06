package repo

import (
	_ "embed"

	"gopkg.in/yaml.v3"
)

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
