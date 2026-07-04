/*
Copyright 2018-2026 Ruohang Feng <rh@vonng.com>

PostgreSQL pg_controldata parsing and rendering helpers.
*/
package postgres

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"pig/internal/output"
	"pig/internal/utils"

	"github.com/sirupsen/logrus"
)

var pgControlDataOutput = readPgControlDataOutput

var pgControlDataStates = []string{
	"starting up",
	"shut down",
	"shut down in recovery",
	"shutting down",
	"in crash recovery",
	"in archive recovery",
	"in production",
}

// PgControlDataRow preserves the original pg_controldata output order for
// human rendering while ControlData maps raw keys to raw values for JSON/YAML.
type PgControlDataRow struct {
	Key   string
	Value string
}

// PgControlData contains parsed pg_controldata evidence.
type PgControlData struct {
	Fields map[string]string
	Rows   []PgControlDataRow
}

// ParsePgControlData splits pg_controldata's "key: value" lines without
// normalizing key names, so structured output can preserve the original source.
func ParsePgControlData(raw string) PgControlData {
	result := PgControlData{
		Fields: make(map[string]string),
	}
	for _, line := range strings.Split(raw, "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" {
			continue
		}
		result.Fields[key] = value
		result.Rows = append(result.Rows, PgControlDataRow{Key: key, Value: value})
	}
	return result
}

// RenderPgControlDataTable renders pg_controldata rows as a simple table.
func RenderPgControlDataTable(control PgControlData) string {
	if len(control.Rows) == 0 {
		return ""
	}
	rows := make([][]string, 0, len(control.Rows))
	for _, row := range control.Rows {
		rows = append(rows, []string{row.Key, row.Value})
	}
	return output.RenderTable([]string{"Key", "Value"}, rows)
}

// RenderPgStatusCompactSummary renders the default human-facing pg status
// summary. It intentionally keeps raw pg_controldata detail out of text mode.
func RenderPgStatusCompactSummary(status *PgStatusResultData, role string, control PgControlData) string {
	return renderPgStatusCompactSummary(status, role, control, false)
}

func RenderPgStatusCompactSummaryColor(status *PgStatusResultData, role string, control PgControlData) string {
	return renderPgStatusCompactSummary(status, role, control, true)
}

func renderPgStatusCompactSummary(status *PgStatusResultData, role string, control PgControlData, color bool) string {
	lines := []string{renderPgStatusCompactHeader(color)}
	lines = append(lines, renderPgStatusCompactBodyLines(status, role, control, color)...)
	return strings.Join(lines, "\n") + "\n"
}

func renderPgStatusCompactHeader(color bool) string {
	header := "[pg_controldata status]"
	if color {
		return utils.ColorBold + header + utils.ColorReset
	}
	return header
}

func renderPgStatusCompactBody(status *PgStatusResultData, role string, control PgControlData, color bool) string {
	lines := renderPgStatusCompactBodyLines(status, role, control, color)
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func renderPgStatusCompactBodyLines(status *PgStatusResultData, role string, control PgControlData, color bool) []string {
	if status == nil {
		status = &PgStatusResultData{}
	}
	if role == "" {
		role = string(RoleUnknown)
	}

	lines := []string{renderPostgresSummaryLine(status, role, color)}

	if line := renderClusterSummaryLine(control, color); line != "" {
		lines = append(lines, line)
	}
	if line := renderCheckpointSummaryLine(control); line != "" {
		lines = append(lines, line)
	}
	if line := renderTransactIDSummaryLine(control); line != "" {
		lines = append(lines, line)
	}

	return lines
}

func renderPostgresSummaryLine(status *PgStatusResultData, role string, color bool) string {
	version := "?"
	if status.Version > 0 {
		version = strconv.Itoa(status.Version)
	}
	state := "DOWN"
	if status.Running {
		state = "UP"
	}
	if color {
		state = colorizePgStatusValue(state, pgRunningStateColor(state))
		role = colorizePgStatusValue(role, pgRoleColor(role))
	}

	line := fmt.Sprintf("PostgreSQL %s  %s %s", version, state, role)
	if status.PID > 0 {
		line += fmt.Sprintf("  pid=%d", status.PID)
	}
	if status.Port > 0 {
		line += fmt.Sprintf(" port=%d", status.Port)
	}
	if status.DataDir != "" {
		line += fmt.Sprintf("  data=%s", status.DataDir)
	}
	return line
}

func renderClusterSummaryLine(control PgControlData, color bool) string {
	systemID := controlField(control, "Database system identifier")
	state := controlField(control, "Database cluster state")
	timeline := controlField(control, "Latest checkpoint's TimeLineID")
	if systemID == "" && state == "" && timeline == "" {
		return ""
	}
	stateValue := controlValueOrUnknown(state)
	if color && state != "" {
		stateValue = colorizePgStatusValue(stateValue, pgControlStateColor(state))
	}
	return renderCompactStatusLine("Cluster",
		fmt.Sprintf("%s  state=\"%s\"  timeline=%s",
			controlValueOrUnknown(systemID), stateValue, controlValueOrUnknown(timeline)))
}

func renderCompactStatusLine(label, body string) string {
	return fmt.Sprintf("%-10s %s", label, body)
}

func renderCheckpointSummaryLine(control PgControlData) string {
	checkpointTime := compactControlTime(controlField(control, "Time of latest checkpoint"))
	redo := controlField(control, "Latest checkpoint's REDO location")
	wal := controlField(control, "Latest checkpoint's REDO WAL file")
	if checkpointTime == "" && redo == "" && wal == "" {
		return ""
	}
	return renderCompactStatusLine("Checkpoint",
		fmt.Sprintf("time=%q  redo=%s  wal=%s",
			controlValueOrUnknown(checkpointTime), controlValueOrUnknown(redo), controlValueOrUnknown(wal)))
}

func renderTransactIDSummaryLine(control PgControlData) string {
	xid := xidSummary(control)
	mxid := multiXactSummary(control)
	if xid == "" && mxid == "" {
		return ""
	}
	if xid == "" {
		return renderCompactStatusLine("TransactID", mxid)
	}
	if mxid == "" {
		return renderCompactStatusLine("TransactID", xid)
	}
	return renderCompactStatusLine("TransactID", xid+"  "+mxid)
}

func pgRunningStateColor(state string) string {
	switch strings.ToUpper(strings.TrimSpace(state)) {
	case "UP":
		return utils.ColorGreen
	case "DOWN":
		return utils.ColorRed
	default:
		return utils.ColorYellow
	}
}

func pgRoleColor(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case string(RolePrimary):
		return utils.ColorDarkBlue
	case string(RoleReplica):
		return utils.ColorOrange
	default:
		return utils.ColorYellow
	}
}

