package inventory

import (
	"fmt"
	"net/netip"
	"regexp"
	"strings"
)

var (
	pigstyClusterName = regexp.MustCompile(`^[A-Za-z0-9-]+$`)
	pigstyDate        = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

	userFields = map[string]string{
		"name": "string", "password": "string", "login": "bool", "superuser": "bool",
		"createdb": "bool", "createrole": "bool", "inherit": "bool", "replication": "bool",
		"bypassrls": "bool", "pgbouncer": "bool", "connlimit": "integer", "expire_in": "integer",
		"expire_at": "date", "comment": "string", "roles": "string[]", "parameters": "mapping",
		"pool_mode": "enum:transaction,session,statement", "pool_connlimit": "integer",
	}
	databaseFields = map[string]string{
		"name": "string", "baseline": "string", "pgbouncer": "bool", "schemas": "string[]",
		"extensions": "sequence", "comment": "string", "owner": "string", "template": "string",
		"encoding": "string", "locale": "string", "lc_collate": "string", "lc_ctype": "string",
		"tablespace": "string", "allowconn": "bool", "revokeconn": "bool",
		"register_datasource": "bool", "connlimit": "integer", "pool_auth_user": "string",
		"pool_mode": "enum:transaction,session,statement", "pool_size": "integer",
		"pool_size_min": "integer", "pool_reserve": "integer", "pool_connlimit": "integer",
		"parameters": "mapping",
	}
	serviceFields = map[string]string{
		"name": "string", "port": "integer", "ip": "string", "selector": "string",
		"backup": "string", "dest": "enum:default,postgres,pgbouncer", "check": "string",
		"maxconn": "integer", "options": "string", "balance": "enum:roundrobin,leastconn",
	}
	hbaFields = map[string]struct{}{
		"title": {}, "rules": {}, "addr": {}, "auth": {}, "user": {}, "db": {}, "order": {},
	}
	hbaAddressAliases = map[string]struct{}{
		"world": {}, "intra": {}, "infra": {}, "admin": {}, "local": {}, "localhost": {}, "cluster": {},
	}
	hbaAuthAliases = map[string]struct{}{
		"deny": {}, "trust": {}, "pwd": {}, "sha": {}, "scram-sha-256": {}, "md5": {},
		"ssl": {}, "ssl-md5": {}, "ssl-sha": {}, "os": {}, "ident": {}, "peer": {}, "cert": {},
	}
	reservedServicePorts = map[uint64]struct{}{
		5432: {}, 6432: {}, 5433: {}, 5434: {}, 5436: {}, 5438: {},
		9100: {}, 9101: {}, 9630: {}, 9631: {}, 9080: {},
	}
)

type pigstyValidator struct {
	doc           *Document
	diagnostics   []Diagnostic
	seenSeq       map[string]map[uint64]string
	memberships   map[string]map[string]string
	modulePresent map[string]bool
}

type moduleSpec struct {
	module  string
	seq     string
	cluster string
}

var pigstyModuleSpecs = []moduleSpec{
	{module: "infra", seq: "infra_seq"},
	{module: "etcd", seq: "etcd_seq", cluster: "etcd_cluster"},
	{module: "minio", seq: "minio_seq", cluster: "minio_cluster"},
	{module: "redis", seq: "redis_node", cluster: "redis_cluster"},
}

func validatePigsty(doc *Document) []Diagnostic {
	if doc.inventory == nil || doc.inventory.All == nil {
		return nil
	}
	v := &pigstyValidator{
		doc: doc, seenSeq: make(map[string]map[uint64]string),
		memberships: make(map[string]map[string]string), modulePresent: make(map[string]bool),
	}
	if allValue, exists := doc.Lookup(Keys("all")); exists && v.unverifiable(allValue, "all group") {
		return v.diagnostics
	}
	v.validateGlobal(doc.inventory.All)
	v.visitGroup(doc.inventory.All)
	return v.diagnostics
}

