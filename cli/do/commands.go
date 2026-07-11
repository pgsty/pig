package do

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

// PlaybookBuilder converts command arguments into one or more ansible-playbook
// argument lists. Keeping this mapping in cli/do makes Cobra commands declarative
// and gives every playbook workflow the same validation and execution path.
type PlaybookBuilder func(args []string) ([][]string, error)

// RunPlaybooks runs a sequence of ansible playbook commands.
func RunPlaybooks(inventory string, commands [][]string) error {
	for _, command := range commands {
		if err := RunPlaybook(inventory, command); err != nil {
			return err
		}
	}
	return nil
}

func BuildPgsqlAddCommands(args []string) ([][]string, error) {
	if err := requireArgs(args, 1, "pgsql cluster is required"); err != nil {
		return nil, err
	}
	cluster, ips := args[0], args[1:]
	if len(ips) == 0 {
		return [][]string{{"pgsql.yml", "-l", cluster}}, nil
	}
	target, exists, err := buildPgsqlInstancePatterns(cluster, ips)
	if err != nil {
		return nil, err
	}
	return [][]string{
		{"pgsql.yml", "-l", target},
		{"pgsql.yml", "-l", exists, "-t", "pg_service"},
	}, nil
}

func BuildPgsqlRmCommands(args []string, uninstall bool) ([][]string, error) {
	if err := requireArgs(args, 1, "pgsql cluster is required"); err != nil {
		return nil, err
	}
	cluster, ips := args[0], args[1:]
	selector := cluster
	if len(ips) > 0 {
		target, _, err := buildPgsqlInstancePatterns(cluster, ips)
		if err != nil {
			return nil, err
		}
		selector = target
	}
	command := []string{"pgsql-rm.yml", "-l", selector}
	if uninstall {
		command = append(command, "-e", "pg_rm_pkg=true")
	}
	return [][]string{command}, nil
}

func BuildPgsqlUserCommands(args []string) ([][]string, error) {
	if err := requireArgs(args, 2, "pgsql cluster and username are required"); err != nil {
		return nil, err
	}
	command := []string{"pgsql-user.yml", "-l", args[0], "-e", fmt.Sprintf("username=%s", args[1])}
	return singleCommand(append(command, args[2:]...)), nil
}

func BuildPgsqlDBCommands(args []string) ([][]string, error) {
	if err := requireArgs(args, 2, "pgsql cluster and database name are required"); err != nil {
		return nil, err
	}
	command := []string{"pgsql-db.yml", "-l", args[0], "-e", fmt.Sprintf("dbname=%s", args[1])}
	return singleCommand(append(command, args[2:]...)), nil
}

func BuildPgsqlExtCommands(args []string) ([][]string, error) {
	if err := requireArgs(args, 1, "pgsql selector is required"); err != nil {
		return nil, err
	}
	command := []string{"pgsql.yml", "-l", args[0], "-t", "pg_extension"}
	if len(args) > 1 {
		packages, err := encodeCommaListExtraVar("pg_extensions", args[1:])
		if err != nil {
			return nil, err
		}
		command = append(command, "-e", packages)
	}
	return singleCommand(command), nil
}

func BuildPgsqlHBACommands(args []string) ([][]string, error) {
	if err := requireExactArgs(args, 1, "exactly one pgsql selector is required"); err != nil {
		return nil, err
	}
	return singleCommand([]string{"pgsql.yml", "-l", args[0], "-t", "pg_hba,pg_reload,pgbouncer_hba,pgbouncer_reload"}), nil
}

func BuildPgsqlServiceCommands(args []string) ([][]string, error) {
	if err := requireExactArgs(args, 1, "exactly one pgsql selector is required"); err != nil {
		return nil, err
	}
	return singleCommand([]string{"pgsql.yml", "-l", args[0], "-t", "pg_service"}), nil
}

func BuildPgmonAddCommands(args []string) ([][]string, error) {
	if err := requireExactArgs(args, 1, "exactly one pgsql cluster is required"); err != nil {
		return nil, err
	}
	return singleCommand([]string{"pgsql-monitor.yml", "-e", fmt.Sprintf("clsname=%s", args[0])}), nil
}

func RemovePgmon(inventory string, args []string) error {
	if err := requireExactArgs(args, 1, "exactly one pgsql cluster is required"); err != nil {
		return err
	}
	target := fmt.Sprintf("/etc/prometheus/targets/pgrds/%s.yml", args[0])
	logrus.Infof("removing pgsql monitor target %s", args[0])
	return RunAnsible(inventory, []string{"infra", "-m", "file", "-b", "-a", fmt.Sprintf("path=%s state=absent", target)})
}

func BuildNodeAddCommands(args []string) ([][]string, error) {
	selector, err := joinSelectors(args)
	if err != nil {
		return nil, err
	}
	return singleCommand([]string{"node.yml", "-l", selector}), nil
}

func BuildNodeRmCommands(args []string) ([][]string, error) {
	selector, err := joinSelectors(args)
	if err != nil {
		return nil, err
	}
	return singleCommand([]string{"node-rm.yml", "-l", selector}), nil
}

