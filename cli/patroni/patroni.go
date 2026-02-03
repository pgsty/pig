package patroni

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"pig/internal/output"
	"pig/internal/utils"

	"github.com/sirupsen/logrus"
)

// DefaultConfigPath is the fixed patroni config file path
const DefaultConfigPath = "/etc/patroni/patroni.yml"

// DefaultDBSU is the default database superuser
const DefaultDBSU = "postgres"

// runPatronictl executes patronictl with given arguments as DBSU
func runPatronictl(dbsu string, args []string) error {
	binPath, err := exec.LookPath("patronictl")
	if err != nil {
		return fmt.Errorf("patronictl not found in PATH, please install patroni first")
	}

	// Build full command: patronictl -c <config> <args...>
	cmdArgs := []string{binPath, "-c", DefaultConfigPath}
	cmdArgs = append(cmdArgs, args...)

	utils.PrintHint(cmdArgs)
	logrus.Debugf("patronictl %s", strings.Join(args, " "))
	return utils.DBSUCommand(dbsu, cmdArgs)
}

// List runs patronictl list with -e -t flags
func List(dbsu string, watch bool, interval float64) error {
	args := []string{"list", "-e", "-t"}
	if watch {
		args = append(args, "-W")
	}
	if interval > 0 {
		args = append(args, "-w", fmt.Sprintf("%g", interval))
	}
	return runPatronictl(dbsu, args)
}

// ConfigEdit opens interactive config editor
func ConfigEdit(dbsu string) error {
	return runPatronictl(dbsu, []string{"edit-config"})
}

// ConfigShow displays cluster configuration
func ConfigShow(dbsu string) error {
	return runPatronictl(dbsu, []string{"show-config"})
}

// ConfigSet sets Patroni configuration with key=value pairs (non-interactive)
func ConfigSet(dbsu string, kvPairs []string) error {
	if len(kvPairs) == 0 {
		return fmt.Errorf("no key=value pairs provided\nUsage: pig pt config set key=value ...")
	}
	args := []string{"edit-config", "--force"}
	for _, kv := range kvPairs {
		args = append(args, "-s", kv)
	}
	return runPatronictl(dbsu, args)
}

// ConfigPG sets PostgreSQL configuration with key=value pairs (non-interactive)
func ConfigPG(dbsu string, kvPairs []string) error {
	if len(kvPairs) == 0 {
		return fmt.Errorf("no key=value pairs provided\nUsage: pig pt config pg key=value ...")
	}
	args := []string{"edit-config", "--force"}
	for _, kv := range kvPairs {
		args = append(args, "-p", kv)
	}
	return runPatronictl(dbsu, args)
}

// Reload reloads PostgreSQL configuration via patronictl reload
func Reload(dbsu string) error {
	return runPatronictl(dbsu, []string{"reload"})
}

// Pause pauses automatic failover for the cluster
func Pause(dbsu string, wait bool) error {
	args := []string{"pause"}
	if wait {
		args = append(args, "--wait")
	}
	return runPatronictl(dbsu, args)
}

// Resume resumes automatic failover for the cluster
func Resume(dbsu string, wait bool) error {
	args := []string{"resume"}
	if wait {
		args = append(args, "--wait")
	}
	return runPatronictl(dbsu, args)
}

// Systemctl runs systemctl command for patroni service
func Systemctl(action string) error {
	logrus.Debugf("systemctl %s patroni", action)
	return utils.RunSystemctl(action, "patroni")
}

// Status shows comprehensive patroni status (systemctl + ps + patronictl list)
func Status(dbsu string) error {
	// 1. systemctl status patroni
	fmt.Println("=== Patroni Service Status ===")
	cmd1 := exec.Command("sudo", "systemctl", "status", "patroni", "--no-pager", "-l")
	cmd1.Stdout = os.Stdout
	cmd1.Stderr = os.Stderr
	if err := cmd1.Run(); err != nil {
		// Intentionally not failing: service might not exist or not be running
		// This is informational output, not a failure condition
		logrus.Debugf("systemctl status patroni: %v (may be expected)", err)
	}

	// 2. ps aux | grep patroni
	fmt.Println("\n=== Patroni Processes ===")
	cmd2 := exec.Command("bash", "-c", "ps aux | grep -E '[p]atroni' || echo 'No patroni processes found'")
	cmd2.Stdout = os.Stdout
	cmd2.Stderr = os.Stderr
	cmd2.Run()

	// 3. patronictl list
	fmt.Println("\n=== Patroni Cluster Status ===")
	binPath, err := exec.LookPath("patronictl")
	if err != nil {
		fmt.Println("patronictl not found, skipping cluster status")
		return nil
	}
	cmdArgs := []string{binPath, "-c", DefaultConfigPath, "list", "-e", "-t"}
	return utils.DBSUCommand(dbsu, cmdArgs)
}

