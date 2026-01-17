package ext

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"
)

func (e *Extension) PrintInfo() {
	tmpl, err := template.New("extension").Funcs(template.FuncMap{
		"join":     join,
		"truncate": truncate,
		"yesno":    yesno,
		"flagline": flagline,
	}).Parse(extensionInfoTmpl)
	if err != nil {
		fmt.Printf("Error parsing template: %v\n", err)
		return
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, e); err != nil {
		fmt.Printf("Error executing template: %v\n", err)
		return
	}

	fmt.Println(buf.String())
}

const extensionInfoTmpl = `
╭──────────────────────────────────────────────────────────────────────────────────────────────╮
│ {{ printf "%-94s" .Name }} │
├──────────────────────────────────────────────────────────────────────────────────────────────┤
│ {{ printf "%-94s" (truncate .EnDesc 94) }} │
├──────────────────────────────────────────────────────────────────────────────────────────────┤
│ Extension : {{ printf "%-82s" .Name }} │
│ Package   : {{ printf "%-82s" .Pkg }} │
│ Lead Ext  : {{ printf "%-82s" .LeadExt }} │
│ Category  : {{ printf "%-82s" .Category }} │
│ State     : {{ printf "%-82s" .State }} │
│ License   : {{ printf "%-82s" .License }} │
│ Website   : {{ printf "%-82s" (truncate .URL 82) }} │
│ Details   : {{ printf "%-82s" .SummaryURL }} │
├──────────────────────────────────────────────────────────────────────────────────────────────┤
│ {{ printf "%-94s" "Extension Properties" }} │
├──────────────────────────────────────────────────────────────────────────────────────────────┤
│ {{ printf "%-94s" (flagline .Contrib .Lead .HasBin .HasLib .NeedDDL .NeedLoad) }} │
│ CREATE  :  {{ yesno .NeedDDL }} |  {{ printf "%-76s" .CreateSQL }} │
│ DYLOAD  :  {{ yesno .NeedLoad }} |  {{ printf "%-76s" .SharedLib }} │
│ {{ printf "%-94s" .SuperUser }} │
│ Reloc   :  {{ if eq .Relocatable "t" }}Yes{{ else }}No {{ end }} |  {{ printf "%-76s" .SchemaStr }} │
{{- if .Requires }}
│ Depend  :  Yes |  {{ printf "%-76s" (truncate (join .Requires ", ") 76) }} │
{{- else }}
│ Depend  :  No  |  {{ printf "%-76s" "" }} │
{{- end }}
{{- if .DependsOn }}
├──────────────────────────────────────────────────────────────────────────────────────────────┤
│ {{ printf "%-94s" "Required By" }} │
├──────────────────────────────────────────────────────────────────────────────────────────────┤
{{- range .DependsOn }}
│ - {{ printf "%-92s" . }} │
{{- end }}
{{- end }}
├──────────────────────────────────────────────────────────────────────────────────────────────┤
│ {{ printf "%-94s" "Package Summary" }} │
├──────────────────────────────────────────────────────────────────────────────────────────────┤
│ Repository     |  {{ printf "%-76s" .Repo }} │
│ Version        |  {{ printf "%-76s" .Version }} │
│ Availability   |  {{ printf "%-76s" (join .PgVer ", ") }} │
{{- if .RpmRepo }}
├──────────────────────────────────────────────────────────────────────────────────────────────┤
│ {{ printf "%-94s" "RPM Package" }} │
├──────────────────────────────────────────────────────────────────────────────────────────────┤
│ Repository     |  {{ printf "%-76s" .RpmRepo }} │
│ Package        |  {{ printf "%-76s" .RpmPkg }} │
│ Version        |  {{ printf "%-76s" .RpmVer }} │
│ Availability   |  {{ printf "%-76s" (join .RpmPg ", ") }} │
{{- if .RpmDeps }}
│ Dependencies   |  {{ printf "%-76s" (truncate (join .RpmDeps ", ") 76) }} │
{{- end }}
{{- end }}
{{- if .DebRepo }}
├──────────────────────────────────────────────────────────────────────────────────────────────┤
│ {{ printf "%-94s" "DEB Package" }} │
├──────────────────────────────────────────────────────────────────────────────────────────────┤
│ Repository     |  {{ printf "%-76s" .DebRepo }} │
│ Package        |  {{ printf "%-76s" .DebPkg }} │
│ Version        |  {{ printf "%-76s" .DebVer }} │
│ Availability   |  {{ printf "%-76s" (join .DebPg ", ") }} │
{{- if .DebDeps }}
│ Dependencies   |  {{ printf "%-76s" (truncate (join .DebDeps ", ") 76) }} │
{{- end }}
{{- end }}
{{- if .Source }}
├──────────────────────────────────────────────────────────────────────────────────────────────┤
│ Source: {{ printf "%-87s" .Source }} │
{{- end }}
{{- if .Comment }}
├──────────────────────────────────────────────────────────────────────────────────────────────┤
│ {{ printf "%-94s" "Comments" }} │
├──────────────────────────────────────────────────────────────────────────────────────────────┤
│ {{ printf "%-94s" (truncate .Comment 94) }} │
{{- end }}
{{- if .SeeAlso }}
├──────────────────────────────────────────────────────────────────────────────────────────────┤
│ See Also: {{ printf "%-85s" (truncate (join .SeeAlso ", ") 85) }} │
{{- end }}
╰──────────────────────────────────────────────────────────────────────────────────────────────╯
`

func join(strs []string, sep string) string {
	return strings.Join(strs, sep)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

func yesno(b bool) string {
	if b {
		return "Yes"
	}
	return "No "
}

func flagline(contrib, lead, hasBin, hasLib, needDDL, needLoad bool) string {
	return fmt.Sprintf("Contrib: %s | Lead: %s | HasBin: %s | HasLib: %s | DDL: %s | Load: %s",
		yesno(contrib), yesno(lead), yesno(hasBin), yesno(hasLib), yesno(needDDL), yesno(needLoad))
}
