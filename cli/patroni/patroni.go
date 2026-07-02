package patroni

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"pig/internal/output"
	"pig/internal/utils"

	"github.com/sirupsen/logrus"
)

var patroniRunJournalctlOutput = runJournalctlOutput

// DefaultConfigPath is the fixed patroni config file path
const DefaultConfigPath = "/etc/patroni/patroni.yml"

// DefaultDBSU is the default database superuser
const DefaultDBSU = "postgres"

// GetClusterName returns the cluster scope name read from `scope:` in the
// patroni config file. Required for patronictl subcommands that take a
// positional CLUSTER_NAME (restart, reinit, switchover, failover); the
// `-c <config>` flag alone does NOT supply scope to those subcommands.
//
// Falls back to reading via DBSU when the config file isn't world-readable
// (typical Pigsty layout: /pg/conf/<scope>.yml is postgres:postgres 0640).
func GetClusterName(dbsu string) (string, error) {
	content, err := patroniReadFile(DefaultConfigPath)
	if err != nil {
		if dbsu == "" {
			dbsu = DefaultDBSU
		}
		text, dbsuErr := patroniDBSUCommandOutput(dbsu, []string{"cat", DefaultConfigPath})
		if dbsuErr != nil {
			return "", newClusterConfigReadError(
				fmt.Errorf("cannot read %s (direct: %v; as %s: %v)", DefaultConfigPath, err, dbsu, dbsuErr),
			)
		}
		content = []byte(text)
	}

	cluster, err := parseClusterNameFromConfig(string(content))
	if err != nil {
		return "", err
	}
	return cluster, nil
}

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

func buildListArgs(cluster string, watch bool, interval float64) []string {
	args := []string{"list"}
	if cluster != "" {
		args = append(args, cluster)
	}
	args = append(args, "-e", "-t")
	if watch {
		args = append(args, "-W")
	}
	if interval > 0 {
		args = append(args, "-w", fmt.Sprintf("%g", interval))
	}
	return args
}

// List runs patronictl list with -e -t flags.
func List(dbsu string, cluster string, watch bool, interval float64) error {
	return patroniRunPatronictl(dbsu, buildListArgs(cluster, watch, interval))
}

