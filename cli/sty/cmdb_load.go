package sty

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"pig/internal/cmdb"
	"pig/internal/config"
	"pig/internal/output"
	"pig/inventory"
)

const maxCMDBStaticSourceBytes = 16 << 20

// CMDBLoadOptions keeps the positional source, invocation override, and
// normally resolved source distinct so revision ambiguity can be rejected
// before any database access.
type CMDBLoadOptions struct {
	StaticFile       string
	InvocationSource string
	ResolvedStatic   string
	Revision         string
	WorkingDirectory string
	ToolVersion      string
	Prune            bool
	Yes              bool
}

type CMDBLoadData struct {
	Kind                 string                     `json:"kind" yaml:"kind"`
	SourceKind           cmdb.RevisionSourceKind    `json:"source_kind" yaml:"source_kind"`
	ExpectedHead         *cmdb.RevisionToken        `json:"expected_head,omitempty" yaml:"expected_head,omitempty"`
	Target               *cmdb.RevisionToken        `json:"target,omitempty" yaml:"target,omitempty"`
	CandidateSHA256      string                     `json:"candidate_sha256" yaml:"candidate_sha256"`
	RevisionCount        int64                      `json:"revision_count" yaml:"revision_count"`
	Changes              []inventory.SnapshotChange `json:"changes,omitempty" yaml:"changes,omitempty"`
	Removals             []inventory.SnapshotChange `json:"removals,omitempty" yaml:"removals,omitempty"`
	NoOp                 bool                       `json:"no_op" yaml:"no_op"`
	RequiresPrune        bool                       `json:"requires_prune" yaml:"requires_prune"`
	RequiresConfirmation bool                       `json:"requires_confirmation" yaml:"requires_confirmation"`
	Applied              bool                       `json:"applied" yaml:"applied"`
	Changed              bool                       `json:"changed" yaml:"changed"`
	Revision             *cmdb.RevisionToken        `json:"revision,omitempty" yaml:"revision,omitempty"`
}

func (d *CMDBLoadData) Text() string {
	if d == nil {
		return ""
	}
	lines := []string{
		"  source: " + string(d.SourceKind),
		"  expected head: " + formatCMDBToken(d.ExpectedHead),
		"  candidate sha256: " + d.CandidateSHA256,
		"  revisions: " + fmt.Sprint(d.RevisionCount),
	}
	if d.Target != nil {
		lines = append(lines, "  rollback target: "+d.Target.String())
	}
	if d.Revision != nil {
		lines = append(lines, "  committed revision: "+d.Revision.String())
	}
	rows := make([][]string, 0, len(d.Changes))
	for _, change := range d.Changes {
		rows = append(rows, []string{string(change.Operation), change.Path.String()})
	}
	if len(rows) > 0 {
		lines = append(lines, strings.TrimSuffix(output.RenderTable([]string{"OPERATION", "PATH"}, rows), "\n"))
	}
	return strings.Join(lines, "\n")
}

// CMDBLoadOperation holds the exact, Store-bound native plan and private
// static source revision across an interactive confirmation. Only Data is
// exposed, and it contains no Inventory body or values.
type CMDBLoadOperation struct {
	data         CMDBLoadData
	store        *cmdb.Store
	plan         cmdb.LoadPlan
	staticPath   string
	staticDigest [sha256.Size]byte
}

func (o *CMDBLoadOperation) Data() *CMDBLoadData {
	if o == nil {
		return nil
	}
	copy := o.data
	copy.Changes = cloneSnapshotChanges(o.data.Changes)
	copy.Removals = cloneSnapshotChanges(o.data.Removals)
	return &copy
}

