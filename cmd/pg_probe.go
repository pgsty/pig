package cmd

import (
	"fmt"
	"pig/cli/ext"
	"strconv"

	"github.com/sirupsen/logrus"
)

type pgMajorProbeOptions struct {
	Version        int
	PGConfig       string
	DefaultVersion int

	BothSetError  func() error
	PGConfigError func(err error) error
}

func probePostgresMajorVersion(opts pgMajorProbeOptions) (int, error) {
	if opts.Version != 0 && opts.PGConfig != "" {
		if opts.BothSetError != nil {
			return 0, opts.BothSetError()
		}
		return 0, fmt.Errorf("both pg version and pg_config path are specified, please specify only one")
	}

	// Detect postgres installation, but don't fail if not found.
	if err := ext.DetectPostgres(); err != nil {
		logrus.Debugf("failed to detect PostgreSQL: %v", err)
	}

	// If a PG major version is specified, try to validate it (but don't hard-fail).
	if opts.Version != 0 {
		if _, err := ext.GetPostgres(strconv.Itoa(opts.Version)); err != nil {
			logrus.Debugf("PostgreSQL installation %d not found: %v , but it's ok", opts.Version, err)
		}
		return opts.Version, nil
	}

	// If pg_config is specified, we must find the actual installation.
	if opts.PGConfig != "" {
		if _, err := ext.GetPostgres(opts.PGConfig); err != nil {
			if opts.PGConfigError != nil {
				return 0, opts.PGConfigError(err)
			}
			return 0, err
		}
		return ext.Postgres.MajorVersion, nil
	}

	// Fall back to active installation when present.
	if ext.Active != nil {
		logrus.Debugf("fallback to active PostgreSQL: %d", ext.Active.MajorVersion)
		ext.Postgres = ext.Active
		return ext.Active.MajorVersion, nil
	}

	// Final fallback when configured.
	if opts.DefaultVersion != 0 {
		logrus.Debugf("no active PostgreSQL found, fall back to the latest Major %d", opts.DefaultVersion)
		return opts.DefaultVersion, nil
	}

	logrus.Debugf("no active PostgreSQL found, but it's ok")
	return 0, nil
}