// Log views patroni logs using journalctl
func Log(follow bool, lines string) error {
	args := []string{"-u", "patroni"}
	if follow {
		args = append(args, "-f")
	}
	if lines != "" {
		args = append(args, "-n", lines)
	}

	cmdArgs := append([]string{"journalctl"}, args...)
	utils.PrintHint(cmdArgs)
	logrus.Debugf("journalctl %s", strings.Join(args, " "))
	cmd := exec.Command("journalctl", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// RestartOptions holds options for patronictl restart
type RestartOptions struct {
	Member  string // specific member to restart (empty = all)
	Role    string // filter by role: leader, replica, any
	Force   bool   // skip confirmation
	Pending bool   // only restart members with pending restart
}

// Restart restarts PostgreSQL via patronictl restart
func Restart(dbsu string, opts *RestartOptions) error {
	args := []string{"restart"}

	if opts != nil {
		// Add role filter if specified
		if opts.Role != "" {
			args = append(args, "--role", opts.Role)
		}

		// Add member name if specified (positional argument after cluster)
		// patronictl restart <cluster> [member]
		// Since we use -c config, cluster name is auto-detected
		if opts.Member != "" {
			args = append(args, opts.Member)
		}

		if opts.Force {
			args = append(args, "--force")
		}
		if opts.Pending {
			args = append(args, "--pending")
		}
	}

	return runPatronictl(dbsu, args)
}

// ReinitOptions holds options for patronictl reinit
type ReinitOptions struct {
	Member string // specific member to reinit (required)
	Force  bool   // skip confirmation
	Wait   bool   // wait for reinit to complete
}

// Reinit reinitializes a cluster member via patronictl reinit
func Reinit(dbsu string, opts *ReinitOptions) error {
	if opts == nil || opts.Member == "" {
		return fmt.Errorf("member name is required for reinit")
	}

	args := []string{"reinit"}
	args = append(args, opts.Member)

	if opts.Force {
		args = append(args, "--force")
	}
	if opts.Wait {
		args = append(args, "--wait")
	}

	return runPatronictl(dbsu, args)
}

// SwitchoverOptions holds options for patronictl switchover
type SwitchoverOptions struct {
	Leader    string // current leader (optional, auto-detected)
	Candidate string // target candidate (optional)
	Force     bool   // skip confirmation
	Scheduled string // scheduled time (e.g., "2024-01-01T12:00:00")
}

// BuildSwitchoverPlan builds a structured execution plan for switchover.
// Returns a Plan with default values if opts is nil.
func BuildSwitchoverPlan(opts *SwitchoverOptions) *output.Plan {
	if opts == nil {
		opts = &SwitchoverOptions{}
	}
	actions := []output.Action{
		{Step: 1, Description: "Validate switchover parameters"},
		{Step: 2, Description: "Execute patronictl switchover"},
		{Step: 3, Description: "Verify new leader and update replicas"},
	}

	affects := []output.Resource{
		{Type: "cluster", Name: "patroni", Impact: "role change", Detail: "leader switchover"},
	}

	if opts != nil && opts.Leader != "" {
		affects = append(affects, output.Resource{
			Type:   "node",
			Name:   opts.Leader,
			Impact: "leader demote",
		})
	}
	if opts != nil && opts.Candidate != "" {
		affects = append(affects, output.Resource{
			Type:   "node",
			Name:   opts.Candidate,
			Impact: "leader promote",
		})
	}
	if opts != nil && opts.Scheduled != "" {
		affects = append(affects, output.Resource{
			Type:   "schedule",
			Name:   opts.Scheduled,
			Impact: "deferred",
		})
	}

	expected := "Leadership transferred to target candidate; old leader becomes replica"
	if opts != nil && opts.Candidate != "" {
		expected = fmt.Sprintf("Leadership transferred to %s; old leader becomes replica", opts.Candidate)
	}

	risks := []string{
		"Brief write downtime during leader transition",
		"Clients may need to reconnect after switchover",
	}
	if opts != nil && opts.Force {
		risks = append(risks, "Confirmation is skipped (--force)")
	}

	return &output.Plan{
		Command:  buildSwitchoverCommand(opts),
		Actions:  actions,
		Affects:  affects,
		Expected: expected,
		Risks:    risks,
	}
}

func buildSwitchoverCommand(opts *SwitchoverOptions) string {
	args := []string{"pig", "pt", "switchover"}
	if opts == nil {
		return strings.Join(args, " ")
	}
	if opts.Leader != "" {
		args = append(args, "--leader", opts.Leader)
	}
	if opts.Candidate != "" {
		args = append(args, "--candidate", opts.Candidate)
	}
	if opts.Scheduled != "" {
		args = append(args, "--scheduled", opts.Scheduled)
	}
	if opts.Force {
		args = append(args, "--force")
	}
	return strings.Join(args, " ")
}

// Switchover performs a planned switchover via patronictl switchover
func Switchover(dbsu string, opts *SwitchoverOptions) error {
	args := []string{"switchover"}

	if opts != nil {
		if opts.Leader != "" {
			args = append(args, "--leader", opts.Leader)
		}
		if opts.Candidate != "" {
			args = append(args, "--candidate", opts.Candidate)
		}
		if opts.Force {
			args = append(args, "--force")
		}
		if opts.Scheduled != "" {
			args = append(args, "--scheduled", opts.Scheduled)
		}
	}

	return runPatronictl(dbsu, args)
}

// FailoverOptions holds options for patronictl failover
type FailoverOptions struct {
	Candidate string // target candidate (optional)
	Force     bool   // skip confirmation
}

// Failover performs an unplanned failover via patronictl failover
func Failover(dbsu string, opts *FailoverOptions) error {
	args := []string{"failover"}

	if opts != nil {
		if opts.Candidate != "" {
			args = append(args, "--candidate", opts.Candidate)
		}
		if opts.Force {
			args = append(args, "--force")
		}
	}

	return runPatronictl(dbsu, args)
}