func resolveClusterName(dbsu string, op string) (string, error) {
	cluster, err := patroniGetClusterName(dbsu)
	if err != nil {
		return "", fmt.Errorf("%s: %w", op, err)
	}
	if err := validateResolvedClusterName(cluster); err != nil {
		return "", fmt.Errorf("%s: %w", op, err)
	}
	return cluster, nil
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
		return fmt.Errorf("no key=value pairs provided; usage: pig pt config set key=value")
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
		return fmt.Errorf("no key=value pairs provided; usage: pig pt config pg key=value")
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

// Log views patroni logs using journalctl.
func Log(follow bool, lines int) error {
	if lines <= 0 {
		return fmt.Errorf("lines must be positive")
	}
	args := []string{"-u", "patroni"}
	if follow {
		args = append(args, "-f")
	}
	args = append(args, "-n", strconv.Itoa(lines))

	cmdArgs := append([]string{"journalctl"}, args...)
	utils.PrintHint(cmdArgs)
	logrus.Debugf("journalctl %s", strings.Join(args, " "))
	cmd := exec.Command("journalctl", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// LogJSONL outputs recent patroni journal lines as JSONL.
func LogJSONL(lines int) error {
	if lines <= 0 {
		return fmt.Errorf("lines must be positive")
	}
	args := []string{"-u", "patroni", "-n", strconv.Itoa(lines), "--no-pager", "-o", "cat"}
	logrus.Debugf("journalctl %s", strings.Join(args, " "))
	stdout, stderr, err := patroniRunJournalctlOutput(args)
	if err != nil {
		if errText := strings.TrimSpace(stderr); errText != "" {
			return fmt.Errorf("%w: %s", err, errText)
		}
		return err
	}
	return utils.PrintLogMessagesJSONL("patroni", filterJournalNoEntries(stdout))
}

func runJournalctlOutput(args []string) (string, string, error) {
	cmd := exec.Command("journalctl", args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func filterJournalNoEntries(text string) string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) == "-- No entries --" {
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// RestartOptions holds options for patronictl restart
type RestartOptions struct {
	Member  string // specific member to restart (empty = all)
	Role    string // filter by role: leader, replica, any
	Force   bool   // pass --force to patronictl (pig owns confirmation, B04)
	Pending bool   // only restart members with pending restart
}

// buildRestartArgs builds the positional + flag args for `patronictl restart`.
// patronictl restart requires CLUSTER_NAME as the first positional argument
// (the `-c <config>` flag does NOT supply scope here, unlike pause/resume/list).
func buildRestartArgs(cluster string, opts *RestartOptions) []string {
	args := []string{"restart", cluster}

	if opts == nil {
		return args
	}
	if opts.Role != "" {
		args = append(args, "--role", opts.Role)
	}
	if opts.Member != "" {
		args = append(args, opts.Member)
	}
	if opts.Force {
		args = append(args, "--force")
	}
	if opts.Pending {
		args = append(args, "--pending")
	}
	return args
}

// Restart restarts PostgreSQL via patronictl restart
func Restart(dbsu string, opts *RestartOptions) error {
	cluster, err := resolveClusterName(dbsu, "restart")
	if err != nil {
		return err
	}
	return patroniRunPatronictl(dbsu, buildRestartArgs(cluster, opts))
}

// ReinitOptions holds options for patronictl reinit
type ReinitOptions struct {
	Member string // specific member to reinit (required)
	Force  bool   // pass --force to patronictl (pig owns confirmation, B04)
	Wait   bool   // wait for reinit to complete
}

// BuildReinitPlan builds a structured execution plan for reinitializing a Patroni member.
func BuildReinitPlan(opts *ReinitOptions) *output.Plan {
	if opts == nil {
		opts = &ReinitOptions{}
	}
	nextOpts := *opts
	nextOpts.Force = true
	member := opts.Member
	if member == "" {
		member = "required-member"
	}
	return &output.Plan{
		Command:      buildReinitCommand(opts),
		Boundary:     "pt:patroni-cluster",
		Confirmation: "required",
		Actions: []output.Action{
			{Step: 1, Description: "Validate target Patroni member"},
			{Step: 2, Description: "Execute patronictl reinit for the target member"},
			{Step: 3, Description: "Wait for reinitialization if requested, then verify cluster state"},
		},
		Affects: []output.Resource{
			{Type: "node", Name: member, Impact: "rebuild", Detail: "target member data is discarded and resynced"},
			{Type: "cluster", Name: "patroni", Impact: "replica rejoin", Detail: "operation is coordinated through Patroni"},
		},
		Expected: "Target Patroni member is rebuilt from the leader and rejoins the cluster",
		Risks: []string{
			"Target member data is removed before rebuild.",
			"Replica capacity is reduced while reinitialization is running.",
			"This primitive does not choose backup sources or pgBackRest restore targets.",
		},
		Preconditions: []output.Check{
			{Name: "member", Status: "required", Detail: member},
			{Name: "patronictl", Status: "required", Detail: "available on PATH"},
			{Name: "patroni config", Status: "required", Detail: DefaultConfigPath},
		},
		Verifications: []output.Check{
			{Name: "cluster list", Status: "manual", Detail: "pig pt list"},
			{Name: "member status", Status: "manual", Detail: "target member should be running as replica"},
		},
		NextActions: []output.NextAction{
			{Command: buildReinitExecuteCommand(&nextOpts), Reason: "execute member rebuild after explicit confirmation", Required: true},
			{Command: "pig pt list", Reason: "verify member state after reinit", Required: false},
		},
	}
}

func buildReinitExecuteCommand(opts *ReinitOptions) string {
	args := []string{"pig", "pt", "reinit"}
	if opts != nil && opts.Member != "" {
		args = append(args, opts.Member)
	}
	if opts != nil && opts.Force {
		args = append(args, "--yes")
	}
	if opts != nil && opts.Wait {
		args = append(args, "--wait")
	}
	return strings.Join(args, " ")
}

func buildReinitCommand(opts *ReinitOptions) string {
	args := []string{buildReinitExecuteCommand(opts)}
	args = append(args, "--plan")
	return strings.Join(args, " ")
}

// buildReinitArgs builds the positional + flag args for `patronictl reinit`.
// Required positional layout: reinit CLUSTER_NAME MEMBER_NAME.
func buildReinitArgs(cluster string, opts *ReinitOptions) []string {
	args := []string{"reinit", cluster}
	if opts == nil {
		return args
	}
	if opts.Member != "" {
		args = append(args, opts.Member)
	}
	if opts.Force {
		args = append(args, "--force")
	}
	if opts.Wait {
		args = append(args, "--wait")
	}
	return args
}

// Reinit reinitializes a cluster member via patronictl reinit
func Reinit(dbsu string, opts *ReinitOptions) error {
	if opts == nil || opts.Member == "" {
		return fmt.Errorf("member name is required for reinit")
	}
	cluster, err := resolveClusterName(dbsu, "reinit")
	if err != nil {
		return err
	}
	return patroniRunPatronictl(dbsu, buildReinitArgs(cluster, opts))
}

// SwitchoverOptions holds options for patronictl switchover
type SwitchoverOptions struct {
	Leader    string // current leader (optional, auto-detected)
	Candidate string // target candidate (optional)
	Force     bool   // pass --force to patronictl (pig owns confirmation, B04)
	Scheduled string // scheduled time (e.g., "2024-01-01T12:00:00")
}

// BuildSwitchoverPlan builds a structured execution plan for switchover.
// Returns a Plan with default values if opts is nil.
func BuildSwitchoverPlan(opts *SwitchoverOptions) *output.Plan {
	if opts == nil {
		opts = &SwitchoverOptions{}
	}
	nextOpts := *opts
	nextOpts.Force = true
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
		risks = append(risks, "Confirmation is skipped (--yes)")
	}

	return &output.Plan{
		Command:      buildSwitchoverCommand(opts),
		Boundary:     "pt:patroni-cluster",
		Confirmation: "required",
		Actions:      actions,
		Affects:      affects,
		Expected:     expected,
		Risks:        risks,
		Preconditions: []output.Check{
			{Name: "patronictl", Status: "required", Detail: "available on PATH"},
			{Name: "patroni config", Status: "required", Detail: DefaultConfigPath},
			{Name: "leader", Status: "planned", Detail: valueOrAuto(opts.Leader)},
			{Name: "candidate", Status: "planned", Detail: valueOrAuto(opts.Candidate)},
		},
		Verifications: []output.Check{
			{Name: "cluster list", Status: "manual", Detail: "pig pt list"},
			{Name: "local role", Status: "manual", Detail: "pig pg role on affected members"},
		},
		NextActions: []output.NextAction{
			{Command: buildSwitchoverCommand(&nextOpts), Reason: "execute planned Patroni switchover after explicit confirmation", Required: true},
			{Command: "pig pt list", Reason: "verify Patroni cluster roles after switchover", Required: false},
		},
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
		args = append(args, "--yes")
	}
	return strings.Join(args, " ")
}

// buildSwitchoverArgs builds the args for `patronictl switchover`.
// Required positional layout: switchover CLUSTER_NAME [--leader X --candidate Y ...].
func buildSwitchoverArgs(cluster string, opts *SwitchoverOptions) []string {
	args := []string{"switchover", cluster}
	if opts == nil {
		return args
	}
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
	return args
}

// Switchover performs a planned switchover via patronictl switchover
func Switchover(dbsu string, opts *SwitchoverOptions) error {
	cluster, err := resolveClusterName(dbsu, "switchover")
	if err != nil {
		return err
	}
	return patroniRunPatronictl(dbsu, buildSwitchoverArgs(cluster, opts))
}

// FailoverOptions holds options for patronictl failover
type FailoverOptions struct {
	Candidate string // target candidate (optional)
	Force     bool   // pass --force to patronictl (pig owns confirmation, B04)
}

// BuildFailoverPlan builds a structured execution plan for failover.
// Returns a Plan with default values if opts is nil.
func BuildFailoverPlan(opts *FailoverOptions) *output.Plan {
	if opts == nil {
		opts = &FailoverOptions{}
	}
	nextOpts := *opts
	nextOpts.Force = true
	actions := []output.Action{
		{Step: 1, Description: "Validate failover parameters and candidate availability"},
		{Step: 2, Description: "Execute patronictl failover to promote candidate"},
		{Step: 3, Description: "Verify new leader is operational and replicas reconnect"},
	}

	affects := []output.Resource{
		{Type: "cluster", Name: "patroni", Impact: "emergency leader change", Detail: "failover to new leader"},
	}
	if opts.Candidate != "" {
		affects = append(affects, output.Resource{
			Type:   "node",
			Name:   opts.Candidate,
			Impact: "leader promote",
		})
	}

	expected := "New leader elected; remaining members become replicas"
	if opts.Candidate != "" {
		expected = fmt.Sprintf("Leadership transferred to %s; remaining members become replicas", opts.Candidate)
	}

	risks := []string{
		"DATA LOSS POSSIBLE: Unreplicated transactions may be lost",
		"Current leader may have committed transactions not yet replicated",
		"Clients will experience downtime during failover",
		"All connections will be reset after failover",
	}
	if opts.Force {
		risks = append(risks, "Confirmation is skipped (--yes)")
	}

	return &output.Plan{
		Command:      buildFailoverCommand(opts),
		Boundary:     "pt:patroni-cluster",
		Confirmation: "required",
		Actions:      actions,
		Affects:      affects,
		Expected:     expected,
		Risks:        risks,
		Preconditions: []output.Check{
			{Name: "patronictl", Status: "required", Detail: "available on PATH"},
			{Name: "patroni config", Status: "required", Detail: DefaultConfigPath},
			{Name: "candidate", Status: "planned", Detail: valueOrAuto(opts.Candidate)},
			{Name: "leader health", Status: "operator-check", Detail: "only use failover when the current leader is unavailable or unsafe"},
		},
		Verifications: []output.Check{
			{Name: "cluster list", Status: "manual", Detail: "pig pt list"},
			{Name: "application writes", Status: "manual", Detail: "verify clients reconnect to the new leader"},
		},
		NextActions: []output.NextAction{
			{Command: buildFailoverCommand(&nextOpts), Reason: "execute emergency Patroni failover after explicit confirmation", Required: true},
			{Command: "pig pt list", Reason: "verify Patroni cluster roles after failover", Required: false},
		},
	}
}

func buildFailoverCommand(opts *FailoverOptions) string {
	args := []string{"pig", "pt", "failover"}
	if opts == nil {
		return strings.Join(args, " ")
	}
	if opts.Candidate != "" {
		args = append(args, "--candidate", opts.Candidate)
	}
	if opts.Force {
		args = append(args, "--yes")
	}
	return strings.Join(args, " ")
}

func valueOrAuto(value string) string {
	if value == "" {
		return "auto"
	}
	return value
}

// buildFailoverArgs builds the args for `patronictl failover`.
// Required positional layout: failover CLUSTER_NAME [--candidate Y ...].
func buildFailoverArgs(cluster string, opts *FailoverOptions) []string {
	args := []string{"failover", cluster}
	if opts == nil {
		return args
	}
	if opts.Candidate != "" {
		args = append(args, "--candidate", opts.Candidate)
	}
	if opts.Force {
		args = append(args, "--force")
	}
	return args
}

// Failover performs an unplanned failover via patronictl failover
func Failover(dbsu string, opts *FailoverOptions) error {
	cluster, err := resolveClusterName(dbsu, "failover")
	if err != nil {
		return err
	}
	return patroniRunPatronictl(dbsu, buildFailoverArgs(cluster, opts))
}
