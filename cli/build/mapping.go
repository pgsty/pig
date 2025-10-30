package build

// SpecialSourceMapping defines special case mappings for non-extension packages
var SpecialSourceMapping = map[string][]string{
	"scws":       {"scws-1.2.3.tar.bz2"},
	"openhalodb": {"openhalodb-1.0.tar.gz"},
	"oriolepg":   {"oriolepg-17.11.tar.gz"},

	// Multi-version PostgreSQL source packages
	"libfepgutils": {
		"postgresql-14.19.tar.gz",
		"postgresql-15.14.tar.gz",
		"postgresql-16.10.tar.gz",
		"postgresql-17.6.tar.gz",
		"postgresql-18.0.tar.gz",
	},

	// Additional mappings can be added here
	"postgresql": {
		"postgresql-14.19.tar.gz",
		"postgresql-15.14.tar.gz",
		"postgresql-16.10.tar.gz",
		"postgresql-17.6.tar.gz",
		"postgresql-18.0.tar.gz",
	},
}
