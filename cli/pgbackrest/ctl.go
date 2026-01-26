package pgbackrest

// Start enables pgBackRest operations for a stanza
func Start(cfg *Config) error {
	effCfg, err := GetEffectiveConfig(cfg)
	if err != nil {
		return err
	}

	return RunPgBackRest(effCfg, "start", nil, true)
}

// StopOptions holds options for stop command
type StopOptions struct {
	Force bool // Terminate running pgBackRest operations
}

// Stop disables pgBackRest operations for a stanza (for maintenance)
func Stop(cfg *Config, opts *StopOptions) error {
	effCfg, err := GetEffectiveConfig(cfg)
	if err != nil {
		return err
	}

	args := []string{}

	if opts.Force {
		args = append(args, "--force")
	}

	return RunPgBackRest(effCfg, "stop", args, true)
}

// Check verifies the backup repository integrity
func Check(cfg *Config) error {
	effCfg, err := GetEffectiveConfig(cfg)
	if err != nil {
		return err
	}

	return RunPgBackRest(effCfg, "check", nil, true)
}