func validatePigstyCompleteness(doc *Document) []Diagnostic {
	if doc == nil || doc.inventory == nil || doc.inventory.All == nil {
		return nil
	}
	v := &pigstyValidator{
		doc: doc, seenSeq: make(map[string]map[uint64]string),
		memberships: make(map[string]map[string]string), modulePresent: make(map[string]bool),
	}
	all := doc.inventory.All
	if !isOpaqueValue(all.Vars) {
		vars := mappingValues(all.Vars)
		if _, exists := vars["version"]; !exists {
			v.add("PIG_VERSION_MISSING", SeverityWarning, AllVars("version"), nil, all.Range, "all.vars.version is not declared")
		}
		if _, exists := vars["region"]; !exists {
			v.add("PIG_REGION_MISSING", SeverityWarning, AllVars("region"), nil, all.Range, "all.vars.region is not declared")
		}
	}
	v.collectModulePresence(all)
	v.validateRequiredModules(all)
	return v.diagnostics
}

func (v *pigstyValidator) add(code string, severity Severity, path Path, source *Value, fallback SourceRange, message string) {
	rangeValue := fallback
	if source != nil {
		path = source.Path
		rangeValue = source.Range
	}
	v.diagnostics = append(v.diagnostics, Diagnostic{
		Code: code, Severity: severity, Path: clonePath(path), Range: rangeValue, Message: message,
	})
}

func (v *pigstyValidator) unverifiable(value *Value, subject string) bool {
	if !isOpaqueValue(value) {
		return false
	}
	v.add("UNVERIFIABLE", SeverityInfo, value.Path, value, SourceRange{}, "plaintext value is unavailable for "+subject+" validation")
	return true
}

func isOpaqueValue(value *Value) bool {
	return value != nil && (value.Alias != "" || isCustomYAMLTag(value.Tag))
}

func (v *pigstyValidator) validateGlobal(all *Group) {
	if all == nil {
		return
	}
	if v.unverifiable(all.Vars, "global declaration") {
		return
	}
	vars := mappingValues(all.Vars)
	admin, hasAdmin := vars["admin_ip"]
	if !hasAdmin {
		v.add("PIG_ADMIN_IP_REQUIRED", SeverityError, AllVars("admin_ip"), nil, all.Range, "all.vars.admin_ip is required")
	} else if !v.unverifiable(admin, "admin_ip") {
		address, ok := stringValue(admin)
		if !ok || !validIPv4(address) {
			v.add("PIG_ADMIN_IP", SeverityError, admin.Path, admin, SourceRange{}, "admin_ip must be a valid IPv4 address")
		}
	}
	if region, exists := vars["region"]; exists && !v.unverifiable(region, "region") {
		name, ok := stringValue(region)
		if !ok || name != "default" && name != "china" && name != "europe" {
			v.add("PIG_REGION", SeverityWarning, region.Path, region, SourceRange{}, "region should be default, china, or europe")
		}
	}
	v.validateDeclaredObjects(vars)
	v.validateVIP(vars, true)
}

func (v *pigstyValidator) collectModulePresence(group *Group) {
	if group == nil {
		return
	}
	if groupValue, exists := v.doc.Lookup(group.Path); exists && isOpaqueValue(groupValue) {
		for _, spec := range pigstyModuleSpecs {
			if group.Name == spec.module {
				v.modulePresent[spec.module] = true
			}
		}
		return
	}
	if group.Name != "all" {
		vars := mappingValues(group.Vars)
		if _, exists := vars["pg_cluster"]; exists {
			v.modulePresent["pgsql"] = true
		}
		for _, spec := range pigstyModuleSpecs {
			if group.Name == spec.module {
				v.modulePresent[spec.module] = true
			}
			if _, exists := vars[spec.cluster]; spec.cluster != "" && exists {
				v.modulePresent[spec.module] = true
			}
		}
		for _, host := range group.Hosts {
			if host == nil {
				continue
			}
			hostVars := mappingValues(&host.Vars)
			if _, cluster := hostVars["pg_cluster"]; cluster {
				v.modulePresent["pgsql"] = true
			}
			if _, seq := hostVars["pg_seq"]; seq {
				v.modulePresent["pgsql"] = true
			}
			if _, role := hostVars["pg_role"]; role {
				v.modulePresent["pgsql"] = true
			}
			for _, spec := range pigstyModuleSpecs {
				if _, seq := hostVars[spec.seq]; seq {
					v.modulePresent[spec.module] = true
				}
				if _, cluster := hostVars[spec.cluster]; spec.cluster != "" && cluster {
					v.modulePresent[spec.module] = true
				}
			}
		}
	}
	for _, child := range group.Children {
		v.collectModulePresence(child)
	}
}

