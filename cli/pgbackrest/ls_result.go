package pgbackrest

import (
	"bufio"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"pig/internal/output"
)

// PbLsResultData contains typed data for `pig pb ls` structured output.
type PbLsResultData struct {
	Type         string               `json:"type" yaml:"type"`
	Backups      *output.EmbeddedJSON `json:"backups,omitempty" yaml:"backups,omitempty"`
	Repositories []RepoListItem       `json:"repositories,omitempty" yaml:"repositories,omitempty"`
	Stanzas      []StanzaListItem     `json:"stanzas,omitempty" yaml:"stanzas,omitempty"`
}

// RepoListItem is a sanitized, typed view of one configured pgBackRest repo.
type RepoListItem struct {
	Name      string `json:"name" yaml:"name"`
	Key       int    `json:"key" yaml:"key"`
	Type      string `json:"type" yaml:"type"`
	URI       string `json:"uri,omitempty" yaml:"uri,omitempty"`
	Path      string `json:"path,omitempty" yaml:"path,omitempty"`
	Bucket    string `json:"bucket,omitempty" yaml:"bucket,omitempty"`
	Endpoint  string `json:"endpoint,omitempty" yaml:"endpoint,omitempty"`
	Region    string `json:"region,omitempty" yaml:"region,omitempty"`
	Container string `json:"container,omitempty" yaml:"container,omitempty"`
}

// StanzaListItem is a typed view of one configured pgBackRest stanza.
type StanzaListItem struct {
	Name   string `json:"name" yaml:"name"`
	PGPath string `json:"pg_path" yaml:"pg_path"`
	PGPort string `json:"pg_port" yaml:"pg_port"`
}

// LsResult returns structured output for `pig pb ls` without wrapping text tables.
func LsResult(cfg *Config, opts *LsOptions) *output.Result {
	listType, err := normalizeLsType(opts)
	if err != nil {
		return output.Fail(output.CodePbInvalidInfoParams, "invalid list type").WithDetail(err.Error())
	}

	switch listType {
	case "backup":
		return backupListResult(cfg)
	case "repo":
		repos, err := loadRepoList(cfg)
		if err != nil {
			return listErrorResult(err)
		}
		return output.OK("pgBackRest repositories listed", &PbLsResultData{
			Type:         "repo",
			Repositories: repos,
		})
	case "stanza":
		stanzas, err := loadStanzaList(cfg)
		if err != nil {
			return listErrorResult(err)
		}
		return output.OK("pgBackRest stanzas listed", &PbLsResultData{
			Type:    "stanza",
			Stanzas: stanzas,
		})
	default:
		return output.Fail(output.CodePbInvalidInfoParams, "invalid list type").
			WithDetail(fmt.Sprintf("unknown list type: %s (use: backup, repo, stanza)", listType))
	}
}

func backupListResult(cfg *Config) *output.Result {
	effCfg, err := GetEffectiveConfig(cfg)
	if err != nil {
		errMsg := err.Error()
		if containsAny(errMsg, "config file not found", "config file not accessible") {
			return output.Fail(output.CodePbConfigNotFound, "pgBackRest configuration not found").
				WithDetail(errMsg)
		}
		if containsAny(errMsg, "no stanza found", "cannot detect stanza") {
			return output.Fail(output.CodePbStanzaNotFound, "pgBackRest stanza not found").
				WithDetail(errMsg)
		}
		return output.Fail(output.CodePbInfoFailed, "Failed to get pgBackRest configuration").
			WithDetail(errMsg)
	}

	jsonOutput, err := RunPgBackRestOutput(effCfg, "info", []string{"--output=json", "--log-level-console=error"})
	if err != nil {
		return output.Fail(output.CodePbInfoFailed, "Failed to execute pgbackrest info").
			WithDetail(combineCommandError(jsonOutput, err))
	}

	var infos []PgBackRestInfo
	if err := json.Unmarshal([]byte(jsonOutput), &infos); err != nil {
		return output.Fail(output.CodePbInfoFailed, "Failed to parse pgbackrest info output").
			WithDetail(err.Error())
	}

	backups := output.NewEmbeddedJSON([]byte(jsonOutput))
	data := &PbLsResultData{Type: "backup", Backups: &backups}
	if len(infos) == 0 {
		return output.Fail(output.CodePbStanzaNotFound, "No stanza information found").
			WithDetail("pgbackrest info returned empty result").
			WithData(data)
	}
	if len(infos) == 1 && infos[0].Status.Code != 0 {
		code := output.CodePbInfoFailed
		if isStanzaNotFoundMessage(infos[0].Status.Message) {
			code = output.CodePbStanzaNotFound
		}
		return output.Fail(code, infos[0].Status.Message).WithData(data)
	}

	return output.OK("pgBackRest backups listed", data)
}

func listErrorResult(err error) *output.Result {
	if err == nil {
		return output.Fail(output.CodePbInfoFailed, "pgBackRest list failed")
	}
	errMsg := err.Error()
	if containsAny(errMsg, "config file not found", "config file not accessible", "cannot read config file") {
		return output.Fail(output.CodePbConfigNotFound, "pgBackRest configuration not found").
			WithDetail(errMsg)
	}
	return output.Fail(output.CodePbInfoFailed, "pgBackRest list failed").WithDetail(errMsg)
}

