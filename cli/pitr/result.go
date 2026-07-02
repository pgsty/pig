package pitr

import "time"

// PITRResultData represents structured output for a PITR execution.
type PITRResultData struct {
	Target            string            `json:"target" yaml:"target"`
	DataDir           string            `json:"data_dir" yaml:"data_dir"`
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
	Queried       bool   `json:"queried" yaml:"queried"`
	Running       bool   `json:"running" yaml:"running"`
	InRecovery    *bool  `json:"in_recovery,omitempty" yaml:"in_recovery,omitempty"`
	CurrentLSN    string `json:"current_lsn,omitempty" yaml:"current_lsn,omitempty"`
	TimelineID    string `json:"timeline_id,omitempty" yaml:"timeline_id,omitempty"`
	PatroniActive bool   `json:"patroni_active" yaml:"patroni_active"`
	Error         string `json:"error,omitempty" yaml:"error,omitempty"`
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
	if state != nil {
		dataDir = state.DataDir
	}

	duration := end.Sub(start).Seconds()
	if duration < 0 {
		duration = 0
	}

	return PITRResultData{
		Target:            target,
		DataDir:           dataDir,
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