func (v *pigstyValidator) validateRequiredModules(all *Group) {
	if !v.modulePresent["infra"] {
		v.add("PIG_INFRA_GROUP_MISSING", SeverityWarning, Keys("all", "children", "infra"), nil, all.Range, "infra group is not declared")
	}
	if v.modulePresent["pgsql"] && !v.modulePresent["etcd"] {
		v.add("PIG_ETCD_GROUP_MISSING", SeverityWarning, Keys("all", "children", "etcd"), nil, all.Range, "etcd group is not declared for PostgreSQL")
	}
	globalVars := mappingValues(all.Vars)
	method, declared := globalVars["pgbackrest_method"]
	if !declared || v.modulePresent["minio"] {
		return
	}
	if v.unverifiable(method, "pgbackrest_method") {
		return
	}
	name, ok := stringValue(method)
	if ok && name == "minio" {
		v.add("PIG_MINIO_GROUP_MISSING", SeverityWarning, Keys("all", "children", "minio"), nil, all.Range, "minio group is not declared for the selected pgBackRest method")
	}
}

func (v *pigstyValidator) visitGroup(group *Group) {
	if group == nil {
		return
	}
	if groupValue, exists := v.doc.Lookup(group.Path); exists && v.unverifiable(groupValue, "group declaration") {
		return
	}
	if group.Name != "all" {
		v.validateGroupHostsShape(group)
	}

	varsReadable := !v.unverifiable(group.Vars, "group vars")
	groupVars := mappingValues(group.Vars)
	if varsReadable && group.Name != "all" {
		v.validateDeclaredObjects(groupVars)
	}
	if group.Name == "all" {
		for _, child := range group.Children {
			v.visitGroup(child)
		}
		return
	}

	pgCluster, pgDeclared := groupVars["pg_cluster"]
	pgIdentity := ""
	if pgDeclared {
		v.modulePresent["pgsql"] = true
		pgIdentity = v.validateCluster(pgCluster, group.Name, true)
	}
	groupVIPNeedsAddress := v.validateVIP(groupVars, false)
	groupVIPKnownMissing := len(group.Hosts) == 0

	groupModules := make(map[string]bool)
	groupIdentities := make(map[string]string)
	for _, spec := range pigstyModuleSpecs {
		clusterValue, clusterDeclared := groupVars[spec.cluster]
		groupModules[spec.module] = group.Name == spec.module || clusterDeclared
		if !groupModules[spec.module] {
			continue
		}
		v.modulePresent[spec.module] = true
		groupIdentities[spec.module] = group.Name
		if spec.cluster != "" && clusterDeclared {
			groupIdentities[spec.module] = v.validateModuleIdentity(clusterValue, spec.cluster, group.Name)
		}
	}

	for _, host := range group.Hosts {
		if host == nil {
			continue
		}
		if !validIPv4(host.Name) {
			v.add("PIG_HOST_IP", SeverityError, host.Path, nil, host.Range, fmt.Sprintf("host %q must be a valid IPv4 address", host.Name))
		}
		hostReadable := !v.unverifiable(&host.Vars, "host vars")
		hostVars := mappingValues(&host.Vars)
		if hostReadable {
			v.validateDeclaredObjects(hostVars)
			v.validateVIP(hostVars, true)
		}
		if groupVIPNeedsAddress && hostReadable {
			_, exists := hostVars["pg_vip_address"]
			if !exists {
				groupVIPKnownMissing = true
			}
		}

		hostPGCluster, hostPGDeclared := hostVars["pg_cluster"]
		_, hasPGSeq := hostVars["pg_seq"]
		_, hasPGRole := hostVars["pg_role"]
		isPG := pgDeclared || hostPGDeclared || hasPGSeq || hasPGRole
		if isPG {
			v.modulePresent["pgsql"] = true
			v.registerMember("pgsql", group, host)
			identity := pgIdentity
			identityDeclared := pgDeclared || !varsReadable
			if hostPGDeclared {
				identity = v.validateCluster(hostPGCluster, group.Name, false)
				identityDeclared = true
			}
			if identity == "" {
				identity = group.Name
			}
			if !identityDeclared {
				v.add("PIG_IDENTITY_MISSING", SeverityError, host.Path, nil, host.Range, fmt.Sprintf("host %q has no pg_cluster identity", host.Name))
			}
			if hostReadable {
				v.validatePGHost(host, hostVars, identity)
			}
		}

		for _, spec := range pigstyModuleSpecs {
			hostCluster, hostClusterDeclared := hostVars[spec.cluster]
			seqValue, hasSeq := hostVars[spec.seq]
			member := groupModules[spec.module] || hostClusterDeclared || hasSeq
			if !member {
				continue
			}
			v.modulePresent[spec.module] = true
			v.registerMember(spec.module, group, host)
			identity := groupIdentities[spec.module]
			identityDeclared := groupModules[spec.module] || !varsReadable
			if identity == "" {
				identity = group.Name
			}
			if hostClusterDeclared {
				identity = v.validateModuleIdentity(hostCluster, spec.cluster, group.Name)
				identityDeclared = true
			}
			if spec.cluster != "" && !identityDeclared {
				v.add("PIG_IDENTITY_MISSING", SeverityError, host.Path, nil, host.Range, fmt.Sprintf("host %q has no %s identity", host.Name, spec.cluster))
			}
			if !hostReadable {
				continue
			}
			if !hasSeq {
				v.add("PIG_SEQ_REQUIRED", SeverityError, host.Path.ChildKey(spec.seq), nil, host.Range, fmt.Sprintf("host %q requires %s", host.Name, spec.seq))
			} else {
				v.validateSequence(host, seqValue, spec.seq, identity)
			}
		}
	}
	if groupVIPNeedsAddress && groupVIPKnownMissing {
		enabled := groupVars["pg_vip_enabled"]
		parent, _ := enabled.Path.Parent()
		v.add("PIG_VIP_ADDRESS_REQUIRED", SeverityError, parent.ChildKey("pg_vip_address"), nil, enabled.Range, "pg_vip_address is required at group scope or for every declared host when pg_vip_enabled is true")
	}

	for _, child := range group.Children {
		v.visitGroup(child)
	}
}

