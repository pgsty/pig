package ext

import (
	"fmt"
	"os"
	"os/exec"
	"pig/internal/config"
	"pig/internal/output"
	"pig/internal/utils"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// ImportExtensionsResult returns a structured Result for the ext import command
func ImportExtensionsResult(pgVer int, names []string, importPath string) *output.Result {
	startTime := time.Now()

	if len(names) == 0 {
		return output.Fail(output.CodeExtensionInvalidArgs, "no extension names provided")
	}

	if pgVer == 0 {
		logrus.Debugf("no PostgreSQL version specified, set target version to the latest major version: %d", PostgresLatestMajorVersion)
		pgVer = PostgresLatestMajorVersion
	}

	if importPath == "" {
		importPath = "/www/pigsty"
	}

	if err := utils.Mkdir(importPath); err != nil {
		result := output.Fail(output.CodeExtensionImportFailed, fmt.Sprintf("failed to create import directory: %v", err))
		result.Data = &ImportResultData{
			PgVersion:  pgVer,
			OSCode:     config.OSCode,
			Arch:       config.OSArch,
			RepoDir:    importPath,
			Requested:  names,
			DurationMs: time.Since(startTime).Milliseconds(),
		}
		return result
	}

	// Check Catalog is initialized
	if Catalog == nil {
		result := output.Fail(output.CodeExtensionCatalogError, "extension catalog not initialized")
		result.Data = &ImportResultData{
			PgVersion:  pgVer,
			OSCode:     config.OSCode,
			Arch:       config.OSArch,
			RepoDir:    importPath,
			Requested:  names,
			DurationMs: time.Since(startTime).Milliseconds(),
		}
		return result
	}

	Catalog.LoadAliasMap(config.OSType)
	if err := validateTool(); err != nil {
		result := output.Fail(output.CodeExtensionImportFailed, err.Error())
		result.Data = &ImportResultData{
			PgVersion:  pgVer,
			OSCode:     config.OSCode,
			Arch:       config.OSArch,
			RepoDir:    importPath,
			Requested:  names,
			DurationMs: time.Since(startTime).Milliseconds(),
		}
		return result
	}

	var pkgNames []string
	var failed []string
	for _, name := range names {
		ext, ok := Catalog.ExtNameMap[name]
		if !ok {
			ext, ok = Catalog.ExtPkgMap[name]
		}

		if !ok {
			// try to find in AliasMap (if it is not a postgres extension)
			if pgPkg, matched, noPackage := resolveAliasPattern(pgVer, name); matched {
				if noPackage {
					failed = append(failed, name)
					continue
				}
				pkgNames = append(pkgNames, ProcessPkgName(pgPkg, pgVer)...)
				continue
			} else {
				logrus.Debugf("cannot find '%s' in extension name or alias", name)
				failed = append(failed, name)
				continue
			}
		}
		pkgName := ext.PackageName(pgVer)
		if pkgName == "" {
			logrus.Warnf("no package found for extension %s", ext.Name)
			failed = append(failed, name)
			continue
		}
		logrus.Debugf("translate extension %s to package name: %s", ext.Name, pkgName)
		pkgNames = append(pkgNames, ProcessPkgName(pkgName, pgVer)...)
	}

	if len(pkgNames) == 0 {
		result := output.Fail(output.CodeExtensionNoPackage, "no packages to be downloaded")
		result.Data = &ImportResultData{
			PgVersion:  pgVer,
			OSCode:     config.OSCode,
			Arch:       config.OSArch,
			RepoDir:    importPath,
			Requested:  names,
			Failed:     failed,
			DurationMs: time.Since(startTime).Milliseconds(),
		}
		return result
	}

	var downloaded []string
	var downloadErr error
	switch config.OSType {
	case config.DistroEL:
		downloaded, downloadErr = DownloadRPM(pkgNames, importPath)
	case config.DistroDEB:
		downloaded, downloadErr = DownloadDEB(pkgNames, importPath)
	default:
		downloadErr = fmt.Errorf("unsupported package manager: %s on %s %s", config.OSType, config.OSVendor, config.OSCode)
	}

	durationMs := time.Since(startTime).Milliseconds()

	if downloadErr != nil {
		result := output.Fail(output.CodeExtensionImportFailed, downloadErr.Error())
		result.Data = &ImportResultData{
			PgVersion:  pgVer,
			OSCode:     config.OSCode,
			Arch:       config.OSArch,
			RepoDir:    importPath,
			Requested:  names,
			Packages:   pkgNames,
			PkgCount:   len(pkgNames),
			Failed:     failed,
			DurationMs: durationMs,
		}
		return result
	}

	data := &ImportResultData{
		PgVersion:  pgVer,
		OSCode:     config.OSCode,
		Arch:       config.OSArch,
		RepoDir:    importPath,
		Requested:  names,
		Packages:   pkgNames,
		PkgCount:   len(pkgNames),
		Downloaded: downloaded,
		Failed:     failed,
		DurationMs: durationMs,
	}

	message := fmt.Sprintf("Imported %d packages to %s", len(pkgNames), importPath)
	return output.OK(message, data)
}

// DownloadRPM downloads RPM packages with repotrack
func DownloadRPM(pkgNames []string, workDir string) ([]string, error) {
	osarch := config.OSArch
	switch osarch {
	case "x86_64", "amd64":
		osarch = "x86_64,noarch"
	case "aarch64", "arm64":
		osarch = "aarch64,noarch"
	}

	downloadCmds := []string{"repotrack", "--arch", osarch}
	downloadCmds = append(downloadCmds, pkgNames...)
	logrus.Infof("download commands: %s", strings.Join(downloadCmds, " "))
	if err := sudoCommandInDir(downloadCmds, workDir); err != nil {
		return nil, fmt.Errorf("failed to download packages: %w", err)
	} else {
		logrus.Infof("downloaded %s successfully", strings.Join(pkgNames, " "))
		logrus.Infof("consider using: pig repo create  to update repo meta")
	}
	return pkgNames, nil
}

// DownloadDEB downloads DEB packages with apt-get and apt-cache
func DownloadDEB(pkgNames []string, workDir string) ([]string, error) {

	// Step 1: Get dependencies one by one
	dependencySet := make(map[string][]string)

	// Iterate over pkgNames and call apt-cache depends to get the dependency list
	for _, pkg := range pkgNames {
		// Call apt-cache depends
		logrus.Debugf("getting dependencies for %s with: %s", pkg, strings.Join([]string{"apt-cache", "depends", "--recurse", "--no-recommends", "--no-suggests", "--no-conflicts", "--no-breaks", "--no-replaces", "--no-enhances", pkg}, " "))
		out, err := utils.ShellOutput(
			"apt-cache", "depends", "--recurse", "--no-recommends", "--no-suggests", "--no-conflicts", "--no-breaks", "--no-replaces", "--no-enhances", pkg,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to run apt-cache depends for %s: %w", pkg, err)
		}

		depList := parseAptDependsOutput(out)
		logrus.Debugf("resolve dependencies for %s: %s", pkg, strings.Join(depList, ", "))
		dependencySet[pkg] = depList
	}

	// Merge requested packages + dependencies into a large list and remove duplicates.
	// NOTE: apt-get download does not include dependencies automatically.
	candidates := mergeDEBCandidates(pkgNames, dependencySet)
	if len(candidates) == 0 {
		logrus.Infof("no packages to download")
		return nil, nil
	}
	logrus.Infof("got %d packages & dependencies", len(candidates))

	downloadCmds := []string{"apt-get", "download"}
	downloadCmds = append(downloadCmds, candidates...)

	logrus.Infof("download commands: %s", strings.Join(downloadCmds, " "))
	if err := sudoCommandInDir(downloadCmds, workDir); err != nil {
		return nil, fmt.Errorf("failed to download packages: %w", err)
	} else {
		logrus.Infof("downloaded %s successfully", strings.Join(candidates, " "))
		logrus.Infof("consider using: pig repo create  to update repo meta")
	}
	return candidates, nil
}

func mergeDEBCandidates(pkgNames []string, dependencySet map[string][]string) []string {
	if len(pkgNames) == 0 {
		return nil
	}

	seen := make(map[string]struct{})
	candidates := make([]string, 0, len(pkgNames))
	add := func(name string) {
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		candidates = append(candidates, name)
	}

	// Keep deterministic order: requested packages first.
	for _, pkg := range pkgNames {
		add(pkg)
	}
	// Then their dependencies, per-request order.
	for _, pkg := range pkgNames {
		for _, dep := range dependencySet[pkg] {
			add(dep)
		}
	}
	return candidates
}

func parseAptDependsOutput(out string) []string {
	lines := strings.Split(out, "\n")
	deps := make([]string, 0, len(lines))
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "|") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "|"))
		}
		if strings.HasPrefix(line, "<") {
			continue
		}
		if idx := strings.Index(line, ":"); idx != -1 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])
			switch key {
			case "Depends", "PreDepends", "Recommends", "Suggests", "Conflicts", "Breaks", "Replaces", "Enhances":
				if value != "" {
					deps = append(deps, value)
				}
			}
		}
	}
	return deps
}

