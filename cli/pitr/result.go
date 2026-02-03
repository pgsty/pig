package pitr

import "time"

// PITRResultData represents structured output for a PITR execution.
type PITRResultData struct {
	Target            string  `json:"target" yaml:"target"`
	DataDir           string  `json:"data_dir" yaml:"data_dir"`
	BackupSet         string  `json:"backup_set" yaml:"backup_set"`
	PatroniStopped    bool    `json:"patroni_stopped" yaml:"patroni_stopped"`
	PostgresRestarted bool    `json:"postgres_restarted" yaml:"postgres_restarted"`
	Promote           bool    `json:"promote" yaml:"promote"`
	Exclusive         bool    `json:"exclusive" yaml:"exclusive"`
	StartedAt         string  `json:"started_at" yaml:"started_at"`
	CompletedAt       string  `json:"completed_at" yaml:"completed_at"`
	DurationSeconds   float64 `json:"duration_seconds" yaml:"duration_seconds"`
}

func newPITRResultData(state *SystemState, opts *Options, patroniStopped bool, postgresStarted bool, start time.Time, end time.Time) PITRResultData {
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
		Promote:           opts != nil && opts.Promote,
		Exclusive:         opts != nil && opts.Exclusive,
		StartedAt:         start.Format(time.RFC3339),
		CompletedAt:       end.Format(time.RFC3339),
		DurationSeconds:   duration,
	}
}
