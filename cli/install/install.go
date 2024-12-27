package install

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"embed"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
)

//go:embed assets/pigsty-v3.2.0.tgz
var embeddedTarball []byte

const DefaultDir = "~/pigsty"

func dummy() {
	_ = embed.FS{}
}

// InstallPigsty installs pigsty to the specified directory.
// If targetDir is empty, it defaults to ~/pigsty.
// If overwrite is true, it will overwrite existing files.
// Returns error if installation fails.
func InstallPigsty(srcTarball []byte, targetDir string, overwrite bool) error {
	if srcTarball == nil {
		srcTarball = embeddedTarball
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

	// Extract content to target directory
	if err := extractPigsty(srcTarball, targetDir); err != nil {
		return fmt.Errorf("failed to extract pigsty: %v", err)
	}

	logrus.Infof("Pigsty installed @ %s", targetDir)
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

// LoadPigsty loads pigsty source tarball from given path and returns the byte array
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
