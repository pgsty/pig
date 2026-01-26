package pgbackrest

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// InfoOptions holds options for the info command.
type InfoOptions struct {
	Output string // Output format: text, json (passed to pgbackrest)
	Set    string // Specific backup set to show
}

// Info displays backup repository information.
func Info(cfg *Config, opts *InfoOptions) error {
	effCfg, err := GetEffectiveConfig(cfg)
	if err != nil {
		return err
	}

	var args []string
	if opts.Output != "" {
		args = append(args, "--output="+opts.Output)
	}
	if opts.Set != "" {
		args = append(args, "--set="+opts.Set)
	}

	return RunPgBackRest(effCfg, "info", args, true)
}

// LsOptions holds options for the ls command.
type LsOptions struct {
	Type string // List type: backup, repo, stanza
}

// Ls lists resources in the backup repository.
func Ls(cfg *Config, opts *LsOptions) error {
	effCfg, err := GetEffectiveConfig(cfg)
	if err != nil {
		return err
	}

	switch opts.Type {
	case "", "backup":
		return RunPgBackRest(effCfg, "info", nil, true)
	case "repo":
		return listRepos(effCfg)
	case "stanza", "cluster", "cls":
		return listStanzas(effCfg)
	default:
		return fmt.Errorf("unknown list type: %s (use: backup, repo, stanza)", opts.Type)
	}
}

// listRepos parses config file and lists configured repositories.
func listRepos(cfg *Config) error {
	file, err := os.Open(cfg.ConfigPath)
	if err != nil {
		return fmt.Errorf("cannot open config file: %w", err)
	}
	defer file.Close()

	repos := make(map[string]map[string]string)
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if matches := repoConfigRegex.FindStringSubmatch(line); matches != nil {
			repoName, key, value := matches[1], matches[2], matches[3]
			if repos[repoName] == nil {
				repos[repoName] = make(map[string]string)
			}
			repos[repoName][key] = value
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading config: %w", err)
	}

	if len(repos) == 0 {
		fmt.Fprintln(os.Stderr, "No repositories configured")
		return nil
	}

	fmt.Printf("%-8s %-8s %s\n", "REPO", "TYPE", "PATH/ENDPOINT")
	fmt.Printf("%-8s %-8s %s\n", "----", "----", "-------------")

	// Print repos in order (repo1, repo2, ... up to repo10)
	for i := 1; i <= 10; i++ {
		repoName := fmt.Sprintf("repo%d", i)
		repo, ok := repos[repoName]
		if !ok {
			continue
		}

		repoType := repo["type"]
		if repoType == "" {
			repoType = "posix"
		}

		path := formatRepoPath(repoType, repo)
		fmt.Printf("%-8s %-8s %s\n", repoName, repoType, path)
	}

	return nil
}

// formatRepoPath formats the repository path based on type.
func formatRepoPath(repoType string, repo map[string]string) string {
	switch repoType {
	case "posix", "cifs":
		return repo["path"]
	case "s3":
		bucket := repo["s3-bucket"]
		if endpoint := repo["s3-endpoint"]; endpoint != "" {
			return fmt.Sprintf("s3://%s (endpoint: %s)", bucket, endpoint)
		}
		if region := repo["s3-region"]; region != "" {
			return fmt.Sprintf("s3://%s (%s)", bucket, region)
		}
		return fmt.Sprintf("s3://%s", bucket)
	case "azure":
		return fmt.Sprintf("azure://%s", repo["azure-container"])
	case "gcs":
		return fmt.Sprintf("gcs://%s", repo["gcs-bucket"])
	default:
		return repo["path"]
	}
}

// stanzaInfo holds parsed stanza information.
type stanzaInfo struct {
	Name   string
	PgPath string
	PgPort string
}

// listStanzas lists all stanzas in the config file.
func listStanzas(cfg *Config) error {
	file, err := os.Open(cfg.ConfigPath)
	if err != nil {
		return fmt.Errorf("cannot open config file: %w", err)
	}
	defer file.Close()

	var stanzas []stanzaInfo
	var current *stanzaInfo

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		if matches := sectionRegex.FindStringSubmatch(line); matches != nil {
			section := matches[1]
			if !strings.HasPrefix(section, "global") {
				if current != nil {
					stanzas = append(stanzas, *current)
				}
				current = &stanzaInfo{Name: section}
			} else {
				if current != nil {
					stanzas = append(stanzas, *current)
					current = nil
				}
			}
			continue
		}

		if current != nil {
			if matches := pgPathRegex.FindStringSubmatch(line); matches != nil {
				current.PgPath = strings.TrimSpace(matches[1])
			}
			if matches := pgPortRegex.FindStringSubmatch(line); matches != nil {
				current.PgPort = strings.TrimSpace(matches[1])
			}
		}
	}

	if current != nil {
		stanzas = append(stanzas, *current)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading config: %w", err)
	}

	if len(stanzas) == 0 {
		fmt.Fprintln(os.Stderr, "No stanzas configured")
		return nil
	}

	fmt.Printf("%-15s %-25s %s\n", "STANZA", "PG PATH", "PG PORT")
	fmt.Printf("%-15s %-25s %s\n", "------", "-------", "-------")
	for _, s := range stanzas {
		port := s.PgPort
		if port == "" {
			port = "5432"
		}
		fmt.Printf("%-15s %-25s %s\n", s.Name, s.PgPath, port)
	}

	return nil
}
