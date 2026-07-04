package pitr

import (
	"time"

	"pig/cli/pgbackrest"
	"pig/internal/output"
)

// PITRResultData represents structured output for a PITR execution.
type PITRResultData struct {
	Target            string            `json:"target" yaml:"target"`
	TargetType        string            `json:"target_type" yaml:"target_type"`
	TargetValue       string            `json:"target_value,omitempty" yaml:"target_value,omitempty"`
	DataDir           string            `json:"data_dir" yaml:"data_dir"`
	RequestedDataDir  string            `json:"requested_data_dir" yaml:"requested_data_dir"`
	EffectiveDataDir  string            `json:"effective_data_dir" yaml:"effective_data_dir"`
	ManagedDataDir    string            `json:"managed_data_dir" yaml:"managed_data_dir"`
	SideRestore       bool              `json:"side_restore" yaml:"side_restore"`
	BackupSet         string            `json:"backup_set" yaml:"backup_set"`
	PatroniStopped    bool              `json:"patroni_stopped" yaml:"patroni_stopped"`
	PostgresRestarted bool              `json:"postgres_restarted" yaml:"postgres_restarted"`
	Exclusive         bool              `json:"exclusive" yaml:"exclusive"`
	TargetAction      string            `json:"target_action,omitempty" yaml:"target_action,omitempty"`
	TargetTimeline    string            `json:"target_timeline,omitempty" yaml:"target_timeline,omitempty"`
	PostRestore       *PostRestoreState `json:"post_restore,omitempty" yaml:"post_restore,omitempty"`
	StartedAt         string            `json:"started_at" yaml:"started_at"`
	CompletedAt       string            `json:"completed_at" yaml:"completed_at"`
	DurationSeconds   float64           `json:"duration_seconds" yaml:"duration_seconds"`
}

type PostRestoreState struct {
	Queried            bool   `json:"queried" yaml:"queried"`
	SQLQueried         bool   `json:"sql_queried" yaml:"sql_queried"`
	QuerySkippedReason string `json:"query_skipped_reason,omitempty" yaml:"query_skipped_reason,omitempty"`
	Running            bool   `json:"running" yaml:"running"`
	InRecovery         *bool  `json:"in_recovery,omitempty" yaml:"in_recovery,omitempty"`
	CurrentLSN         string `json:"current_lsn,omitempty" yaml:"current_lsn,omitempty"`
	TimelineID         string `json:"timeline_id,omitempty" yaml:"timeline_id,omitempty"`
	PatroniActive      bool   `json:"patroni_active" yaml:"patroni_active"`
	Error              string `json:"error,omitempty" yaml:"error,omitempty"`
}

func newPITRResultData(state *SystemState, opts *Options, patroniStopped bool, postgresStarted bool, start time.Time, end time.Time, postRestore *PostRestoreState) PITRResultData {
	backupSet := "latest"
	if opts != nil && opts.Set != "" {
		backupSet = opts.Set
	}

	target := "unknown"
	if opts != nil {
		target = getTargetDescription(opts)
	}

	dataDir := ""
	managedDataDir := ""
	sideRestore := false
	if state != nil {
		dataDir = state.DataDir
		managedDataDir = state.ManagedDataDir
		sideRestore = state.SideRestore
	}
	requestedDataDir := ""
	if opts != nil {
		requestedDataDir = opts.DataDir
	}
	targetType, targetValue := targetTypeValueFromOptions(opts)

	duration := end.Sub(start).Seconds()
	if duration < 0 {
		duration = 0
	}

	return PITRResultData{
		Target:            target,
		TargetType:        targetType,
		TargetValue:       targetValue,
		DataDir:           dataDir,
		RequestedDataDir:  requestedDataDir,
		EffectiveDataDir:  dataDir,
		ManagedDataDir:    managedDataDir,
		SideRestore:       sideRestore,
		BackupSet:         backupSet,
		PatroniStopped:    patroniStopped,
		PostgresRestarted: postgresStarted,
		Exclusive:         opts != nil && opts.Exclusive,
		TargetAction:      targetActionFromOptions(opts),
		TargetTimeline:    targetTimelineFromOptions(opts),
		PostRestore:       postRestore,
		StartedAt:         start.Format(time.RFC3339),
		CompletedAt:       end.Format(time.RFC3339),
		DurationSeconds:   duration,
	}
}

func newPITRSuccessResult(state *SystemState, opts *Options, patroniStopped bool, postgresStarted bool, start time.Time, end time.Time, postRestore *PostRestoreState) *output.Result {
	data := newPITRResultData(state, opts, patroniStopped, postgresStarted, start, end, postRestore)
	return output.OK("pitr completed", data).
		WithNextActions(buildPostRestoreNextActions(state, opts, patroniStopped)...)
}

func targetTypeValueFromOptions(opts *Options) (string, string) {
	if opts == nil {
		return "unknown", ""
	}
	switch {
	case opts.Default:
		return "default", ""
	case opts.Immediate:
		return "immediate", ""
	case opts.Time != "":
		return "time", pgbackrest.NormalizeRestoreTime(opts.Time)
	case opts.Name != "":
		return "name", opts.Name
	case opts.LSN != "":
		return "lsn", opts.LSN
	case opts.XID != "":
		return "xid", opts.XID
	default:
		return "unknown", ""
	}
}

func targetActionFromOptions(opts *Options) string {
	if opts == nil {
		return ""
	}
	return opts.TargetAction
}

func targetTimelineFromOptions(opts *Options) string {
	if opts == nil {
		return ""
	}
	return opts.TargetTimeline
}
