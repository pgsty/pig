package repo

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"pig/cli/get"
	"pig/cli/utils"
	"pig/internal/config"
	"runtime"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"golang.org/x/crypto/openpgp/armor"
	"gopkg.in/yaml.v3"
)

var (
	//go:embed assets/rpm.yml
	embedRpmRepo []byte

	//go:embed assets/deb.yml
	embedDebRepo []byte

	//go:embed assets/key.gpg
	embedGPGKey []byte
)

const (
	pigstyRpmGPGPath = "/etc/pki/rpm-gpg/RPM-GPG-KEY-pigsty"
	pigstyDebGPGPath = "/etc/apt/keyrings/pigsty.gpg"
)

/********************
* Global Vars
********************/

var (
	Repos          []*Repository
	RepoMap        map[string]*Repository
	ModuleMap      map[string][]string = make(map[string][]string)
	PigstyGPGCheck bool                = false
)

/********************
* Repo Data Type
********************/

// Repository represents a package repository configuration
type Repository struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description"`
	Module      string            `yaml:"module"`
	Releases    []int             `yaml:"releases"`
	Arch        []string          `yaml:"arch"`
	BaseURL     map[string]string `yaml:"baseurl"`
	Meta        map[string]string `yaml:"-"`
	Distro      string            `yaml:"-"` // el|deb
}

func (r *Repository) SupportAmd64() bool {
	return slices.Contains(r.Arch, "x86_64")
}

func (r *Repository) SupportArm64() bool {
	return slices.Contains(r.Arch, "aarch64")
}

func (r *Repository) GetBaseURL(region string) string {
	if url, ok := r.BaseURL[region]; ok {
		return url
	}
	return r.BaseURL["default"]
}

// Available checks if the repository is available for a given distribution and architecture
func (r *Repository) Available(code string, arch string) bool {
	code = strings.ToLower(code)
	arch = strings.ToLower(arch)
	if arch == "amd64" || arch == "x86_64" {
		// convert arch to x86_64 or aarch64
		arch = "x86_64"
	} else if arch == "arm64" || arch == "aarch64" {
		arch = "aarch64"
	}
	major := GetMajorVersionFromCode(code)
	if code != "" && (major == -1 || !slices.Contains(r.Releases, major)) {
		return false
	}
	if arch != "" && !slices.Contains(r.Arch, arch) {
		return false
	}
	return true
}

func (r *Repository) String() string {
	json, err := json.Marshal(r)
	if err != nil {
		return fmt.Sprintf("%s: %s", r.Name, r.Description)
	}
	return string(json)
}

