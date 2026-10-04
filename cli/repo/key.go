package repo

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"pig/internal/config"
	"pig/internal/utils"
	"slices"
	"strings"
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
	statPigstyKey    = os.Stat
)

func (m *Manager) selectedPigstyRepositories(modules []string) []*Repository {
	var repositories []*Repository
	for _, name := range []string{"pigsty-infra", "pigsty-pgsql"} {
		repository := m.Map[name]
		if repository == nil || !repository.Available(config.OSCode, config.OSArch) {
			continue
		}
		for _, module := range modules {
			if slices.Contains(m.Module[module], name) {
				repositories = append(repositories, repository)
				break
			}
		}
	}
	return repositories
}

func (r *Repository) hasExplicitSigningKey() bool {
	return strings.TrimSpace(r.Meta["gpgkey"]) != "" || strings.TrimSpace(r.Meta["signed-by"]) != ""
}

// preparePigstyKey treats the implicit Pigsty key as a convenience. A key named
// explicitly in selected repository metadata makes preparation mandatory.
func (m *Manager) preparePigstyKey(modules []string, check bool) (string, error) {
	repositories := m.selectedPigstyRepositories(modules)
	if len(repositories) == 0 {
		return "", nil
	}
	if err := InstallPigstyKey(); err != nil {
		for _, repository := range repositories {
			if repository.hasExplicitSigningKey() {
				return "", fmt.Errorf("%s explicitly configures a signing key: %w", repository.Name, err)
			}
		}
		for _, repository := range repositories {
			if m.OsType == config.DistroDEB {
				delete(repository.Meta, "signed-by")
				repository.Meta["trusted"] = "yes"
			} else {
				delete(repository.Meta, "gpgkey")
				repository.Meta["gpgcheck"] = "0"
				repository.Meta["repo_gpgcheck"] = "0"
			}
		}
		return fmt.Sprintf("Pigsty signing key unavailable; continuing with signature verification disabled for selected Pigsty repositories: %v", err), nil
	}
	// APT does not discover keys in /etc/apt/keyrings automatically. Reference
	// the prepared file even when retaining the permissive signature policy.
	field, key := "gpgkey", "file://"+pigstyRPMKeyPath
	if m.OsType == config.DistroDEB {
		field, key = "signed-by", pigstyAPTKeyPath
	}
	for _, repository := range repositories {
		if strings.TrimSpace(repository.Meta[field]) == "" {
			repository.Meta[field] = key
		}
	}
	if check {
		m.enablePigstyGPGCheck()
	}
	return "", nil
}

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
	// Reuse an existing file without requiring read or write access. If the
	// existence check fails (including permission errors), attempt installation.
	if info, err := statPigstyKey(path); err == nil && info.Mode().IsRegular() {
		return nil
	}
	if len(embeddedPigstyKey) == 0 {
		return fmt.Errorf("embedded Pigsty signing key is empty")
	}
	// A privileged mv would otherwise accept a directory destination and place
	// the temporary file inside it instead of installing the key at this path.
	if info, err := os.Lstat(path); err == nil && !info.Mode().IsRegular() {
		return fmt.Errorf("install Pigsty signing key: destination %q is not a regular file", path)
	}
	// repo add/set may run as a regular user on a minimal host where the key
	// directory does not yet exist. Create it through the same sudo fallback.
	if err := utils.Mkdir(filepath.Dir(path)); err != nil {
		return fmt.Errorf("create Pigsty signing key directory: %w", err)
	}
	if err := putPigstyKey(path, embeddedPigstyKey, 0o644); err != nil {
		return fmt.Errorf("install Pigsty signing key: %w", err)
	}
	return nil
}