func PlanCMDBLoad(ctx context.Context, store *cmdb.Store, options CMDBLoadOptions) (*CMDBLoadOperation, *output.Result) {
	revisionText := options.Revision
	if revisionText != "" && (options.StaticFile != "" || options.InvocationSource != "") {
		return nil, output.Fail(output.CodeStyCMDBInvalidArgs, "CMDB revision load is mutually exclusive with a static file and invocation Inventory").
			WithData(&CMDBFailureData{Kind: CMDBFailureKind, CMDBCode: "CMDB_PARAMETER_INVALID"})
	}
	toolVersion := strings.TrimSpace(options.ToolVersion)
	if toolVersion == "" {
		toolVersion = config.PigVersion
	}

	if revisionText != "" {
		token, err := cmdb.ParseRevisionToken(revisionText)
		if err != nil {
			return nil, cmdbFailureResult("CMDB revision token is invalid", err)
		}
		plan, err := store.PlanRollback(ctx, token, toolVersion)
		if err != nil {
			return nil, cmdbFailureResult("CMDB rollback plan could not be created", err)
		}
		return newCMDBLoadOperation(store, plan, "", [sha256.Size]byte{}), nil
	}

	staticName := options.StaticFile
	if staticName == "" {
		staticName = options.ResolvedStatic
	}
	if staticName == "" {
		return nil, output.Fail(output.CodeStyCMDBInvalidArgs, "CMDB load requires one static Inventory source").
			WithData(&CMDBFailureData{Kind: CMDBFailureKind, CMDBCode: "CMDB_SOURCE_INVALID"}).
			WithNextActions(output.NextAction{
				Command: "pig sty cmdb load <static-file>", Reason: "name one static Inventory file explicitly", Required: true,
			})
	}
	load, validationFailure := prepareCMDBStaticLoad(staticName, options.WorkingDirectory, toolVersion)
	if validationFailure != nil {
		return nil, validationFailure
	}
	plan, err := store.PlanStaticLoad(ctx, load)
	if err != nil {
		return nil, cmdbFailureResult("CMDB static load plan could not be created", err)
	}
	return newCMDBLoadOperation(store, plan, load.SourceLocator, load.SourceBytesSHA256), nil
}

func ApplyCMDBLoad(ctx context.Context, operation *CMDBLoadOperation, options CMDBLoadOptions) *output.Result {
	if operation == nil || operation.store == nil {
		return output.Fail(output.CodeStyCMDBInvalidArgs, "CMDB load requires an exact prepared plan").
			WithData(&CMDBFailureData{Kind: CMDBFailureKind, CMDBCode: "CMDB_LOAD_PLAN_INVALID"})
	}
	data := operation.Data()
	if operation.plan.RequiresPrune && !options.Prune {
		return output.Fail(output.CodeStyCMDBPruneRequired, "CMDB load removals require explicit prune scope").
			WithData(data).
			WithNextActions(output.NextAction{
				Command: "pig sty cmdb load ... --prune", Reason: "select the displayed declaration removals", Required: true,
			})
	}
	if operation.plan.RequiresConfirmation && !options.Yes {
		command := "pig sty cmdb load ... --yes"
		if operation.plan.RequiresPrune {
			command = "pig sty cmdb load ... --prune --yes"
		}
		return output.Fail(output.CodeStyCMDBConfirmationRequired, "CMDB load requires explicit confirmation").
			WithData(data).
			WithNextActions(output.NextAction{
				Command: command, Reason: "apply the exact displayed CMDB plan", Required: true,
			})
	}
	if operation.staticPath != "" {
		_, current, issue := readCMDBStaticSource(operation.staticPath, "")
		if issue != nil || sha256.Sum256(current) != operation.staticDigest {
			return output.Fail(output.CodeStyCMDBSourceChanged, "CMDB static source changed after planning").
				WithData(&CMDBFailureData{Kind: CMDBFailureKind, CMDBCode: "CMDB_SOURCE_CHANGED"})
		}
	}
	result, err := operation.store.ApplyLoad(ctx, operation.plan, cmdb.LoadConsent{Prune: options.Prune, Confirmed: options.Yes})
	if err != nil {
		return cmdbFailureResult("CMDB load could not be applied", err)
	}
	data.Applied = true
	data.Changed = result.Changed
	data.RevisionCount = result.RevisionCount
	token := result.Token
	data.Revision = &token
	message := "CMDB load applied"
	if !result.Changed {
		message = "CMDB load is already current"
	}
	return output.OK(message, data)
}

func CMDBLoad(ctx context.Context, store *cmdb.Store, options CMDBLoadOptions) *output.Result {
	operation, failure := PlanCMDBLoad(ctx, store, options)
	if failure != nil {
		return failure
	}
	return ApplyCMDBLoad(ctx, operation, options)
}