func runCommandInDir(args []string, workDir string) error {
	if len(args) == 0 {
		return fmt.Errorf("no command specified")
	}
	cmd := exec.Command(args[0], args[1:]...)
	if workDir != "" {
		cmd.Dir = workDir
	}
	cmd.Stdin = os.Stdin
	if config.IsStructuredOutput() {
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("command failed: %w", err)
	}
	return nil
}

func sudoCommandInDir(args []string, workDir string) error {
	if len(args) == 0 {
		return fmt.Errorf("no command specified")
	}

	if nosudoEnv := os.Getenv("PIG_NO_SUDO"); nosudoEnv == "1" || nosudoEnv == "true" {
		logrus.Debugf("PIG_NO_SUDO set, executing without sudo in %s: %v", workDir, args)
		return runCommandInDir(args, workDir)
	}

	finalArgs := args
	if isRoot := os.Geteuid() == 0 || config.CurrentUser == "root"; !isRoot {
		if _, err := exec.LookPath("sudo"); err == nil {
			finalArgs = append([]string{"sudo"}, args...)
		} else {
			logrus.Warnf("sudo command not available, trying to execute directly")
		}
	}
	return runCommandInDir(finalArgs, workDir)
}

// validateTool checks if the required tools are installed
func validateTool() error {
	switch config.OSType {
	case config.DistroEL:
		// check repotrack in path, if not, hint to install it
		if _, err := exec.LookPath("repotrack"); err != nil {
			logrus.Warnf("repotrack is required to download el rpm, install with: yum install -y yum-utils")
			return fmt.Errorf("repotrack not found, please install yum-utils: %w", err)
		} else {
			logrus.Debugf("repotrack (yum-utils) is installed")
			return nil
		}
	case config.DistroDEB:
		if _, err := exec.LookPath("apt-get"); err != nil {
			logrus.Warnf("apt-get is required to download deb package")
			return fmt.Errorf("apt-get not found: %w", err)
		} else {
			logrus.Debugf("apt-get is installed")
			return nil
		}
	default:
		return fmt.Errorf("unsupported package manager: %s on %s %s", config.OSType, config.OSVendor, config.OSCode)
	}
}