func BuildNodeRepoCommands(args []string) ([][]string, error) {
	if len(args) > 2 {
		return nil, fmt.Errorf("node-repo accepts at most selector and module")
	}
	command := []string{"node.yml", "-t", "node_repo"}
	if len(args) >= 1 && args[0] != "" {
		command = append(command, "-l", args[0])
	}
	if len(args) == 2 && args[1] != "" {
		command = append(command, "-e", fmt.Sprintf("node_repo_modules=%s", args[1]))
	}
	return singleCommand(command), nil
}

func BuildNodePackageCommands(args []string) ([][]string, error) {
	if err := requireArgs(args, 1, "node selector is required"); err != nil {
		return nil, err
	}
	command := []string{"node.yml", "-l", args[0], "-t", "node_pkg_extra"}
	if len(args) > 1 {
		packages, err := encodeCommaListExtraVar("node_packages", args[1:])
		if err != nil {
			return nil, err
		}
		command = append(command, "-e", packages)
	}
	return singleCommand(command), nil
}

func BuildRepoBuildCommands(args []string) ([][]string, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf("repo-build accepts no arguments")
	}
	return singleCommand([]string{"infra.yml", "-l", "infra", "-t", "repo_build"}), nil
}

func BuildRedisAddCommands(args []string) ([][]string, error) {
	if err := requireArgs(args, 1, "redis selector is required"); err != nil {
		return nil, err
	}
	selector, ports := args[0], args[1:]
	if len(ports) == 0 {
		return [][]string{{"redis.yml", "-l", selector}}, nil
	}
	if !isIPv4(selector) {
		return nil, fmt.Errorf("redis instance operations require an IP selector")
	}
	commands := make([][]string, 0, len(ports))
	for _, port := range ports {
		if err := validateRedisPort(port); err != nil {
			return nil, err
		}
		commands = append(commands, []string{"redis.yml", "-l", selector, "-e", fmt.Sprintf("redis_port=%s", port)})
	}
	return commands, nil
}

func BuildRedisRmCommands(args []string, uninstall bool) ([][]string, error) {
	if err := requireArgs(args, 1, "redis selector is required"); err != nil {
		return nil, err
	}
	selector, ports := args[0], args[1:]
	if len(ports) == 0 {
		command := []string{"redis-rm.yml", "-l", selector}
		if uninstall {
			command = append(command, "-e", "redis_rm_pkg=true")
		}
		return [][]string{command}, nil
	}
	if !isIPv4(selector) {
		return nil, fmt.Errorf("redis instance operations require an IP selector")
	}
	if uninstall {
		return nil, fmt.Errorf("redis package uninstall is only supported for node or cluster removal")
	}
	commands := make([][]string, 0, len(ports))
	for _, port := range ports {
		if err := validateRedisPort(port); err != nil {
			return nil, err
		}
		command := []string{"redis-rm.yml", "-l", selector, "-e", fmt.Sprintf("redis_port=%s", port)}
		commands = append(commands, command)
	}
	return commands, nil
}

func singleCommand(command []string) [][]string {
	return [][]string{command}
}

// encodeCommaListExtraVar preserves the historical single comma-joined list
// value while using a real JSON encoder so package names cannot inject extra
// Ansible variables. HTML escaping is disabled to keep ordinary output stable.
func encodeCommaListExtraVar(key string, values []string) (string, error) {
	var encoded strings.Builder
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(map[string][]string{
		key: []string{strings.Join(values, ",")},
	}); err != nil {
		return "", fmt.Errorf("failed to encode %s extra variable: %w", key, err)
	}
	return strings.TrimSuffix(encoded.String(), "\n"), nil
}

func requireArgs(args []string, count int, message string) error {
	if len(args) < count {
		return errors.New(message)
	}
	for _, arg := range args[:count] {
		if arg == "" {
			return errors.New(message)
		}
	}
	return nil
}

func requireExactArgs(args []string, count int, message string) error {
	if len(args) != count {
		return errors.New(message)
	}
	return requireArgs(args, count, message)
}

func buildPgsqlInstancePatterns(cluster string, ips []string) (string, string, error) {
	target := "&" + cluster
	existing := cluster
	for _, ip := range ips {
		if !isIPv4(ip) {
			return "", "", fmt.Errorf("invalid ip address: %s", ip)
		}
		target = ip + "," + target
		existing += ",!" + ip
	}
	return target, existing, nil
}

func joinSelectors(selectors []string) (string, error) {
	if len(selectors) == 0 {
		return "", fmt.Errorf("selector is required")
	}
	for _, selector := range selectors {
		if selector == "" {
			return "", fmt.Errorf("selector cannot be empty")
		}
	}
	return strings.Join(selectors, ","), nil
}

func validateRedisPort(port string) error {
	n, err := strconv.Atoi(port)
	if err != nil || n < 1024 || n > 65535 {
		return fmt.Errorf("invalid redis port: %s", port)
	}
	return nil
}

func isIPv4(value string) bool {
	ip := net.ParseIP(value)
	return ip != nil && ip.To4() != nil
}
