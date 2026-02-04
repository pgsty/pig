package status

import (
	"fmt"
	"pig/cli/ext"
	"pig/internal/config"
	"pig/internal/output"

	"github.com/sirupsen/logrus"
)

// StatusData represents structured output for the pig status command.
type StatusData struct {
	Pig      PigInfo       `json:"pig" yaml:"pig"`
	Host     HostInfo      `json:"host" yaml:"host"`
	Postgres *PostgresInfo `json:"postgres,omitempty" yaml:"postgres,omitempty"`
	Pigsty   *PigstyInfo   `json:"pigsty,omitempty" yaml:"pigsty,omitempty"`
}

// PigInfo represents pig build/runtime information.
type PigInfo struct {
	Version   string `json:"version" yaml:"version"`
	Revision  string `json:"revision" yaml:"revision"`
	BuildDate string `json:"build_date" yaml:"build_date"`
	GoVersion string `json:"go_version" yaml:"go_version"`
}

// HostInfo represents host environment information.
type HostInfo struct {
	OS       string `json:"os" yaml:"os"`
	Arch     string `json:"arch" yaml:"arch"`
	Distro   string `json:"distro" yaml:"distro"`
	Version  string `json:"version" yaml:"version"`
	Hostname string `json:"hostname" yaml:"hostname"`
	User     string `json:"user" yaml:"user"`
}

// PostgresInfo represents PostgreSQL installation information.
type PostgresInfo struct {
	Version      string `json:"version" yaml:"version"`
	MajorVersion int    `json:"major_version" yaml:"major_version"`
	BinDir       string `json:"bin_dir" yaml:"bin_dir"`
	ExtensionDir string `json:"extension_dir" yaml:"extension_dir"`
}

// PigstyInfo represents Pigsty environment information.
type PigstyInfo struct {
	Home      string `json:"home,omitempty" yaml:"home,omitempty"`
	Inventory string `json:"inventory,omitempty" yaml:"inventory,omitempty"`
}

// VersionData represents structured output for the pig version command.
type VersionData struct {
	Version   string `json:"version" yaml:"version"`
	Revision  string `json:"revision" yaml:"revision"`
	Branch    string `json:"branch" yaml:"branch"`
	BuildDate string `json:"build_date" yaml:"build_date"`
	GoVersion string `json:"go_version" yaml:"go_version"`
	OS        string `json:"os" yaml:"os"`
	Arch      string `json:"arch" yaml:"arch"`
}

// GetStatusResult returns a structured Result for the pig status command.
func GetStatusResult() *output.Result {
	pig := PigInfo{
		Version:   config.PigVersion,
		Revision:  config.Revision,
		BuildDate: config.BuildDate,
		GoVersion: config.GoVersion,
	}

	host := HostInfo{
		OS:       config.GOOS,
		Arch:     config.GOARCH,
		Distro:   config.OSVendor,
		Version:  config.OSVersionFull,
		Hostname: config.NodeHostname,
		User:     config.CurrentUser,
	}

	var pgInfo *PostgresInfo
	if err := ext.DetectPostgres(); err != nil {
		logrus.Debugf("failed to detect PostgreSQL: %v", err)
	}
	if ext.Active != nil {
		pg := ext.Active
		pgInfo = &PostgresInfo{
			Version:      pg.Version,
			MajorVersion: pg.MajorVersion,
			BinDir:       pg.BinPath,
			ExtensionDir: pg.ExtPath,
		}
	}

	var pigstyInfo *PigstyInfo
	if config.PigstyHome != "" || config.PigstyConfig != "" {
		pigstyInfo = &PigstyInfo{
			Home:      config.PigstyHome,
			Inventory: config.PigstyConfig,
		}
	}

	data := &StatusData{
		Pig:      pig,
		Host:     host,
		Postgres: pgInfo,
		Pigsty:   pigstyInfo,
	}

	return output.OK("status collected", data)
}

// GetVersionResult returns a structured Result for the pig version command.
func GetVersionResult() *output.Result {
	data := &VersionData{
		Version:   config.PigVersion,
		Revision:  config.Revision,
		Branch:    config.Branch,
		BuildDate: config.BuildDate,
		GoVersion: config.GoVersion,
		OS:        config.GOOS,
		Arch:      config.GOARCH,
	}

	return output.OK(fmt.Sprintf("pig version %s", config.PigVersion), data)
}