func (v *pigstyValidator) validateGroupHostsShape(group *Group) {
	hostsPath := group.Path.ChildKey("hosts")
	hosts, declared := v.doc.Lookup(hostsPath)
	if !declared {
		if len(group.Children) == 0 {
			v.add("PIG_GROUP_HOSTS_REQUIRED", SeverityError, hostsPath, nil, group.Range, fmt.Sprintf("group %q must declare hosts", group.Name))
		}
		return
	}
	if v.unverifiable(hosts, "group hosts") {
		return
	}
	if (hosts.Kind == NullValue || hosts.Kind == MappingValue && len(hosts.Mapping) == 0) && len(group.Children) == 0 {
		v.add("PIG_GROUP_HOSTS_EMPTY", SeverityError, hostsPath, hosts, group.Range, fmt.Sprintf("group %q has no hosts", group.Name))
	}
}

func (v *pigstyValidator) validateCluster(value *Value, groupName string, compareGroup bool) string {
	if v.unverifiable(value, "pg_cluster") {
		return groupName
	}
	name, ok := stringValue(value)
	if !ok {
		v.add("PIG_PG_CLUSTER_TYPE", SeverityError, value.Path, value, SourceRange{}, "pg_cluster must be a string")
		return groupName
	}
	if compareGroup && name != groupName {
		v.add("PIG_PG_CLUSTER_GROUP", SeverityWarning, value.Path, value, SourceRange{}, "pg_cluster differs from its declaring group name")
	}
	if !pigstyClusterName.MatchString(name) {
		v.add("PIG_PG_CLUSTER", SeverityError, value.Path, value, SourceRange{}, "pg_cluster must contain only letters, digits, and hyphens")
		return groupName
	}
	return name
}