func normalizeLsType(opts *LsOptions) (string, error) {
	listType := ""
	if opts != nil {
		listType = strings.ToLower(strings.TrimSpace(opts.Type))
	}
	switch listType {
	case "", "backup":
		return "backup", nil
	case "repo":
		return "repo", nil
	case "stanza", "cluster", "cls":
		return "stanza", nil
	default:
		return "", fmt.Errorf("unknown list type: %s (use: backup, repo, stanza)", listType)
	}
}

func loadRepoList(cfg *Config) ([]RepoListItem, error) {
	effCfg, err := listConfigOnly(cfg)
	if err != nil {
		return nil, err
	}
	content, err := readConfigFile(effCfg.ConfigPath, effCfg.DbSU)
	if err != nil {
		return nil, fmt.Errorf("cannot read config file: %w", err)
	}
	return parseRepoConfigs(content)
}

func loadStanzaList(cfg *Config) ([]StanzaListItem, error) {
	effCfg, err := listConfigOnly(cfg)
	if err != nil {
		return nil, err
	}
	content, err := readConfigFile(effCfg.ConfigPath, effCfg.DbSU)
	if err != nil {
		return nil, fmt.Errorf("cannot read config file: %w", err)
	}
	return parseStanzaConfigs(content)
}

func listConfigOnly(cfg *Config) (*Config, error) {
	result := DefaultConfig()
	if cfg != nil {
		if cfg.ConfigPath != "" {
			result.ConfigPath = cfg.ConfigPath
		}
		if cfg.Stanza != "" {
			result.Stanza = cfg.Stanza
		}
		if cfg.Repo != "" {
			result.Repo = cfg.Repo
		}
		if cfg.DbSU != "" {
			result.DbSU = cfg.DbSU
		}
	}
	if err := checkConfigExists(result.ConfigPath, result.DbSU); err != nil {
		return nil, err
	}
	return result, nil
}

func parseRepoConfigs(content string) ([]RepoListItem, error) {
	repos := make(map[string]map[string]string)
	scanner := bufio.NewScanner(strings.NewReader(content))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if matches := repoConfigRegex.FindStringSubmatch(line); matches != nil {
			repoName, key, value := matches[1], matches[2], strings.TrimSpace(matches[3])
			if repos[repoName] == nil {
				repos[repoName] = make(map[string]string)
			}
			repos[repoName][key] = value
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading config: %w", err)
	}

	names := make([]string, 0, len(repos))
	for name := range repos {
		names = append(names, name)
	}
	sort.Slice(names, func(i, j int) bool {
		return repoSortKey(names[i]) < repoSortKey(names[j])
	})

	items := make([]RepoListItem, 0, len(names))
	for _, name := range names {
		items = append(items, repoListItemFromConfig(name, repos[name]))
	}
	return items, nil
}

func repoListItemFromConfig(name string, repo map[string]string) RepoListItem {
	repoType := repo["type"]
	if repoType == "" {
		repoType = "posix"
	}

	item := RepoListItem{
		Name: name,
		Key:  repoSortKey(name),
		Type: repoType,
	}

	switch repoType {
	case "posix", "cifs":
		item.Path = repo["path"]
		item.URI = item.Path
	case "s3":
		item.Bucket = repo["s3-bucket"]
		item.Endpoint = repo["s3-endpoint"]
		item.Region = repo["s3-region"]
		if item.Bucket != "" {
			item.URI = "s3://" + item.Bucket
		}
	case "azure":
		item.Container = repo["azure-container"]
		if item.Container != "" {
			item.URI = "azure://" + item.Container
		}
	case "gcs":
		item.Bucket = repo["gcs-bucket"]
		if item.Bucket != "" {
			item.URI = "gcs://" + item.Bucket
		}
	default:
		item.Path = repo["path"]
		item.URI = item.Path
	}

	return item
}

func repoSortKey(name string) int {
	n, err := strconv.Atoi(strings.TrimPrefix(name, "repo"))
	if err != nil {
		return 0
	}
	return n
}

func repoDisplayLocation(repo RepoListItem) string {
	switch repo.Type {
	case "posix", "cifs":
		return repo.Path
	case "s3":
		if repo.Endpoint != "" {
			return fmt.Sprintf("%s (endpoint: %s)", repo.URI, repo.Endpoint)
		}
		if repo.Region != "" {
			return fmt.Sprintf("%s (%s)", repo.URI, repo.Region)
		}
		return repo.URI
	case "azure", "gcs":
		return repo.URI
	default:
		return repo.Path
	}
}

func parseStanzaConfigs(content string) ([]StanzaListItem, error) {
	var stanzas []StanzaListItem
	var current *StanzaListItem

	appendCurrent := func() {
		if current == nil {
			return
		}
		if current.PGPort == "" {
			current.PGPort = "5432"
		}
		stanzas = append(stanzas, *current)
		current = nil
	}

	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}

		if matches := sectionRegex.FindStringSubmatch(line); matches != nil {
			section := matches[1]
			appendCurrent()
			if !strings.HasPrefix(section, "global") {
				current = &StanzaListItem{Name: section}
			}
			continue
		}

		if current != nil {
			if matches := pgPathRegex.FindStringSubmatch(line); matches != nil {
				current.PGPath = strings.TrimSpace(matches[1])
			}
			if matches := pgPortRegex.FindStringSubmatch(line); matches != nil {
				current.PGPort = strings.TrimSpace(matches[1])
			}
		}
	}
	appendCurrent()
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading config: %w", err)
	}
	return stanzas, nil
}
