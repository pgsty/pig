package fork

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"pig/internal/utils"
)

type ForkInfo struct {
	Kind      string         `json:"kind" yaml:"kind"`
	Version   int            `json:"version" yaml:"version"`
	Name      string         `json:"name" yaml:"name"`
	CreatedAt string         `json:"created_at,omitempty" yaml:"created_at,omitempty"`
	Source    ForkEndpoint   `json:"source" yaml:"source"`
	Target    ForkEndpoint   `json:"target" yaml:"target"`
	Copy      ForkCopyInfo   `json:"copy" yaml:"copy"`
	Backup    ForkBackupInfo `json:"backup" yaml:"backup"`
	Commands  ForkCommands   `json:"commands" yaml:"commands"`
	Orphan    bool           `json:"orphan,omitempty" yaml:"orphan,omitempty"`
}

type ForkEndpoint struct {
	Data    string `json:"data" yaml:"data"`
	Port    int    `json:"port,omitempty" yaml:"port,omitempty"`
	Started bool   `json:"started,omitempty" yaml:"started,omitempty"`
}

type ForkCopyInfo struct {
	Method         string `json:"method" yaml:"method"`
	Actual         string `json:"actual" yaml:"actual"`
	Filesystem     string `json:"filesystem,omitempty" yaml:"filesystem,omitempty"`
	SameFilesystem bool   `json:"same_filesystem,omitempty" yaml:"same_filesystem,omitempty"`
}

type ForkBackupInfo struct {
	Mode  string `json:"mode" yaml:"mode"`
	Label string `json:"label,omitempty" yaml:"label,omitempty"`
}

type ForkCommands struct {
	Connect string `json:"connect,omitempty" yaml:"connect,omitempty"`
	Stop    string `json:"stop,omitempty" yaml:"stop,omitempty"`
	Remove  string `json:"remove,omitempty" yaml:"remove,omitempty"`
}

func BuildForkInfo(opts *Options, state *State) ForkInfo {
	inst := opts.Instance
	info := ForkInfo{
		Kind:      "pg_fork",
		Version:   1,
		Name:      inst.Name,
		CreatedAt: time.Now().Format(time.RFC3339),
		Source: ForkEndpoint{
			Data: inst.SourceData,
			Port: inst.SourcePort,
		},
		Target: ForkEndpoint{
			Data:    inst.DestData,
			Port:    inst.DestPort,
			Started: opts.Start,
		},
		Copy: ForkCopyInfo{
			Method: "reflink_auto",
			Actual: string(CloneModeUnknown),
		},
		Backup: ForkBackupInfo{
			Mode: string(BackupModeUnknown),
		},
		Commands: ForkCommands{
			Connect: "psql -p " + strconv.Itoa(inst.DestPort),
			Stop:    "pg_ctl -D " + inst.DestData + " stop",
			Remove:  "rm -rf " + inst.DestData,
		},
	}
	if state != nil {
		info.Copy.Actual = string(state.CloneMode)
		info.Copy.Filesystem = state.FS
		info.Backup.Mode = string(state.BackupMode)
		info.Target.Started = state.Started
	}
	return info
}

func WriteForkInfoAs(dbsu, dataDir string, info ForkInfo) error {
	payload, err := marshalForkInfo(info)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp("", "pig-fork-info-*.json")
	if err != nil {
		return err
	}
	path := file.Name()
	defer os.Remove(path)
	if _, err := file.Write(payload); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Chmod(path, 0644); err != nil {
		return err
	}
	dest := filepath.Join(dataDir, "fork.json")
	if err := utils.DBSUCommand(dbsu, []string{"cp", path, dest}); err != nil {
		return err
	}
	return utils.DBSUCommand(dbsu, []string{"chmod", "0644", dest})
}

func marshalForkInfo(info ForkInfo) ([]byte, error) {
	payload, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return nil, err
	}
	payload = append(payload, '\n')
	return payload, nil
}

func ScanForks(root string) ([]ForkInfo, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []ForkInfo{}, nil
		}
		return nil, err
	}
	forks := make([]ForkInfo, 0)
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "data-") {
			continue
		}
		dataDir := filepath.Join(root, entry.Name())
		info, err := readForkInfo(dataDir)
		if err != nil {
			name := strings.TrimPrefix(entry.Name(), "data-")
			info = ForkInfo{
				Kind:    "pg_fork",
				Version: 1,
				Name:    name,
				Target:  ForkEndpoint{Data: dataDir},
				Orphan:  true,
			}
		}
		forks = append(forks, info)
	}
	sort.Slice(forks, func(i, j int) bool { return forks[i].Name < forks[j].Name })
	return forks, nil
}

func readForkInfo(dataDir string) (ForkInfo, error) {
	payload, err := os.ReadFile(filepath.Join(dataDir, "fork.json"))
	if err != nil {
		return ForkInfo{}, err
	}
	var info ForkInfo
	if err := json.Unmarshal(payload, &info); err != nil {
		return ForkInfo{}, err
	}
	if info.Target.Data == "" {
		info.Target.Data = dataDir
	}
	if info.Name == "" {
		info.Name = strings.TrimPrefix(filepath.Base(dataDir), "data-")
	}
	return info, nil
}