func (v *pigstyValidator) validateModuleIdentity(value *Value, field, fallback string) string {
	if v.unverifiable(value, field) {
		return fallback
	}
	identity, ok := stringValue(value)
	if !ok || identity == "" || strings.TrimSpace(identity) != identity {
		v.add("PIG_IDENTITY_TYPE", SeverityError, value.Path, value, SourceRange{}, field+" must be a non-empty string")
		return fallback
	}
	return identity
}

func (v *pigstyValidator) validatePGHost(host *Host, vars map[string]*Value, identity string) {
	if seq, exists := vars["pg_seq"]; !exists {
		v.add("PIG_PG_SEQ_REQUIRED", SeverityError, host.Path.ChildKey("pg_seq"), nil, host.Range, fmt.Sprintf("host %q requires pg_seq", host.Name))
	} else {
		v.validateSequence(host, seq, "pg_seq", identity)
	}
	if role, exists := vars["pg_role"]; !exists {
		v.add("PIG_PG_ROLE_REQUIRED", SeverityError, host.Path.ChildKey("pg_role"), nil, host.Range, fmt.Sprintf("host %q requires pg_role", host.Name))
	} else if !v.unverifiable(role, "pg_role") {
		name, ok := stringValue(role)
		if !ok || !validPGRole(name) {
			v.add("PIG_PG_ROLE", SeverityError, role.Path, role, SourceRange{}, fmt.Sprintf("pg_role for host %q must be primary, replica, standby, offline, or delayed", host.Name))
		}
	}
	if upstream, exists := vars["pg_upstream"]; exists && !v.unverifiable(upstream, "pg_upstream") {
		address, ok := stringValue(upstream)
		if !ok || !validIPv4(address) {
			v.add("PIG_PG_UPSTREAM", SeverityError, upstream.Path, upstream, SourceRange{}, "pg_upstream must be a valid IPv4 address")
		}
	}
}

func (v *pigstyValidator) registerMember(module string, group *Group, host *Host) {
	if v.memberships[module] == nil {
		v.memberships[module] = make(map[string]string)
	}
	previous, exists := v.memberships[module][host.Name]
	if exists && previous != group.Path.String() {
		v.add("PIG_HOST_MULTI_GROUP", SeverityError, host.Path, nil, host.Range, fmt.Sprintf("host %q belongs to multiple %s groups: %s and %s", host.Name, module, previous, group.Path.String()))
		return
	}
	v.memberships[module][host.Name] = group.Path.String()
}

func (v *pigstyValidator) validateSequence(host *Host, value *Value, name, identity string) {
	if v.unverifiable(value, name) {
		return
	}
	sequence, ok := nonNegativeInteger(value)
	if !ok {
		v.add("PIG_SEQ_TYPE", SeverityError, value.Path, value, SourceRange{}, fmt.Sprintf("%s for host %q must be a non-negative integer", name, host.Name))
		return
	}
	scope := name + "\x00" + identity
	if v.seenSeq[scope] == nil {
		v.seenSeq[scope] = make(map[uint64]string)
	}
	if previous, exists := v.seenSeq[scope][sequence]; exists {
		v.add("PIG_SEQ_DUPLICATE", SeverityError, value.Path, value, SourceRange{}, fmt.Sprintf("%s %d is duplicated by hosts %q and %q for identity %q", name, sequence, previous, host.Name, identity))
		return
	}
	v.seenSeq[scope][sequence] = host.Name
}

func (v *pigstyValidator) validateDeclaredObjects(vars map[string]*Value) {
	if value, exists := vars["pg_users"]; exists {
		v.validateUsers(value)
	}
	if value, exists := vars["pg_databases"]; exists {
		v.validateDatabases(value)
	}
	if value, exists := vars["pg_services"]; exists {
		v.validateServices(value)
	}
	for _, name := range []string{"pg_hba_rules", "pgb_hba_rules", "pg_default_hba_rules", "pgb_default_hba_rules"} {
		if value, exists := vars[name]; exists {
			v.validateHBA(name, value)
		}
	}
}

