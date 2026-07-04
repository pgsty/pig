package postgres

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sirupsen/logrus"
)

type filesystemProbe struct {
	Mount  string
	Type   string
	SizeGB int
}

var filesystemDFOutput = func(args ...string) (string, error) {
	out, err := exec.Command("df", args...).Output()
	return string(out), err
}

func detectFilesystem(path string) (filesystemProbe, error) {
	out, err := filesystemDFOutput("-T", path)
	if err != nil {
		return filesystemProbe{}, err
	}
	return parseFilesystemProbe(out)
}

func detectFilesystemAs(dbsu, path string) (filesystemProbe, error) {
	out, err := forkDBSUCommandOutput(dbsu, []string{"df", "-T", path})
	if err != nil {
		return filesystemProbe{}, err
	}
	return parseFilesystemProbe(out)
}

func detectDiskGB(path string) int {
	info, err := detectDiskFilesystem(path)
	if err == nil {
		return info.SizeGB
	}
	logrus.Debugf("df failed for %s: %v", path, err)

	parent := existingParent(path)
	if parent == "" || parent == path {
		return 0
	}
	info, err = detectDiskFilesystem(parent)
	if err != nil {
		logrus.Debugf("df fallback failed for %s (from %s): %v", parent, path, err)
		return 0
	}
	return info.SizeGB
}

func detectDiskFilesystem(path string) (filesystemProbe, error) {
	out, err := filesystemDFOutput("-k", path)
	if err != nil {
		return filesystemProbe{}, err
	}
	return parseFilesystemProbe(out)
}

func dfMountAndFS(path string) (string, string) {
	info, err := detectFilesystem(path)
	if err != nil {
		return "", ""
	}
	return info.Mount, info.Type
}

func dfMountAndFSAs(dbsu, path string) (string, string) {
	info, err := detectFilesystemAs(dbsu, path)
	if err != nil {
		return "", ""
	}
	return info.Mount, info.Type
}

func parseFilesystemProbe(out string) (filesystemProbe, error) {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 2 {
		return filesystemProbe{}, fmt.Errorf("df output has no data row")
	}
	fields := strings.Fields(lines[len(lines)-1])
	var fsType string
	var sizeField string
	var mount string
	switch {
	case len(fields) >= 7:
		fsType = fields[1]
		sizeField = fields[2]
		mount = fields[6]
	case len(fields) >= 6:
		sizeField = fields[1]
		mount = fields[5]
	default:
		return filesystemProbe{}, fmt.Errorf("df data row has too few fields")
	}
	kb, err := strconv.Atoi(sizeField)
	if err != nil {
		return filesystemProbe{}, fmt.Errorf("parse df size %q: %w", sizeField, err)
	}
	return filesystemProbe{
		Mount:  mount,
		Type:   fsType,
		SizeGB: kb / (1024 * 1024),
	}, nil
}

func existingParent(path string) string {
	candidate, err := filepath.Abs(strings.TrimSpace(path))
	if err != nil || candidate == "" {
		return ""
	}
	candidate = filepath.Clean(candidate)
	for candidate != "" && candidate != "." {
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			return candidate
		}
		next := filepath.Dir(candidate)
		if next == candidate {
			break
		}
		candidate = next
	}
	return ""
}
