package repo

import "strings"

const pgdgProxyURLPrefix = "http://beta.pigsty.cc"

var pgdgProxyPrefixReplacements = []struct {
	from string
	to   string
}{
	{"https://download.postgresql.org/pub/repos/yum/", pgdgProxyURLPrefix + "/yum/pgdg/"},
	{"http://download.postgresql.org/pub/repos/yum/", pgdgProxyURLPrefix + "/yum/pgdg/"},
	{"https://mirrors.aliyun.com/postgresql/repos/yum/", pgdgProxyURLPrefix + "/yum/pgdg/"},
	{"http://mirrors.aliyun.com/postgresql/repos/yum/", pgdgProxyURLPrefix + "/yum/pgdg/"},
	{"https://mirrors.xtom.de/postgresql/repos/yum/", pgdgProxyURLPrefix + "/yum/pgdg/"},
	{"http://mirrors.xtom.de/postgresql/repos/yum/", pgdgProxyURLPrefix + "/yum/pgdg/"},
	{"https://apt.postgresql.org/pub/repos/apt/", pgdgProxyURLPrefix + "/apt/pgdg/"},
	{"http://apt.postgresql.org/pub/repos/apt/", pgdgProxyURLPrefix + "/apt/pgdg/"},
	{"https://mirrors.aliyun.com/postgresql/repos/apt/", pgdgProxyURLPrefix + "/apt/pgdg/"},
	{"http://mirrors.aliyun.com/postgresql/repos/apt/", pgdgProxyURLPrefix + "/apt/pgdg/"},
	{"https://mirrors.xtom.de/postgresql/repos/apt/", pgdgProxyURLPrefix + "/apt/pgdg/"},
	{"http://mirrors.xtom.de/postgresql/repos/apt/", pgdgProxyURLPrefix + "/apt/pgdg/"},
}

func applyPGDGProxyRoute(m *Manager) {
	if m == nil {
		return
	}
	m.Region = "china"
	for _, repo := range m.Data {
		if repo == nil || !isPGDGRepo(repo) {
			continue
		}
		if repo.BaseURL == nil {
			repo.BaseURL = make(map[string]string)
		}
		sourceURL := repo.BaseURL["china"]
		if sourceURL == "" {
			sourceURL = repo.BaseURL["default"]
		}
		repo.BaseURL["china"] = rewritePGDGProxyURL(sourceURL)
	}
}

func isPGDGRepo(repo *Repository) bool {
	return strings.HasPrefix(strings.ToLower(repo.Name), "pgdg")
}

func rewritePGDGProxyURL(repoURL string) string {
	for _, replacement := range pgdgProxyPrefixReplacements {
		if strings.HasPrefix(repoURL, replacement.from) {
			return replacement.to + strings.TrimPrefix(repoURL, replacement.from)
		}
	}
	return repoURL
}
