package inventory

import (
	"strings"
)

// AnsibleInventoryJSON renders the private declaration-only protocol body used
// by the native adapter. The returned bytes may contain Inventory secrets and
// must never be placed in a public Result, Plan, diagnostic, or log.
func (s *Snapshot) AnsibleInventoryJSON() ([]byte, error) {
	if s == nil || s.root.kind != SnapshotMap {
		return nil, snapshotFailure(nil, SnapshotReasonInvalidProjection)
	}
	projection := newAnsibleProjection()
	for i := range s.root.entries {
		entry := s.root.entries[i]
		if err := projection.declareGroup(entry.key, entry.value, Keys(entry.key)); err != nil {
			return nil, err
		}
	}
	return projection.json()
}

type ansibleProjection struct {
	groups      []*projectedGroup
	groupsByKey map[string]*projectedGroup
	hosts       []projectedHost
	hostsByKey  map[string]int
}

type projectedGroup struct {
	name       string
	fieldOrder []string
	hosts      []string
	hostSet    map[string]struct{}
	children   []string
	childSet   map[string]struct{}
	vars       *SnapshotValue
}

type projectedHost struct {
	name string
	vars SnapshotValue
}

func newAnsibleProjection() *ansibleProjection {
	return &ansibleProjection{
		groupsByKey: make(map[string]*projectedGroup),
		hostsByKey:  make(map[string]int),
	}
}

func (p *ansibleProjection) declareGroup(name string, value SnapshotValue, path Path) error {
	group := p.groupsByKey[name]
	if group == nil {
		group = &projectedGroup{
			name: name, hosts: make([]string, 0), hostSet: make(map[string]struct{}),
			children: make([]string, 0), childSet: make(map[string]struct{}),
		}
		p.groupsByKey[name] = group
		p.groups = append(p.groups, group)
	}
	if value.kind == SnapshotNull {
		return nil
	}
	if value.kind != SnapshotMap {
		return snapshotFailure(path, SnapshotReasonInvalidProjection)
	}
	for i := range value.entries {
		field := value.entries[i]
		fieldPath := path.ChildKey(field.key)
		switch field.key {
		case "hosts":
			group.rememberField("hosts")
			if field.value.kind == SnapshotNull {
				continue
			}
			if field.value.kind != SnapshotMap {
				return snapshotFailure(fieldPath, SnapshotReasonInvalidProjection)
			}
			for hostIndex := range field.value.entries {
				host := field.value.entries[hostIndex]
				if host.value.kind != SnapshotMap && host.value.kind != SnapshotNull {
					return snapshotFailure(fieldPath.ChildKey(host.key), SnapshotReasonInvalidProjection)
				}
				if _, exists := group.hostSet[host.key]; !exists {
					group.hostSet[host.key] = struct{}{}
					group.hosts = append(group.hosts, host.key)
				}
				vars := host.value
				if vars.kind == SnapshotNull {
					vars = SnapshotValue{kind: SnapshotMap}
				}
				if previous, exists := p.hostsByKey[host.key]; exists {
					if !snapshotValueEqual(p.hosts[previous].vars, vars) {
						return snapshotFailure(fieldPath.ChildKey(host.key), SnapshotReasonDuplicateHostVars)
					}
					continue
				}
				p.hostsByKey[host.key] = len(p.hosts)
				p.hosts = append(p.hosts, projectedHost{name: host.key, vars: vars})
			}

		case "children":
			group.rememberField("children")
			if field.value.kind == SnapshotNull {
				continue
			}
			if field.value.kind != SnapshotMap {
				return snapshotFailure(fieldPath, SnapshotReasonInvalidProjection)
			}
			for childIndex := range field.value.entries {
				child := field.value.entries[childIndex]
				if _, exists := group.childSet[child.key]; !exists {
					group.childSet[child.key] = struct{}{}
					group.children = append(group.children, child.key)
				}
				if err := p.declareGroup(child.key, child.value, fieldPath.ChildKey(child.key)); err != nil {
					return err
				}
			}

		case "vars":
			group.rememberField("vars")
			vars := field.value
			if vars.kind == SnapshotNull {
				vars = SnapshotValue{kind: SnapshotMap}
			}
			if vars.kind != SnapshotMap {
				return snapshotFailure(fieldPath, SnapshotReasonInvalidProjection)
			}
			merged := mergeProjectedVars(group.vars, vars)
			group.vars = &merged
		}
	}
	return nil
}

func (g *projectedGroup) rememberField(field string) {
	for _, existing := range g.fieldOrder {
		if existing == field {
			return
		}
	}
	g.fieldOrder = append(g.fieldOrder, field)
}

func mergeProjectedVars(current *SnapshotValue, incoming SnapshotValue) SnapshotValue {
	if current == nil {
		return cloneSnapshotValue(incoming)
	}
	merged := cloneSnapshotValue(*current)
	indexes := make(map[string]int, len(merged.entries))
	for i := range merged.entries {
		indexes[merged.entries[i].key] = i
	}
	for i := range incoming.entries {
		entry := incoming.entries[i]
		if index, exists := indexes[entry.key]; exists {
			merged.entries[index].value = cloneSnapshotValue(entry.value)
			continue
		}
		indexes[entry.key] = len(merged.entries)
		merged.entries = append(merged.entries, SnapshotEntry{key: entry.key, value: cloneSnapshotValue(entry.value)})
	}
	return merged
}