func pgControlStateColor(state string) string {
	switch strings.ToLower(strings.TrimSpace(state)) {
	case "in production":
		return utils.ColorGreen
	case "in archive recovery":
		return utils.ColorOrange
	case "in crash recovery":
		return utils.ColorOrange
	case "starting up":
		return utils.ColorYellow
	case "shutting down":
		return utils.ColorYellow
	case "shut down":
		return utils.ColorRed
	case "shut down in recovery":
		return utils.ColorRed
	default:
		return ""
	}
}

func colorizePgStatusValue(value, color string) string {
	if color == "" {
		return value
	}
	return color + value + utils.ColorReset
}

func xidSummary(control PgControlData) string {
	next := parseControlCounter(controlField(control, "Latest checkpoint's NextXID"))
	oldest := parseControlCounter(controlField(control, "Latest checkpoint's oldestXID"))
	db := controlField(control, "Latest checkpoint's oldestXID's DB")
	active := parseControlCounter(controlField(control, "Latest checkpoint's oldestActiveXID"))
	if next == "" && oldest == "" && db == "" && active == "" {
		return ""
	}
	age := counterAge(next, oldest)
	parts := []string{
		"xid=" + controlValueOrUnknown(age),
		"next=" + controlValueOrUnknown(next),
		"oldest=" + controlValueOrUnknown(oldest),
		"db=" + controlValueOrUnknown(db),
	}
	if active != "" {
		parts = append(parts, "active="+active)
	}
	return strings.Join(parts, " ")
}

func multiXactSummary(control PgControlData) string {
	next := parseControlCounter(controlField(control, "Latest checkpoint's NextMultiXactId"))
	oldest := parseControlCounter(controlField(control, "Latest checkpoint's oldestMultiXid"))
	db := controlField(control, "Latest checkpoint's oldestMulti's DB")
	if next == "" && oldest == "" && db == "" {
		return ""
	}
	age := counterAge(next, oldest)
	return strings.Join([]string{
		"mxid=" + controlValueOrUnknown(age),
		"next=" + controlValueOrUnknown(next),
		"oldest=" + controlValueOrUnknown(oldest),
		"db=" + controlValueOrUnknown(db),
	}, " ")
}

func controlField(control PgControlData, key string) string {
	if control.Fields == nil {
		return ""
	}
	return strings.TrimSpace(control.Fields[key])
}

func parseControlCounter(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if idx := strings.LastIndex(value, ":"); idx >= 0 && idx+1 < len(value) {
		value = value[idx+1:]
	}
	return strings.TrimSpace(value)
}

func counterAge(next, oldest string) string {
	nextInt, nextErr := strconv.ParseInt(next, 10, 64)
	oldestInt, oldestErr := strconv.ParseInt(oldest, 10, 64)
	if nextErr != nil || oldestErr != nil {
		return ""
	}
	return strconv.FormatInt(nextInt-oldestInt, 10)
}

func compactControlTime(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	for _, layout := range []string{
		"Mon Jan _2 15:04:05 2006",
		"Mon Jan 2 15:04:05 2006",
		"2006-01-02 15:04:05",
		time.RFC3339,
	} {
		if t, err := time.ParseInLocation(layout, value, time.Local); err == nil {
			return t.Format("2006-01-02 15:04:05")
		}
	}
	return value
}

func controlValueOrUnknown(value string) string {
	if value == "" {
		return "?"
	}
	return value
}

func collectPgControlData(cfg *Config, dbsu, dataDir string) (PgControlData, error) {
	raw, err := pgControlDataOutput(cfg, dbsu, dataDir)
	if err != nil {
		return PgControlData{}, err
	}
	return ParsePgControlData(raw), nil
}

func readPgControlDataOutput(cfg *Config, dbsu, dataDir string) (string, error) {
	pg, err := GetPgInstall(cfg)
	if err != nil {
		return "", err
	}
	return postgresDBSUCommandOutput(dbsu, []string{pg.PgControldata(), "-D", dataDir})
}

func attachPgControlData(statusData *PgStatusResultData, cfg *Config, dbsu, dataDir string) {
	controlData, err := collectPgControlData(cfg, dbsu, dataDir)
	if err != nil {
		logrus.Debugf("pg_controldata failed: %v", err)
		return
	}
	if len(controlData.Fields) > 0 {
		statusData.ControlData = controlData.Fields
	}
}
