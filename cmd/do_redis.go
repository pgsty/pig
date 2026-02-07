package cmd

import (
	"fmt"
	"pig/cli/do"

	"github.com/spf13/cobra"
)

var doRedisAddCmd = &cobra.Command{
	Use:   "redis-add",
	Short: "add redis to pigsty",
	Annotations: map[string]string{
		"name":       "pig do redis-add",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "unsafe",
		"idempotent": "false",
		"risk":       "medium",
		"confirm":    "recommended",
		"os_user":    "root",
		"cost":       "300000",
	},
	Aliases: []string{"redis", "re-add", "ra"},
	Long:    `pig do redis-add <sel> [port...]`,
	Example: `
  pig do redis-add redis-meta                 # init redis cluster
  pig do re-add    redis-test                 # init redis cluster redis-test
  pig do ra        10.10.10.10                # init redis on given node
  pig do ra        10.10.10.11 6379 6380      # init specific redis instances
  `,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDoLegacy("pig do redis-add", args, nil, func() error {
			selector := args[0]
			if len(args) == 1 {
				command := []string{"redis.yml", "-l", selector}
				return do.RunPlaybook(inventory, command)
			}
			for _, port := range args[1:] {
				command := []string{"redis.yml", "-l", selector, "-e", fmt.Sprintf("redis_port=%s", port)}
				if err := do.RunPlaybook(inventory, command); err != nil {
					return err
				}
			}
			return nil
		})
	},
}

// doRedisRmCmd - Remove redis from pigsty
var doRedisRmCmd = &cobra.Command{
	Use:   "redis-rm",
	Short: "remove redis from pigsty",
	Annotations: map[string]string{
		"name":       "pig do redis-rm",
		"type":       "action",
		"volatility": "volatile",
		"parallel":   "unsafe",
		"idempotent": "false",
		"risk":       "high",
		"confirm":    "recommended",
		"os_user":    "root",
		"cost":       "300000",
	},
	Aliases: []string{"re-rm", "rr"},
	Long:    `pig do redis-rm <sel> [port...]`,
	Example: `
  pig do redis-rm redis-meta                 # remove redis cluster
  pig do re-rm    redis-test                 # remove redis cluster redis-test
  pig do rr       10.10.10.10                # remove redis on given node
  pig do rr       10.10.10.11 6379           # remove one specific redis instance
  pig do rr       10.10.10.11 6379 6380      # remove two specific redis instances
  `,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runDoLegacy("pig do redis-rm", args, map[string]interface{}{
			"uninstall": doRemoveWithUninstall,
		}, func() error {
			selector := args[0]
			if len(args) == 1 {
				command := []string{"redis-rm.yml", "-l", selector}
				return do.RunPlaybook(inventory, command)
			}
			for _, port := range args[1:] {
				command := []string{"redis-rm.yml", "-l", selector, "-e", fmt.Sprintf("redis_port=%s", port)}
				if err := do.RunPlaybook(inventory, command); err != nil {
					return err
				}
			}
			return nil
		})
	},
}