func (p *ansibleProjection) json() ([]byte, error) {
	writer := newSnapshotByteWriter(maxSnapshotOutputBytes)
	if err := writer.literal(`{"_meta":{"hostvars":{`); err != nil {
		return nil, snapshotFailure(nil, SnapshotReasonOutputTooLarge)
	}
	for i := range p.hosts {
		if i > 0 {
			if err := writer.byte(','); err != nil {
				return nil, snapshotFailure(nil, SnapshotReasonOutputTooLarge)
			}
		}
		if err := writer.jsonString(p.hosts[i].name); err != nil {
			return nil, snapshotFailure(nil, SnapshotReasonOutputTooLarge)
		}
		if err := writer.byte(':'); err != nil {
			return nil, snapshotFailure(nil, SnapshotReasonOutputTooLarge)
		}
		if err := writeProjectionValue(writer, p.hosts[i].vars); err != nil {
			return nil, err
		}
	}
	if err := writer.literal(`}}`); err != nil {
		return nil, snapshotFailure(nil, SnapshotReasonOutputTooLarge)
	}
	for i := range p.groups {
		if err := writer.byte(','); err != nil {
			return nil, snapshotFailure(nil, SnapshotReasonOutputTooLarge)
		}
		if err := writer.jsonString(p.groups[i].name); err != nil {
			return nil, snapshotFailure(nil, SnapshotReasonOutputTooLarge)
		}
		if err := writer.byte(':'); err != nil {
			return nil, snapshotFailure(nil, SnapshotReasonOutputTooLarge)
		}
		if err := writeProjectedGroup(writer, p.groups[i]); err != nil {
			return nil, err
		}
	}
	if err := writer.byte('}'); err != nil {
		return nil, snapshotFailure(nil, SnapshotReasonOutputTooLarge)
	}
	return writer.bytes(), nil
}

func writeProjectedGroup(writer *snapshotByteWriter, group *projectedGroup) error {
	if err := writer.byte('{'); err != nil {
		return snapshotFailure(nil, SnapshotReasonOutputTooLarge)
	}
	fields := 0
	for _, field := range group.fieldOrder {
		if fields > 0 {
			if err := writer.byte(','); err != nil {
				return snapshotFailure(nil, SnapshotReasonOutputTooLarge)
			}
		}
		fields++
		if err := writer.jsonString(field); err != nil {
			return snapshotFailure(nil, SnapshotReasonOutputTooLarge)
		}
		if err := writer.byte(':'); err != nil {
			return snapshotFailure(nil, SnapshotReasonOutputTooLarge)
		}
		switch field {
		case "hosts":
			if err := writeProjectionStrings(writer, group.hosts); err != nil {
				return err
			}
		case "children":
			if err := writeProjectionStrings(writer, group.children); err != nil {
				return err
			}
		case "vars":
			vars := SnapshotValue{kind: SnapshotMap}
			if group.vars != nil {
				vars = *group.vars
			}
			if err := writeProjectionValue(writer, vars); err != nil {
				return err
			}
		}
	}
	if err := writer.byte('}'); err != nil {
		return snapshotFailure(nil, SnapshotReasonOutputTooLarge)
	}
	return nil
}

func writeProjectionStrings(writer *snapshotByteWriter, values []string) error {
	if err := writer.byte('['); err != nil {
		return snapshotFailure(nil, SnapshotReasonOutputTooLarge)
	}
	for i := range values {
		if i > 0 {
			if err := writer.byte(','); err != nil {
				return snapshotFailure(nil, SnapshotReasonOutputTooLarge)
			}
		}
		if err := writer.jsonString(values[i]); err != nil {
			return snapshotFailure(nil, SnapshotReasonOutputTooLarge)
		}
	}
	if err := writer.byte(']'); err != nil {
		return snapshotFailure(nil, SnapshotReasonOutputTooLarge)
	}
	return nil
}

func writeProjectionValue(writer *snapshotByteWriter, value SnapshotValue) error {
	var err error
	switch value.kind {
	case SnapshotMap:
		err = writer.byte('{')
		for i := range value.entries {
			if err != nil {
				break
			}
			if i > 0 {
				err = writer.byte(',')
			}
			if err == nil {
				err = writer.jsonString(value.entries[i].key)
			}
			if err == nil {
				err = writer.byte(':')
			}
			if err == nil {
				err = writeProjectionValue(writer, value.entries[i].value)
			}
		}
		if err == nil {
			err = writer.byte('}')
		}
	case SnapshotSeq:
		err = writer.byte('[')
		for i := range value.items {
			if err != nil {
				break
			}
			if i > 0 {
				err = writer.byte(',')
			}
			if err == nil {
				err = writeProjectionValue(writer, value.items[i])
			}
		}
		if err == nil {
			err = writer.byte(']')
		}
	case SnapshotStr:
		err = writer.jsonString(value.text)
	case SnapshotBool:
		if value.boolean {
			err = writer.literal("true")
		} else {
			err = writer.literal("false")
		}
	case SnapshotInt:
		err = writer.literal(value.text)
	case SnapshotFloat:
		err = writer.literal(snapshotFloatLiteral(value.text))
	case SnapshotNull:
		err = writer.literal("null")
	default:
		return snapshotFailure(nil, SnapshotReasonInvalidProjection)
	}
	if err != nil {
		return snapshotFailure(nil, SnapshotReasonOutputTooLarge)
	}
	return nil
}

func snapshotFloatLiteral(value string) string {
	if !strings.ContainsAny(value, ".eE") {
		return value + ".0"
	}
	return value
}