func newCMDBLoadOperation(store *cmdb.Store, plan cmdb.LoadPlan, path string, digest [sha256.Size]byte) *CMDBLoadOperation {
	data := CMDBLoadData{
		Kind: plan.Kind, SourceKind: plan.SourceKind, ExpectedHead: cloneRevisionToken(plan.ExpectedHead),
		Target: cloneRevisionToken(plan.Target), CandidateSHA256: plan.CandidateSHA256,
		RevisionCount: plan.RevisionCount, Changes: cloneSnapshotChanges(plan.Changes),
		Removals: cloneSnapshotChanges(plan.Removals), NoOp: plan.NoOp,
		RequiresPrune: plan.RequiresPrune, RequiresConfirmation: plan.RequiresConfirmation,
	}
	return &CMDBLoadOperation{data: data, store: store, plan: plan, staticPath: path, staticDigest: digest}
}

func prepareCMDBStaticLoad(name, workingDirectory, toolVersion string) (cmdb.StaticLoad, *output.Result) {
	path, source, issue := readCMDBStaticSource(name, workingDirectory)
	if issue != nil {
		return cmdb.StaticLoad{}, staticSourceIssueResult(path, issue)
	}
	document, err := inventory.Parse(path, source)
	if err != nil {
		data := &CMDBValidationData{Kind: CMDBValidationKind, Path: path, Diagnostics: []inventory.Diagnostic{parseDiagnostic(err)}}
		return cmdb.StaticLoad{}, output.Fail(output.CodeStyCMDBConfigInvalid, "CMDB static Inventory YAML is invalid").WithData(data)
	}
	diagnostics := document.Validate(
		inventory.YAMLRules, inventory.AnsibleRules, inventory.PigstyRules, inventory.CompleteInventoryRules,
	)
	if inventory.HasErrors(diagnostics) {
		data := &CMDBValidationData{Kind: CMDBValidationKind, Path: path, Diagnostics: diagnostics}
		return cmdb.StaticLoad{}, output.Fail(output.CodeStyCMDBConfigInvalid, "CMDB static Inventory validation failed").WithData(data)
	}
	snapshot, err := inventory.SnapshotFromDocument(document)
	if err != nil {
		data := &CMDBValidationData{Kind: CMDBValidationKind, Path: path}
		var snapshotError *inventory.SnapshotError
		if errors.As(err, &snapshotError) && snapshotError != nil {
			copy := *snapshotError
			copy.Path = append(inventory.Path(nil), snapshotError.Path...)
			data.Representability = &copy
		}
		return cmdb.StaticLoad{}, output.Fail(output.CodeStyCMDBSnapshotUnrepresentable, "CMDB static Inventory is not representable").WithData(data)
	}
	return cmdb.StaticLoad{
		Snapshot: snapshot, SourceLocator: path, SourceBytesSHA256: sha256.Sum256(source), ToolVersion: toolVersion,
	}, nil
}

type cmdbStaticSourceIssue string

const (
	cmdbStaticMissing    cmdbStaticSourceIssue = "missing"
	cmdbStaticPermission cmdbStaticSourceIssue = "permission"
	cmdbStaticInvalid    cmdbStaticSourceIssue = "invalid"
	cmdbStaticChanged    cmdbStaticSourceIssue = "changed"
	cmdbStaticTooLarge   cmdbStaticSourceIssue = "too-large"
)

