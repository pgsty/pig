package cmd

import (
	"fmt"
	"pig/cli/do"
	"strings"

	"github.com/spf13/cobra"
)

var doNodeAddCmd = &cobra.Command{
	Use:         "node-add",
	Short:       "add node to pigsty",
	Annotations: ancsAnn("pig do node-add", "action", "volatile", "unsafe", false, "medium", "recommended", "root", 300000),
	Aliases:     []string{"node", "node-a", "nadd", "na"},
	Long:        `pig do node-add <sel>`,
	Example: `
  pig do node-add pg-test                 # add node by cluster name
  pig do nadd     10.10.10.10             # add node by ip address
  pig do na       10.10.10.10,10.10.10.11 # add multiple nodes
  `,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleDo, "pig do node-add", args, nil, func() error {
			cls := args[0]
			command := []string{"node.yml", "-l", cls}
			return do.RunPlaybook(inventory, command)
		})
	},
}

// doNodeRmCmd - Remove node from pigsty
var doNodeRmCmd = &cobra.Command{
	Use:         "node-rm",
	Short:       "remove node from pigsty",
	Annotations: ancsAnn("pig do node-rm", "action", "volatile", "unsafe", false, "high", "recommended", "root", 300000),
	Aliases:     []string{"node-r", "nrm"},
	Long:        `pig do node-rm <sel>`,
	Example: `
  pig do node-rm pg-test                 # remove node by cluster name
  pig do node-r  10.10.10.10             # remove node by ip address
  pig do nrm     10.10.10.10,10.10.10.11  # remove multiple nodes
  `,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleDo, "pig do node-rm", args, nil, func() error {
			selector := args[0]
			command := []string{"node-rm.yml", "-l", selector}
			return do.RunPlaybook(inventory, command)
		})
	},
}

// doNodeRepoCmd - Update node repository configuration
var doNodeRepoCmd = &cobra.Command{
	Use:         "node-repo",
	Short:       "update node repo",
	Annotations: ancsAnn("pig do node-repo", "action", "volatile", "unsafe", false, "low", "recommended", "root", 60000),
	Aliases:     []string{"node-rp", "nrp"},
	Long:        `pig do node-repo <sel> [module]`,
	Example: `
  pig do node-repo pg-meta               # add default local repo to pg-meta
  pig do node-rp   pg-test node          # add node repo to pg-test
  pig do nrp       pg-meta node,infra    # add default local repo to all nodes

  modules: local,infra,pgsql,node,extra,mysql,mongo,redis,haproxy,grafana,kube,...
  `,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleDo, "pig do node-repo", args, nil, func() error {
			var selector, module string
			if len(args) >= 1 {
				selector = args[0]
			}
			if len(args) >= 2 {
				module = args[1]
			}
			if len(args) >= 3 {
				return cmd.Help()
			}
			command := []string{"node.yml", "-t", "node_repo"}
			if selector != "" {
				command = append(command, "-l", selector)
			}
			if module != "" {
				command = append(command, "-e", fmt.Sprintf("node_repo_modules=%s", module))
			}
			return do.RunPlaybook(inventory, command)
		})
	},
}

// doNodePkgCmd - Install/update node packages
var doNodePkgCmd = &cobra.Command{
	Use:         "node-pkg",
	Short:       "update node package",
	Annotations: ancsAnn("pig do node-pkg", "action", "volatile", "unsafe", false, "low", "recommended", "root", 60000),
	Aliases:     []string{"node-p", "np"},
	Long:        `pig do node-pkg <sel> [module]`,
	Example: `
  pig do node-pkg pg-meta openssh       # upgrade openssh on pg-meta
  pig do node-p   pg-test juicefs       # install juicefs on pg-test
  pig do np       all duckdb restic     # install 2 packages on all nodes

  PS: make sure required repo is available on target nodes
  `,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleDo, "pig do node-pkg", args, nil, func() error {
			selector := args[0]
			command := []string{"node.yml", "-l", selector, "-t", "node_pkg_extra"}
			if len(args) > 1 {
				packages := strings.Join(args[1:], ",")
				packages = fmt.Sprintf(`{"node_packages":["%s"]}`, packages)
				command = append(command, "-e", packages)
			}
			return do.RunPlaybook(inventory, command)
		})
	},
}

// doRepoBuildCmd - Rebuild infrastructure repository
var doRepoBuildCmd = &cobra.Command{
	Use:         "repo-build",
	Short:       "rebuild infra repo",
	Annotations: ancsAnn("pig do repo-build", "action", "volatile", "unsafe", false, "low", "recommended", "root", 60000),
	Aliases:     []string{"repo-b", "rb"},
	Long:        `pig do repo-build`,
	Example: `
  pig do repo-build   # rebuild infra repo
  `,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runLegacyStructured(legacyModuleDo, "pig do repo-build", args, nil, func() error {
			command := []string{"infra.yml", "-l", "infra", "-t", "repo_build"}
			return do.RunPlaybook(inventory, command)
		})
	},
}
