package pgext

import (
	"fmt"
	"os"
	"pig/cli/pgsql"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/sirupsen/logrus"
)

// ExtensionStatus prints the status of installed extensions
func ExtensionStatus(pg *pgsql.PostgresInstallation, contrib bool) {
	// join pg.Extensions and pgext Extension Catalog and print the information
	var exts []*Extension
	var notFound []string
	repocount := map[string]int{"CONTRIB": 0, "PGDG": 0, "PIGSTY": 0}
	for _, ext := range pg.Extensions {
		extInfo := ExtNameMap[ext.Name]
		if extInfo == nil {
			logrus.Infof("Extension: %s (not found in catalog)", ext.Name)
			notFound = append(notFound, ext.Name)
			continue
		}
		if extInfo.RepoName() != "" {
			// if not in dict, add a zero count
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

	// print not found
	if len(notFound) > 0 {
		logrus.Warnf("not found in catalog : %s", strings.Join(notFound, ", "))
	}

	// sort by id
	sort.Slice(exts, func(i, j int) bool {
		return exts[i].ID < exts[j].ID
	})

	// summary
	nonContribCnt := repocount["PGDG"] + repocount["PIGSTY"]
	nonContribStr := fmt.Sprintf("PIGSTY %d, PGDG %d", repocount["PIGSTY"], repocount["PGDG"])
	for repo, count := range repocount {
		if repo != "CONTRIB" && repo != "PGDG" && repo != "PIGSTY" {
			nonContribCnt += count
			nonContribStr += fmt.Sprintf(", %s %d", repo, count)
		}
	}
	extSummary := fmt.Sprintf("Extension Stat :  %d Installed (%s) + %d CONTRIB = %d Total\n",
		nonContribCnt, nonContribStr, repocount["CONTRIB"], len(pg.Extensions))
	fmt.Println(extSummary)

	// tabulate installed extensions
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Name\tVersion\tCate\tFlags\tLicense\tRepo\tPackage\tDescription")
	fmt.Fprintln(w, "----\t-------\t----\t------\t-------\t------\t------------\t---------------------")
	for _, ext := range exts {
		desc := ext.EnDesc
		if len(desc) > 64 {
			desc = desc[:64]
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n", ext.Name, ext.Version, ext.Category, ext.GetFlag(), ext.License, ext.RepoName(), ext.PackageName(pg.MajorVersion), desc)
	}
	w.Flush()

	fmt.Printf("\n(%d Rows) (Flags: b = HasBin, d = HasDDL, s = HasSolib, l = NeedLoad, t = Trusted, r = Relocatable, x = Unknown)\n\n", len(exts))

}