func (r *Repository) Content(region ...string) string {
	regionStr := "default"
	if len(region) > 0 {
		regionStr = region[0]
	}
	if r.Distro == config.DistroEL {
		rpmMeta := ""
		// Get sorted keys
		keys := make([]string, 0, len(r.Meta))
		for k := range r.Meta {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		// Add meta in sorted order
		for _, k := range keys {
			rpmMeta += fmt.Sprintf("%s=%s\n", k, r.Meta[k])
		}
		return fmt.Sprintf("[%s]\nname=%s\nbaseurl=%s\n%s", r.Name, r.Name, r.GetBaseURL(regionStr), rpmMeta)
	}
	if r.Distro == config.DistroDEB {
		// Get sorted keys
		keys := make([]string, 0, len(r.Meta))
		for k := range r.Meta {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		// Build meta string in sorted order
		debMeta := ""
		for _, k := range keys {
			debMeta += fmt.Sprintf("%s=%s ", k, r.Meta[k])
		}
		debMeta = strings.TrimSpace(debMeta)
		repoURL := r.GetBaseURL(regionStr)
		repoURL = strings.ReplaceAll(repoURL, "${distro_codename}", config.OSVersionCode)
		repoURL = strings.ReplaceAll(repoURL, "${distro_name}", config.OSVendor)
		return fmt.Sprintf("# %s %s\ndeb [%s] %s", r.Name, r.Description, debMeta, repoURL)
	}
	return ""
}

/********************
* Repo API Command
********************/

// AddRepo adds the Pigsty repository to the system
func AddRepo(region string, modules ...string) error {
	// if "all" in modules, replace it with node, pgsql
	if slices.Contains(modules, "all") {
		modules = append(modules, "node", "pigsty", "pgdg")
		modules = slices.DeleteFunc(modules, func(module string) bool {
			return module == "all"
		})
	}
	// if "pgsql" in modules, remove "pgdg"
	if slices.Contains(modules, "pgsql") {
		modules = slices.DeleteFunc(modules, func(module string) bool {
			return module == "pgdg"
		})
	}
	modules = slices.Compact(modules)
	slices.Sort(modules)

	// load repo data according to OS type
	var err error
	if config.OSType == config.DistroEL {
		err = LoadRpmRepo(embedRpmRepo)
	} else if config.OSType == config.DistroDEB {
		err = LoadDebRepo(embedDebRepo)
	}
	if err != nil {
		return err
	}

	// check module availability
	for _, module := range modules {
		if _, ok := ModuleMap[module]; !ok {
			logrus.Warnf("available modules: %v", strings.Join(GetModuleList(), ", "))
			return fmt.Errorf("module %s not found", module)
		}
	}

	// infer region if not set
	if region == "" {
		get.Timeout = time.Second
		get.NetworkCondition()
		if !get.InternetAccess {
			logrus.Warn("no internet access, assume region = default")
			region = "default"
		} else {
			logrus.Infof("infer region %s from network condition", get.Region)
			region = get.Region
		}
	}
	logrus.Infof("add repo for %s.%s , region = %s", config.OSCode, config.OSArch, region)

	// we don't check gpg key by default (since you may have to sudo to add keys)
	if PigstyGPGCheck && (containsAny(modules, "pgsql", "infra", "pigsty", "all")) {
		switch config.OSType {
		case config.DistroDEB:
			err = AddDebGPGKey()
		case config.DistroEL:
			err = AddRpmGPGKey()
		}
		if err != nil {
			return err
		}
	}

	for _, module := range modules {
		if err := AddModule(module, region); err != nil {
			return err
		}
		logrus.Infof("add repo module: %s", module)
	}
	return nil
}

// AddModule adds a module to the system
func AddModule(module string, region string) error {
	modulePath := ModuleRepoPath(module)
	moduleContent := ModuleRepoConfig(module, region)

	randomFile := filepath.Join(os.TempDir(), fmt.Sprintf("%s-%s.repo", module, strconv.FormatInt(time.Now().UnixNano(), 36)))
	logrus.Debugf("write module %s to %s, content: %s", module, randomFile, moduleContent)
	if err := os.WriteFile(randomFile, []byte(moduleContent), 0644); err != nil {
		return err
	}
	defer os.Remove(randomFile)
	logrus.Debugf("sudo move %s to %s", randomFile, modulePath)
	return utils.SudoCommand([]string{"mv", "-f", randomFile, modulePath})
}

// BackupRepo makes a backup of the current repo files (sudo required)
func BackupRepo() error {
	var backupDir, repoPattern string

	if config.OSType == config.DistroEL {
		backupDir = "/etc/yum.repos.d/backup"
		repoPattern = "/etc/yum.repos.d/*.repo"
		logrus.Warn("old repos = moved to /etc/yum.repos.d/backup")
	} else if config.OSType == config.DistroDEB {
		backupDir = "/etc/apt/backup"
		repoPattern = "/etc/apt/sources.list.d/*"
		logrus.Warn("old repos = moved to /etc/apt/backup")
	}

	// Create backup directory and move files using sudo
	if err := utils.SudoCommand([]string{"mkdir", "-p", backupDir}); err != nil {
		return err
	}

	files, err := filepath.Glob(repoPattern)
	if err != nil {
		return err
	}

	for _, file := range files {
		dest := filepath.Join(backupDir, filepath.Base(file))
		logrus.Debugf("backup %s to %s", file, dest)
		if err := utils.SudoCommand([]string{"mv", "-f", file, dest}); err != nil {
			logrus.Errorf("failed to backup %s to %s: %v", file, dest, err)
		}
	}

	if config.OSType == config.DistroDEB {
		debSourcesList := "/etc/apt/sources.list"
		if fileInfo, err := os.Stat(debSourcesList); err == nil && fileInfo.Size() > 0 {
			if err := utils.SudoCommand([]string{"mv", "-f", debSourcesList, filepath.Join(backupDir, "sources.list")}); err != nil {
				return err
			}
			if err := utils.SudoCommand([]string{"touch", debSourcesList}); err != nil {
				return err
			}
		}
	}

	return nil
}

// RemoveRepo removes the Pigsty repository from the system
func RemoveRepo(modules ...string) error {
	var rmFileList []string
	for _, module := range modules {
		if module != "" {
			rmFileList = append(rmFileList, ModuleRepoPath(module))
		}
	}
	if len(rmFileList) == 0 {
		return fmt.Errorf("no module specified")
	}
	rmCmd := []string{"rm", "-f"}
	rmCmd = append(rmCmd, rmFileList...)
	logrus.Warnf("remove repo with: %s", strings.Join(rmCmd, " "))
	return utils.SudoCommand(rmCmd)
}

// AddPigstyRpmRepo adds the Pigsty RPM repository to the system
func AddPigstyRpmRepo(region string) error {
	if err := RpmPrecheck(); err != nil {
		return err
	}
	LoadRpmRepo(embedRpmRepo)
	if region == "" { // check network condition (if region is not set)
		get.Timeout = time.Second
		get.NetworkCondition()
		if !get.InternetAccess {
			logrus.Warn("no internet access, assume region = default")
			region = "default"
		}
	}

	// write gpg key
	err := TryReadMkdirWrite(pigstyRpmGPGPath, embedGPGKey)
	if err != nil {
		return err
	}
	logrus.Infof("import gpg key B9BD8B20 to %s", pigstyRpmGPGPath)

	// write repo file
	repoContent := ModuleRepoConfig("pigsty", region)
	err = TryReadMkdirWrite(ModuleRepoPath("pigsty"), []byte(repoContent))
	if err != nil {
		return err
	}
	logrus.Infof("repo added: %s", ModuleRepoPath("pigsty"))
	return nil
}

// RemovePigstyRpmRepo removes the Pigsty RPM repository from the system
func RemovePigstyRpmRepo() error {
	if err := RpmPrecheck(); err != nil {
		return err
	}

	// wipe pigsty repo file
	err := WipeFile(ModuleRepoPath("pigsty"))
	if err != nil {
		return err
	}
	logrus.Infof("remove %s", ModuleRepoPath("pigsty"))

	// wipe pigsty gpg file
	err = WipeFile(pigstyRpmGPGPath)
	if err != nil {
		return err
	}
	logrus.Infof("remove gpg key B9BD8B20 from %s", pigstyRpmGPGPath)
	return nil
}

// ListPigstyRpmRepo lists the Pigsty RPM repository
func ListPigstyRpmRepo() error {
	if err := RpmPrecheck(); err != nil {
		return err
	}
	fmt.Println("\n======== ls /etc/yum.repos.d/ ")
	if err := utils.ShellCommand([]string{"ls", "/etc/yum.repos.d/"}); err != nil {
		return err
	}
	fmt.Println("\n======== yum repolist")
	if err := utils.ShellCommand([]string{"yum", "repolist"}); err != nil {
		return err
	}
	return nil
}

// RpmPrecheck checks if the system is an EL distro
func RpmPrecheck() error {
	if runtime.GOOS != "linux" { // check if linux
		return fmt.Errorf("pigsty works on linux, unsupported os: %s", runtime.GOOS)
	}
	if config.OSType != config.DistroEL { // check if EL distro
		return fmt.Errorf("can not add rpm repo to %s distro", config.OSType)
	}
	return nil
}

// AddPigstyDebRepo adds the Pigsty DEB repository to the system
func AddPigstyDebRepo(region string) error {
	if err := DebPrecheck(); err != nil {
		return err
	}
	LoadDebRepo(embedDebRepo)

	if region == "" { // check network condition (if region is not set)
		get.Timeout = time.Second
		get.NetworkCondition()
		if !get.InternetAccess {
			logrus.Warn("no internet access, assume region = default")
			region = "default"
		}
	}

	// write gpg key
	err := AddDebGPGKey()
	if err != nil {
		return err
	}
	logrus.Infof("import gpg key B9BD8B20 to %s", pigstyDebGPGPath)

	// write repo file
	repoContent := ModuleRepoConfig("pigsty", region)
	err = TryReadMkdirWrite(ModuleRepoPath("pigsty"), []byte(repoContent))
	if err != nil {
		return err
	}
	logrus.Infof("repo added: %s", ModuleRepoPath("pigsty"))
	return nil
}

// RemovePigstyDebRepo removes the Pigsty DEB repository from the system
func RemovePigstyDebRepo() error {
	if err := DebPrecheck(); err != nil {
		return err
	}

	// wipe pigsty repo file
	err := WipeFile(ModuleRepoPath("pigsty"))
	if err != nil {
		return err
	}
	logrus.Infof("remove %s", ModuleRepoPath("pigsty"))

	// wipe pigsty gpg file
	err = WipeFile(pigstyDebGPGPath)
	if err != nil {
		return err
	}
	logrus.Infof("remove gpg key B9BD8B20 from %s", pigstyDebGPGPath)
	return nil
}

// ListPigstyDebRepo lists the Pigsty DEB repository
func ListPigstyDebRepo() error {
	if err := DebPrecheck(); err != nil {
		return err
	}

	fmt.Println("\n======== ls /etc/apt/sources.list.d/")
	if err := utils.ShellCommand([]string{"ls", "/etc/apt/sources.list.d/"}); err != nil {
		return err
	}

	// also check /etc/apt/sources.list if exists and non empty, print it
	if fileInfo, err := os.Stat("/etc/apt/sources.list"); err == nil {
		if fileInfo.Size() > 0 {
			fmt.Println("\n======== /etc/apt/sources.list")
			if err := utils.ShellCommand([]string{"cat", "/etc/apt/sources.list"}); err != nil {
				return err
			}
		}
	}

	fmt.Println("\n===== [apt-cache policy] =========================")
	if err := utils.ShellCommand([]string{"apt-cache", "policy"}); err != nil {
		return err
	}
	return nil
}

// DebPrecheck checks if the system is a DEB distro
func DebPrecheck() error {
	if runtime.GOOS != "linux" { // check if linux
		return fmt.Errorf("pigsty works on linux, unsupported os: %s", runtime.GOOS)
	}
	if config.OSType != config.DistroDEB { // check if DEB distro
		return fmt.Errorf("can not add deb repo to %s distro", config.OSType)
	}
	return nil
}

// TryReadMkdirWrite will try to read file and compare content, if same, return nil, otherwise write content to file, and make sure the directory exists
func TryReadMkdirWrite(filePath string, content []byte) error {
	// if it is permission denied, don't try to write
	if _, err := os.Stat(filePath); err == nil {
		if target, err := os.ReadFile(filePath); err == nil {
			if string(target) == string(content) {
				return nil
			}
		}
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return err //fmt.Errorf("failed to create parent directories: %v", err)
	}
	return os.WriteFile(filePath, content, 0644)
}

// AddDebGPGKey adds the Pigsty GPG key to the Debian repository
func AddDebGPGKey() error {
	block, _ := armor.Decode(bytes.NewReader(embedGPGKey))
	keyBytes, err := io.ReadAll(block.Body)
	if err != nil {
		return fmt.Errorf("failed to read GPG key: %v", err)
	}
	return TryReadMkdirWrite(pigstyDebGPGPath, keyBytes)
}

// AddRpmGPGKey adds the Pigsty GPG key to the RPM repository
func AddRpmGPGKey() error {
	return TryReadMkdirWrite(pigstyRpmGPGPath, embedGPGKey)
}

// ModuleRepoPath returns the path to the repository configuration file for a given module
func ModuleRepoPath(moduleName string) string {
	if config.OSType == config.DistroEL {
		return fmt.Sprintf("/etc/yum.repos.d/%s.repo", moduleName)
	} else if config.OSType == config.DistroDEB {
		return fmt.Sprintf("/etc/apt/sources.list.d/%s.list", moduleName)
	}
	return fmt.Sprintf("/tmp/%s.repo", moduleName)
}

// ModuleRepoConfig generates the repository configuration for a given module
func ModuleRepoConfig(moduleName string, region string) (content string) {
	var repoContent string
	if module, ok := ModuleMap[moduleName]; ok {
		for _, repoName := range module {
			if repo, ok := RepoMap[repoName]; ok {
				if RepoMap[repoName].Available(config.OSCode, config.OSArch) {
					repoContent += repo.Content(region) + "\n"
				}
			}
		}
	}
	return repoContent
}

// WipeFile removes a file, if permission denied, try to remove with sudo
func WipeFile(filePath string) error {
	// if not exists, just return
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		logrus.Debugf("file not exists, do nothing : %s %v", filePath, err)
		return nil
	}
	return os.Remove(filePath)
}

/********************
* Load Repo Data
********************/

// LoadRpmRepo loads RPM repository configurations
func LoadRpmRepo(data []byte) error {
	if data == nil {
		data = embedRpmRepo
	}
	var tmpRepos []Repository
	if err := yaml.Unmarshal(data, &tmpRepos); err != nil {
		return fmt.Errorf("failed to parse rpm repo: %v", err)
	}

	// Filter available repos and build maps
	Repos = make([]*Repository, 0)
	RepoMap = make(map[string]*Repository)
	ModuleMap = make(map[string][]string)

	for i := range tmpRepos {
		repo := &tmpRepos[i]
		repo.Distro = config.DistroEL
		repo.Meta = map[string]string{"enabled": "1", "gpgcheck": "0", "module_hotfixes": "1"}
		if repo.Available(config.OSCode, config.OSArch) {
			Repos = append(Repos, repo)
			RepoMap[repo.Name] = repo
			if repo.Module != "" {
				if _, exists := ModuleMap[repo.Module]; !exists {
					ModuleMap[repo.Module] = make([]string, 0)
				}
				ModuleMap[repo.Module] = append(ModuleMap[repo.Module], repo.Name)
			}
		}
	}

	if PigstyGPGCheck {
		if repo, ok := RepoMap["pigsty-pgsql"]; ok {
			repo.Meta["gpgcheck"] = "1"
			repo.Meta["gpgkey"] = "file:///etc/pki/rpm-gpg/RPM-GPG-KEY-pigsty"
		}
		if repo, ok := RepoMap["pigsty-infra"]; ok {
			repo.Meta["gpgcheck"] = "1"
			repo.Meta["gpgkey"] = "file:///etc/pki/rpm-gpg/RPM-GPG-KEY-pigsty"
		}
	}

	ModuleMap["pigsty"] = []string{"pigsty-infra", "pigsty-pgsql"}
	ModuleMap["pgdg"] = []string{"pgdg-common", "pgdg-el8fix", "pgdg-el9fix", "pgdg17", "pgdg16", "pgdg15", "pgdg14", "pgdg13"}
	ModuleMap["all"] = append(ModuleMap["pigsty"], append(ModuleMap["pgdg"], ModuleMap["node"]...)...)

	logrus.Debugf("load %d rpm repo, %d modules", len(RepoMap), len(ModuleMap))
	return nil
}

// LoadDebRepo loads DEB repository configurations
func LoadDebRepo(data []byte) error {
	if data == nil {
		data = embedDebRepo
	}
	var tmpRepos []Repository
	if err := yaml.Unmarshal(data, &tmpRepos); err != nil {
		return fmt.Errorf("failed to parse deb repo: %v", err)
	}

	// Filter available repos and build maps
	Repos = make([]*Repository, 0)
	RepoMap = make(map[string]*Repository)
	ModuleMap = make(map[string][]string)

	for i := range tmpRepos {
		repo := &tmpRepos[i]
		repo.Distro = config.DistroDEB
		repo.Meta = map[string]string{"trusted": "yes"}
		if repo.Available(config.OSCode, config.OSArch) {
			Repos = append(Repos, repo)
			RepoMap[repo.Name] = repo
			if repo.Module != "" {
				if _, exists := ModuleMap[repo.Module]; !exists {
					ModuleMap[repo.Module] = make([]string, 0)
				}
				ModuleMap[repo.Module] = append(ModuleMap[repo.Module], repo.Name)
			}
		}
	}

	if PigstyGPGCheck {
		if repo, ok := RepoMap["pigsty-pgsql"]; ok {
			delete(repo.Meta, "trusted")
			repo.Meta["signed-by"] = "/etc/apt/keyrings/pigsty.gpg"
		}
		if repo, ok := RepoMap["pigsty-infra"]; ok {
			delete(repo.Meta, "trusted")
			repo.Meta["signed-by"] = "/etc/apt/keyrings/pigsty.gpg"
		}
	}

	ModuleMap["pigsty"] = []string{"pigsty-infra", "pigsty-pgsql"}
	ModuleMap["pgdg"] = []string{"pgdg"}
	ModuleMap["all"] = append(ModuleMap["pigsty"], append(ModuleMap["pgdg"], ModuleMap["node"]...)...)

	logrus.Debugf("load %d deb repo, %d modules", len(RepoMap), len(ModuleMap))
	return nil
}
