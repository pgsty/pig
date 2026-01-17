package ext

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/sirupsen/logrus"
)

// ExtensionStatus prints the status of installed extensions
func ExtensionStatus(contrib bool) {
	if Catalog == nil {
		logrus.Debugf("catalog is not initialized, initializing from embedded data")
		var err error
		Catalog, err = NewExtensionCatalog()
		if err != nil {
			logrus.Errorf("failed to initialize catalog: %v", err)
			return
		}
	}

	PostgresInstallSummary()
	if Postgres == nil {
		logrus.Errorf("no PostgreSQL specified and not active PostgreSQL found")
		fmt.Printf("hint: use -v or -p to specify PostgreSQL installation\n\n")
		return
	}

	// Count extensions by repo
	var exts []*Extension
	var notFound []string
	repocount := map[string]int{"CONTRIB": 0, "PGDG": 0, "PIGSTY": 0}
	extMap := Catalog.ExtNameMap
	for _, ext := range Postgres.Extensions {
		if ext.Extension == nil {
			logrus.Debugf("un-cataloged extension found: %v", ext)
			continue
		}
		extInfo := extMap[ext.Name]
		if extInfo == nil {
			logrus.Infof("Extension: %s (not found in catalog)", ext.Name)
			notFound = append(notFound, ext.Name)
			continue
		}
		if extInfo.RepoName() != "" {
			if _, ok := repocount[extInfo.RepoName()]; !ok {
				repocount[extInfo.RepoName()] = 0
			}
			repocount[extInfo.RepoName()]++
		}
		if !contrib && extInfo.Repo == "CONTRIB" {
			continue
		}
		exts = append(exts, extInfo)
	}
	sort.Slice(exts, func(i, j int) bool {
		return exts[i].ID < exts[j].ID
	})

	if len(notFound) > 0 {
		logrus.Warnf("not found in catalog : %s", strings.Join(notFound, ", "))
	}

	printExtensionSummary(repocount, len(Postgres.Extensions))
	tabulateExtensions(exts)
}

func printExtensionSummary(repocount map[string]int, totalExtensions int) {
	nonContribCnt := repocount["PGDG"] + repocount["PIGSTY"]
	nonContribStr := fmt.Sprintf("PIGSTY %d, PGDG %d", repocount["PIGSTY"], repocount["PGDG"])
	for repo, count := range repocount {
		if repo != "CONTRIB" && repo != "PGDG" && repo != "PIGSTY" {
			nonContribCnt += count
			nonContribStr += fmt.Sprintf(", %s %d", repo, count)
		}
	}
	extSummary := fmt.Sprintf("Extension Stat  :  %d Installed (%s) + %d CONTRIB = %d Total\n",
		nonContribCnt, nonContribStr, repocount["CONTRIB"], totalExtensions)
	fmt.Println(extSummary)
}

func tabulateExtensions(exts []*Extension) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	if ShowPkg {
		fmt.Fprintln(w, "Pkg\tVersion\tCate\tFlags\tLicense\tRepo\tPackage\tDescription")
	} else {
		fmt.Fprintln(w, "Name\tVersion\tCate\tFlags\tLicense\tRepo\tPackage\tDescription")
	}
	fmt.Fprintln(w, "----\t-------\t----\t------\t-------\t------\t------------\t---------------------")
	count := 0
	for _, ext := range exts {
		if ShowPkg && !ext.Lead {
			continue
		}
		desc := ext.EnDesc
		if len(desc) > 64 {
			desc = desc[:64]
		}
		firstCol := ext.Name
		if ShowPkg {
			firstCol = ext.Pkg
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", firstCol, ext.Version, ext.Category, ext.GetFlag(), ext.License, ext.RepoName(), ext.PackageName(Postgres.MajorVersion), desc)
		count++
	}
	w.Flush()

	fmt.Printf("\n(%d Rows) (Flags: b = HasBin, d = HasDDL, s = HasLib, l = NeedLoad, t = Trusted, r = Relocatable, x = Unknown)\n\n", count)
}