func (v *pigstyValidator) validateUsers(value *Value) {
	items, ok := v.declarationSequence("PIG_USERS_LIST", "pg_users", value)
	if !ok {
		return
	}
	for index := range items {
		item := &items[index]
		if v.unverifiable(item, "pg_users item") {
			continue
		}
		if item.Kind != MappingValue {
			v.add("PIG_USER_ITEM", SeverityError, item.Path, item, SourceRange{}, "pg_users items must be mappings")
			continue
		}
		fields := mappingValues(item)
		if _, exists := fields["name"]; !exists {
			v.add("PIG_USER_NAME_REQUIRED", SeverityError, item.Path.ChildKey("name"), nil, item.Range, "pg_users item requires name")
		}
		if _, exists := fields["password"]; !exists {
			v.add("PIG_USER_PASSWORD_MISSING", SeverityWarning, item.Path.ChildKey("password"), nil, item.Range, "pg_users item does not declare password")
		}
		v.validateFields("PIG_USER", item, userFields, nil)
	}
}

func (v *pigstyValidator) validateDatabases(value *Value) {
	items, ok := v.declarationSequence("PIG_DATABASES_LIST", "pg_databases", value)
	if !ok {
		return
	}
	for index := range items {
		item := &items[index]
		if v.unverifiable(item, "pg_databases item") {
			continue
		}
		if item.Kind != MappingValue {
			v.add("PIG_DATABASE_ITEM", SeverityError, item.Path, item, SourceRange{}, "pg_databases items must be mappings")
			continue
		}
		fields := mappingValues(item)
		if _, exists := fields["name"]; !exists {
			v.add("PIG_DATABASE_NAME_REQUIRED", SeverityError, item.Path.ChildKey("name"), nil, item.Range, "pg_databases item requires name")
		}
		v.validateFields("PIG_DATABASE", item, databaseFields, nil)
	}
}

func (v *pigstyValidator) validateServices(value *Value) {
	items, ok := v.declarationSequence("PIG_SERVICES_LIST", "pg_services", value)
	if !ok {
		return
	}
	ports := make(map[uint64]Path)
	for index := range items {
		item := &items[index]
		if v.unverifiable(item, "pg_services item") {
			continue
		}
		if item.Kind != MappingValue {
			v.add("PIG_SERVICE_ITEM", SeverityError, item.Path, item, SourceRange{}, "pg_services items must be mappings")
			continue
		}
		fields := mappingValues(item)
		if _, exists := fields["name"]; !exists {
			v.add("PIG_SERVICE_NAME_REQUIRED", SeverityError, item.Path.ChildKey("name"), nil, item.Range, "pg_services item requires name")
		}
		port, exists := fields["port"]
		if !exists {
			v.add("PIG_SERVICE_PORT_REQUIRED", SeverityError, item.Path.ChildKey("port"), nil, item.Range, "pg_services item requires port")
		} else if !v.unverifiable(port, "pg_services port") {
			number, integer := nonNegativeInteger(port)
			if !integer || number > 65535 {
				v.add("PIG_SERVICE_PORT", SeverityError, port.Path, port, SourceRange{}, "pg_services port must be an integer between 0 and 65535")
			} else {
				if _, duplicate := ports[number]; duplicate {
					v.add("PIG_SERVICE_PORT_DUPLICATE", SeverityError, port.Path, port, SourceRange{}, "pg_services port is duplicated")
				} else {
					ports[number] = clonePath(port.Path)
				}
				if _, reserved := reservedServicePorts[number]; reserved {
					v.add("PIG_SERVICE_PORT_RESERVED", SeverityWarning, port.Path, port, SourceRange{}, "pg_services port may be reserved by a built-in service")
				}
			}
		}
		v.validateFields("PIG_SERVICE", item, serviceFields, map[string]struct{}{"port": {}})
	}
}

