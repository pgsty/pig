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
	importPigstyKeys = utils.SudoCommand
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
	var implicit []*Repository
	for _, repository := range repositories {
		if repository.hasExplicitSigningKey() {
			if err := m.prepareExplicitPigstyKey(repository); err != nil {
				return "", fmt.Errorf("%s explicitly configures a signing key: %w", repository.Name, err)
			}
			if check {
				m.enablePigstyGPGCheck(repository)
			}
		} else {
			implicit = append(implicit, repository)
		}
	}
	if len(implicit) == 0 {
		return "", nil
	}
	err := InstallPigstyKey()
	// Keep the implicit reference stable even on failure: APT rejects two
	// definitions of the same source with different Signed-By values.
	field, key := "gpgkey", "file://"+pigstyRPMKeyPath
	if m.OsType == config.DistroDEB {
		field, key = "signed-by", pigstyAPTKeyPath
	}
	for _, repository := range implicit {
		repository.Meta[field] = key
		if err != nil {
			if m.OsType == config.DistroDEB {
				repository.Meta["trusted"] = "yes"
			} else {
				repository.Meta["gpgcheck"] = "0"
				repository.Meta["repo_gpgcheck"] = "0"
			}
		} else if check {
			m.enablePigstyGPGCheck(repository)
		}
	}
	if err != nil {
		return fmt.Sprintf("Pigsty signing key unavailable; continuing with signature verification disabled for selected Pigsty repositories: %v", err), nil
	}
	return "", nil
}

// Explicit references name the required key, not an unrelated default file.
// RPM owns key acquisition/import; APT signed-by names local keyring files.
func (m *Manager) prepareExplicitPigstyKey(repository *Repository) error {
	if m.OsType == config.DistroEL {
		if strings.TrimSpace(repository.Meta["signed-by"]) != "" {
			return fmt.Errorf("use gpgkey for RPM repositories")
		}
		keys := strings.Fields(repository.Meta["gpgkey"])
		for _, key := range keys {
			if strings.TrimPrefix(key, "file://") == pigstyRPMKeyPath {
				if err := InstallPigstyKey(); err != nil {
					return err
				}
			}
		}
		if err := importPigstyKeys(append([]string{"rpm", "--import", "--"}, keys...)); err != nil {
			return fmt.Errorf("import explicitly configured RPM signing key: %w", err)
		}
		return nil
	}
	if strings.TrimSpace(repository.Meta["gpgkey"]) != "" {
		return fmt.Errorf("use signed-by for APT repositories")
	}
	for _, key := range strings.Split(repository.Meta["signed-by"], ",") {
		key = strings.TrimSpace(key)
		if key == pigstyAPTKeyPath {
			if err := InstallPigstyKey(); err != nil {
				return err
			}
			continue
		}
		if !filepath.IsAbs(key) {
			return fmt.Errorf("signed-by must name an absolute keyring file: %q", key)
		}
		info, err := os.Stat(key)
		if err != nil {
			return fmt.Errorf("required signing key %q: %w", key, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("required signing key %q is not a regular file", key)
		}
	}
	return nil
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
