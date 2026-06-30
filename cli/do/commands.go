package do

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// RunPlaybooks runs a sequence of ansible playbook commands.
func RunPlaybooks(inventory string, commands [][]string) error {
	for _, command := range commands {
		if err := RunPlaybook(inventory, command); err != nil {
			return err
		}
	}
	return nil
}

func BuildPgsqlAddCommands(cluster string, ips []string) ([][]string, error) {
	if cluster == "" {
		return nil, fmt.Errorf("pgsql cluster is required")
	}
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

func BuildPgsqlRmCommands(cluster string, ips []string, uninstall bool) ([][]string, error) {
	if cluster == "" {
		return nil, fmt.Errorf("pgsql cluster is required")
	}
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

func BuildNodeAddCommand(selectors []string) ([]string, error) {
	selector, err := joinSelectors(selectors)
	if err != nil {
		return nil, err
	}
	return []string{"node.yml", "-l", selector}, nil
}

func BuildNodeRmCommand(selectors []string) ([]string, error) {
	selector, err := joinSelectors(selectors)
	if err != nil {
		return nil, err
	}
	return []string{"node-rm.yml", "-l", selector}, nil
}

func BuildNodeRepoCommand(args []string) ([]string, error) {
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
	return command, nil
}

func BuildRedisAddCommands(selector string, ports []string) ([][]string, error) {
	if selector == "" {
		return nil, fmt.Errorf("redis selector is required")
	}
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

func BuildRedisRmCommands(selector string, ports []string, uninstall bool) ([][]string, error) {
	if selector == "" {
		return nil, fmt.Errorf("redis selector is required")
	}
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