func (v *pigstyValidator) declarationSequence(code, subject string, value *Value) ([]Value, bool) {
	if v.unverifiable(value, subject) {
		return nil, false
	}
	if value == nil || value.Kind != SequenceValue {
		path := Path(nil)
		if value != nil {
			path = value.Path
		}
		v.add(code, SeverityError, path, value, SourceRange{}, subject+" must be a sequence")
		return nil, false
	}
	return value.Sequence, true
}

func (v *pigstyValidator) validateFields(prefix string, item *Value, known map[string]string, skip map[string]struct{}) {
	for index := range item.Mapping {
		entry := &item.Mapping[index]
		name := fmt.Sprint(entry.Key.Value)
		expected, recognized := known[name]
		if !recognized {
			v.add(prefix+"_FIELD_UNKNOWN", SeverityWarning, entry.Path, &entry.Value, SourceRange{}, "declaration contains an unknown field "+name)
			continue
		}
		if _, skipped := skip[name]; skipped {
			continue
		}
		if v.unverifiable(&entry.Value, name) {
			continue
		}
		if !v.valueMatchesType(&entry.Value, expected) {
			v.add(prefix+"_FIELD_TYPE", SeverityError, entry.Path, &entry.Value, SourceRange{}, name+" must be "+expected)
		}
	}
}

func (v *pigstyValidator) valueMatchesType(value *Value, expected string) bool {
	switch expected {
	case "string":
		_, ok := stringValue(value)
		return ok
	case "integer":
		return value != nil && value.Kind == ScalarValue && value.Scalar != nil && value.Scalar.Kind == IntegerScalar
	case "float":
		return value != nil && value.Kind == ScalarValue && value.Scalar != nil && value.Scalar.Kind == FloatScalar
	case "bool":
		return value != nil && value.Kind == ScalarValue && value.Scalar != nil && value.Scalar.Kind == BoolScalar
	case "sequence":
		return value != nil && value.Kind == SequenceValue
	case "mapping":
		return value != nil && value.Kind == MappingValue
	case "date":
		text, ok := stringValue(value)
		return ok && pigstyDate.MatchString(text)
	case "string[]":
		if value == nil || value.Kind != SequenceValue {
			return false
		}
		valid := true
		for index := range value.Sequence {
			item := &value.Sequence[index]
			if v.unverifiable(item, "sequence item") {
				continue
			}
			if _, ok := stringValue(item); !ok {
				valid = false
			}
		}
		return valid
	case "mapping[]":
		if value == nil || value.Kind != SequenceValue {
			return false
		}
		valid := true
		for index := range value.Sequence {
			item := &value.Sequence[index]
			if v.unverifiable(item, "sequence item") {
				continue
			}
			if item.Kind != MappingValue {
				valid = false
			}
		}
		return valid
	}
	if strings.HasPrefix(expected, "enum:") {
		text, ok := stringValue(value)
		if !ok {
			return false
		}
		for _, allowed := range strings.Split(strings.TrimPrefix(expected, "enum:"), ",") {
			if text == allowed {
				return true
			}
		}
		return false
	}
	return false
}

