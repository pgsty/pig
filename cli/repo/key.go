package repo

import (
	_ "embed"
	"fmt"
	"pig/internal/config"
	"pig/internal/utils"
)

//go:embed assets/key.gpg
var embeddedPigstyKey []byte

var (
	pigstyRPMKeyPath = "/etc/pki/rpm-gpg/RPM-GPG-KEY-pigsty"
	// The embedded key is ASCII-armored. Modern APT selects the parser from the
	// extension, so using .gpg here makes it reject the key as an unsupported
	// binary keyring. .asc works directly without gpg/apt-key/dearmor tooling.
	pigstyAPTKeyPath = "/etc/apt/keyrings/pigsty.asc"
	putPigstyKey     = utils.PutFileMode
)

// InstallPigstyKey installs the embedded Pigsty signing key at the path used by
// generated repository definitions. Keeping this in the repository package
// avoids curl/wget and apt-key dependencies during minimal-host bootstrap.
func InstallPigstyKey() error {
	path := ""
	switch config.OSType {
	case config.DistroEL:
		path = pigstyRPMKeyPath
	case config.DistroDEB:
		path = pigstyAPTKeyPath
	default:
		return fmt.Errorf("unsupported platform for Pigsty signing key: %s", config.OSType)
	}
	if len(embeddedPigstyKey) == 0 {
		return fmt.Errorf("embedded Pigsty signing key is empty")
	}
	if err := putPigstyKey(path, embeddedPigstyKey, 0o644); err != nil {
		return fmt.Errorf("install Pigsty signing key: %w", err)
	}
	return nil
}