func readCMDBStaticSource(name, workingDirectory string) (string, []byte, *cmdbStaticSourceIssue) {
	if strings.TrimSpace(name) == "" || !utf8.ValidString(name) || strings.ContainsRune(name, 0) {
		issue := cmdbStaticInvalid
		return "", nil, &issue
	}
	path := name
	if !filepath.IsAbs(path) {
		base := workingDirectory
		if base == "" {
			var err error
			base, err = os.Getwd()
			if err != nil {
				issue := cmdbStaticInvalid
				return "", nil, &issue
			}
		}
		path = filepath.Join(base, path)
	}
	absolute, err := filepath.Abs(path)
	if err != nil || filepath.Clean(absolute) != absolute || !utf8.ValidString(absolute) {
		issue := cmdbStaticInvalid
		return "", nil, &issue
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		issue := classifyStaticPathError(err)
		return absolute, nil, &issue
	}
	if resolved != absolute {
		issue := cmdbStaticInvalid
		return absolute, nil, &issue
	}
	before, err := os.Lstat(absolute)
	if err != nil {
		issue := classifyStaticPathError(err)
		return absolute, nil, &issue
	}
	if !before.Mode().IsRegular() || before.Mode().Perm()&0o111 != 0 || before.Size() < 0 {
		issue := cmdbStaticInvalid
		return absolute, nil, &issue
	}
	if before.Size() > maxCMDBStaticSourceBytes {
		issue := cmdbStaticTooLarge
		return absolute, nil, &issue
	}
	file, err := os.Open(absolute)
	if err != nil {
		issue := classifyStaticPathError(err)
		return absolute, nil, &issue
	}
	defer file.Close()
	after, statErr := file.Stat()
	pathAfter, pathErr := os.Lstat(absolute)
	if statErr != nil || pathErr != nil || !sameCMDBStaticRevision(before, after) || !sameCMDBStaticRevision(after, pathAfter) {
		issue := cmdbStaticChanged
		return absolute, nil, &issue
	}
	source, err := io.ReadAll(io.LimitReader(file, maxCMDBStaticSourceBytes+1))
	if err != nil {
		issue := cmdbStaticInvalid
		return absolute, nil, &issue
	}
	if len(source) > maxCMDBStaticSourceBytes {
		issue := cmdbStaticChanged
		return absolute, nil, &issue
	}
	readAfter, statErr := file.Stat()
	pathFinal, pathErr := os.Lstat(absolute)
	if statErr != nil || pathErr != nil || !sameCMDBStaticRevision(after, readAfter) || !sameCMDBStaticRevision(readAfter, pathFinal) {
		issue := cmdbStaticChanged
		return absolute, nil, &issue
	}
	return absolute, source, nil
}

func sameCMDBStaticRevision(left, right os.FileInfo) bool {
	return left != nil && right != nil && os.SameFile(left, right) && left.Mode() == right.Mode() &&
		left.Size() == right.Size() && left.ModTime().Equal(right.ModTime())
}

func classifyStaticPathError(err error) cmdbStaticSourceIssue {
	switch {
	case errors.Is(err, os.ErrNotExist):
		return cmdbStaticMissing
	case errors.Is(err, os.ErrPermission):
		return cmdbStaticPermission
	default:
		return cmdbStaticInvalid
	}
}

func staticSourceIssueResult(path string, issue *cmdbStaticSourceIssue) *output.Result {
	data := &CMDBValidationData{Kind: CMDBValidationKind, Path: path}
	if issue == nil {
		return output.Fail(output.CodeStyCMDBOperationFailed, "CMDB static Inventory could not be read").WithData(data)
	}
	switch *issue {
	case cmdbStaticMissing:
		return output.Fail(output.CodeStyCMDBResourceMissing, "CMDB static Inventory is unavailable").WithData(data)
	case cmdbStaticPermission:
		return output.Fail(output.CodeStyCMDBPermissionDenied, "CMDB static Inventory permission was denied").WithData(data)
	case cmdbStaticChanged:
		return output.Fail(output.CodeStyCMDBSourceChanged, "CMDB static Inventory changed during inspection").WithData(data)
	case cmdbStaticTooLarge:
		data.Representability = &inventory.SnapshotError{
			Code: "CMDB_SNAPSHOT_UNREPRESENTABLE", Reason: inventory.SnapshotReasonSourceTooLarge,
		}
		return output.Fail(output.CodeStyCMDBSnapshotUnrepresentable, "CMDB static Inventory is not representable").WithData(data)
	default:
		return output.Fail(output.CodeStyCMDBInvalidArgs, "CMDB source must be one bounded, regular, non-executable, symlink-free static Inventory").WithData(data)
	}
}

func cloneRevisionToken(token *cmdb.RevisionToken) *cmdb.RevisionToken {
	if token == nil {
		return nil
	}
	copy := *token
	return &copy
}

func cloneSnapshotChanges(changes []inventory.SnapshotChange) []inventory.SnapshotChange {
	if len(changes) == 0 {
		return nil
	}
	copy := make([]inventory.SnapshotChange, len(changes))
	for index := range changes {
		copy[index] = inventory.SnapshotChange{
			Operation: changes[index].Operation,
			Path:      append(inventory.Path(nil), changes[index].Path...),
		}
	}
	return copy
}