func (v *pigstyValidator) validateHBA(name string, value *Value) {
	items, ok := v.declarationSequence("PIG_HBA_LIST", name, value)
	if !ok {
		return
	}
	for index := range items {
		item := &items[index]
		if v.unverifiable(item, name+" item") {
			continue
		}
		if item.Kind != MappingValue {
			v.add("PIG_HBA_ITEM", SeverityError, item.Path, item, SourceRange{}, name+" items must be mappings")
			continue
		}
		fields := mappingValues(item)
		if _, exists := fields["title"]; !exists {
			v.add("PIG_HBA_TITLE_MISSING", SeverityWarning, item.Path.ChildKey("title"), nil, item.Range, name+" item does not declare title")
		}
		for fieldIndex := range item.Mapping {
			entry := &item.Mapping[fieldIndex]
			field := fmt.Sprint(entry.Key.Value)
			if _, known := hbaFields[field]; !known {
				v.add("PIG_HBA_FIELD_UNKNOWN", SeverityWarning, entry.Path, &entry.Value, SourceRange{}, name+" item contains an unknown field "+field)
			}
		}
		if rules, exists := fields["rules"]; exists {
			if v.unverifiable(rules, name+" rules") {
				continue
			}
			if !v.valueMatchesType(rules, "string[]") {
				v.add("PIG_HBA_RULES_TYPE", SeverityError, rules.Path, rules, SourceRange{}, name+" rules must be a sequence of strings")
			}
			continue
		}
		for _, required := range []string{"addr", "auth", "user", "db"} {
			if _, exists := fields[required]; !exists {
				v.add("PIG_HBA_FIELD_REQUIRED", SeverityError, item.Path.ChildKey(required), nil, item.Range, name+" item requires "+required)
			}
		}
		if address, exists := fields["addr"]; exists && !v.unverifiable(address, name+" addr") {
			text, stringType := stringValue(address)
			_, alias := hbaAddressAliases[text]
			if !stringType || !alias && !validIPv4CIDR(text) {
				v.add("PIG_HBA_ADDR", SeverityError, address.Path, address, SourceRange{}, name+" addr must be a known alias or valid IPv4 CIDR")
			}
		}
		if auth, exists := fields["auth"]; exists && !v.unverifiable(auth, name+" auth") {
			text, stringType := stringValue(auth)
			_, alias := hbaAuthAliases[text]
			if !stringType || !alias {
				v.add("PIG_HBA_AUTH", SeverityError, auth.Path, auth, SourceRange{}, name+" auth must be a supported alias")
			}
		}
	}
}

// validateVIP validates explicit declarations and reports whether a true
// enablement still needs an address supplied by its enclosing group hosts.
func (v *pigstyValidator) validateVIP(vars map[string]*Value, enforceAddress bool) bool {
	enabled, hasEnabled := vars["pg_vip_enabled"]
	address, hasAddress := vars["pg_vip_address"]
	addressNeeded := false
	if hasEnabled && !v.unverifiable(enabled, "pg_vip_enabled") {
		if enabled.Kind != ScalarValue || enabled.Scalar == nil || enabled.Scalar.Kind != BoolScalar {
			v.add("PIG_VIP_ENABLED_TYPE", SeverityError, enabled.Path, enabled, SourceRange{}, "pg_vip_enabled must be a boolean")
		} else {
			addressNeeded, _ = enabled.Scalar.Value.(bool)
		}
	}
	if addressNeeded && !hasAddress && enforceAddress {
		parent, _ := enabled.Path.Parent()
		v.add("PIG_VIP_ADDRESS_REQUIRED", SeverityError, parent.ChildKey("pg_vip_address"), nil, enabled.Range, "pg_vip_address is required when pg_vip_enabled is true")
	}
	if hasAddress && !v.unverifiable(address, "pg_vip_address") {
		text, ok := stringValue(address)
		if !ok || !validIPv4CIDR(text) {
			v.add("PIG_VIP_ADDRESS", SeverityError, address.Path, address, SourceRange{}, "pg_vip_address must be a valid IPv4 CIDR")
		}
	}
	return addressNeeded && !hasAddress
}

func validPGRole(role string) bool {
	return role == "primary" || role == "replica" || role == "standby" || role == "offline" || role == "delayed"
}

func validIPv4(value string) bool {
	address, err := netip.ParseAddr(value)
	return err == nil && address.Is4()
}

func validIPv4CIDR(value string) bool {
	prefix, err := netip.ParsePrefix(value)
	return err == nil && prefix.Addr().Is4()
}

func stringValue(value *Value) (string, bool) {
	if value == nil || value.Kind != ScalarValue || value.Scalar == nil || value.Scalar.Kind != StringScalar {
		return "", false
	}
	text, ok := value.Scalar.Value.(string)
	if !ok {
		return "", false
	}
	return text, true
}

func nonNegativeInteger(value *Value) (uint64, bool) {
	if value == nil || value.Kind != ScalarValue || value.Scalar == nil || value.Scalar.Kind != IntegerScalar {
		return 0, false
	}
	switch number := value.Scalar.Value.(type) {
	case int:
		return uint64(number), number >= 0
	case int64:
		return uint64(number), number >= 0
	case uint64:
		return number, true
	case uint:
		return uint64(number), true
	default:
		return 0, false
	}
}
