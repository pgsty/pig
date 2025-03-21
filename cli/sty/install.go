package sty

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	_ "embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"pig/cli/license"
	"strings"

	"github.com/sirupsen/logrus"
)

//go:embed assets/EULA.md
var embeddedEULA []byte

const DefaultDir = "~/pigsty"

// InstallPigsty installs pigsty to the specified directory.
// If targetDir is empty, it defaults to ~/pigsty.
// If overwrite is true, it will overwrite existing files.
// Returns error if installation fails.
func InstallPigsty(srcTarball []byte, targetDir string, overwrite bool) error {
	if srcTarball == nil {
		return fmt.Errorf("embedded tarball not found")
		// srcTarball = embeddedTarball
	}
	if targetDir == "" {
		targetDir = DefaultDir
	}

	// Expand ~ to home directory
	if strings.HasPrefix(targetDir, "~/") {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get user home directory: %v", err)
		}
		targetDir = filepath.Join(homeDir, targetDir[2:])
	}

	// Check if target directory exists
	if exists := pathExists(targetDir); exists {
		if !overwrite {
			return fmt.Errorf("target directory %s already exists, use -f|--force flag to overwrite", targetDir)
		}
		logrus.Warnf("target directory %s already exists, overwriting", targetDir)
	} else if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory %s: %v", targetDir, err)
	}

	// if a valid license is found, goes to pro mode, ask user to agree the EULA
	if license.Manager.Valid {
		// fmt.Println("------------------------------------------------------------------------------")
		fmt.Println("##############################################################################")
		fmt.Println(string(embeddedEULA))
		fmt.Println("##############################################################################\n\n")
		logrus.Warnf("To proceed with the installation of Pigsty Pro Edition, you must agree to the End User License Agreement (EULA).")
		logrus.Infof("Do you accept the terms of the EULA? Please type 'yes' to accept or 'no' to decline:")
		fmt.Printf("> ")
		var response string
		fmt.Scanln(&response)
		response = strings.ToLower(strings.TrimSpace(response))
		switch strings.ToLower(response) {
		case "no", "n", "nay", "off", "false":
			logrus.Errorf("Installation aborted due to EULA rejection.")
			logrus.Warnf("Consider using the AGPLv3 OSS version instead (remove the license file)")
			return fmt.Errorf("installation aborted due to EULA rejection")
		case "yes", "y", "ok", "true":
			logrus.Infof("EULA accepted, continue pigsty pro installation")
		default:
			logrus.Errorf("Invalid response: %s", response)
			return fmt.Errorf("invalid response to EULA inquiry")
		}
	}

	// Extract content to target directory
	if err := extractPigsty(srcTarball, targetDir); err != nil {
		return fmt.Errorf("failed to extract pigsty: %v", err)
	}

	if license.Manager.Valid {
		licensePath := filepath.Join(targetDir, "EULA.md")
		if err := os.WriteFile(licensePath, embeddedEULA, 0644); err != nil {
			return fmt.Errorf("failed to write EULA file: %v", err)
		}
		logrus.Infof("the EULA is generated at %s", licensePath)
	}

	logrus.Infof("pigsty installed @ %s", targetDir)
	logrus.Infof("pig sty boot    # install ansible and prepare offline pkg")
	logrus.Infof("pig sty conf    # configure pigsty and generate config")
	logrus.Infof("pig sty install # install & provisioning env (DANGEROUS!)")

	return nil
}

// extractPigsty extracts pigsty source from embedded tarball to destination directory.
// It handles files, directories and symlinks while preserving file modes.
// Protected files like pigsty.yml and files/pki/* are not overwritten if they exist.
func extractPigsty(data []byte, dst string) error {
	buf := bytes.NewReader(data)
	gzr, err := gzip.NewReader(buf)
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %v", err)
	}
	defer gzr.Close()

	tarReader := tar.NewReader(gzr)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read tar header: %v", err)
		}
		if header == nil {
			continue
		}

		// Get relative path and construct target path
		relPath := header.Name
		parts := strings.SplitN(relPath, "/", 2)
		if len(parts) <= 1 {
			continue // Skip root directory
		}
		relPath = parts[1]
		target := filepath.Join(dst, relPath)

		// Skip protected files and directories
		if isProtectedFile(relPath, dst) {
			logrus.Warnf("Skipping overwriting existing file: %s", relPath)
			continue
		}

		if err := extractTarEntry(header, target, tarReader); err != nil {
			return fmt.Errorf("failed to extract %s: %v", target, err)
		}
	}

	return nil
}

// LoadPigstySrc loads pigsty source tarball from given path and returns the byte array
func LoadPigstySrc(path string) ([]byte, error) {
	// Open and read the file
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %v", path, err)
	}
	defer f.Close()

	// Read file contents into byte slice
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %v", path, err)
	}

	return data, nil
}

// isProtectedFile checks if a file should be protected from overwriting
func isProtectedFile(relPath string, dst string) bool {
	// fmt.Println(relPath)
	switch {
	case filepath.Base(relPath) == "pigsty.yml" && fileExists(filepath.Join(dst, "pigsty.yml")):
		return true
	case strings.HasPrefix(relPath, "files/pki") && !strings.HasSuffix(relPath, "/"):
		return true
	default:
		return false
	}
}

// extractTarEntry handles extraction of a single tar entry
func extractTarEntry(header *tar.Header, target string, reader *tar.Reader) error {
	switch header.Typeflag {
	case tar.TypeDir:
		return os.MkdirAll(target, os.FileMode(header.Mode))

	case tar.TypeReg:
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return fmt.Errorf("failed to create parent directory: %v", err)
		}

		f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR|os.O_TRUNC, os.FileMode(header.Mode))
		if err != nil {
			return fmt.Errorf("failed to create file: %v", err)
		}
		defer f.Close()

		if _, err := io.Copy(f, reader); err != nil {
			return fmt.Errorf("failed to write file contents: %v", err)
		}
	case tar.TypeSymlink:
		os.Remove(target) // Remove existing symlink if any
		if err := os.Symlink(header.Linkname, target); err != nil {
			return fmt.Errorf("failed to create symlink: %v", err)
		}

	default:
		logrus.Warnf("Skipping unsupported file type %v: %s", header.Typeflag, target)
	}

	return nil
}

// fileExists checks if a path exists and is a regular file
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// pathExists checks if a path exists (can be file or directory)
func pathExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// if path exists and is a directory
func dirExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return info.IsDir()
}
