package inventory

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	SelectorSyntax   = "SELECTOR_SYNTAX"
	SelectorNotFound = "SELECTOR_NOT_FOUND"
	SelectorType     = "SELECTOR_TYPE"
)

// SelectorError is a stable, user-facing selector failure.
type SelectorError struct {
	Code     string `json:"code" yaml:"code"`
	Selector string `json:"selector" yaml:"selector"`
	Message  string `json:"message" yaml:"message"`
}

func (e *SelectorError) Error() string {
	if e == nil {
		return "inventory selector failed"
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// ParseSelector resolves a user selector against doc. An empty selector means
// the whole document. A leading slash is an unambiguous canonical pointer;
// otherwise Pigsty shorthand maps vars to all.vars and a leading group name to
// all.children.<group>. Keys containing dots or brackets must be quoted, e.g.
// vars.infra_portal["i.pigsty"] or pg-meta.hosts["10.10.10.10"].
func ParseSelector(doc *Document, selector string) (Path, error) {
	if doc == nil {
		return nil, selectorFailure(SelectorType, selector, "inventory document is nil")
	}
	selector = strings.TrimSpace(selector)
	if selector == "" || selector == "/" {
		return nil, nil
	}
	if strings.HasPrefix(selector, "/") {
		return resolvePointer(doc, selector)
	}
	parts, err := parseSelectorParts(selector)
	if err != nil {
		return nil, err
	}
	if len(parts) == 0 || parts[0].Kind != KeySegment {
		return nil, selectorFailure(SelectorSyntax, selector, "selector must start with vars or a group name")
	}

	var path Path
	group := false
	children := false
	if parts[0].Key == "vars" {
		path = Keys("all", "vars")
	} else {
		path = Keys("all", "children", parts[0].Key)
		group = true
	}
	if err := requireSelectorPath(doc, selector, path); err != nil {
		return nil, err
	}

	for _, part := range parts[1:] {
		if part.Kind == IndexSegment {
			path = path.ChildIndex(part.Index)
			group, children = false, false
		} else if children {
			path = path.ChildKey(part.Key)
			group, children = true, false
		} else if group {
			switch part.Key {
			case "vars", "hosts":
				path = path.ChildKey(part.Key)
				group = false
			case "children":
				path = path.ChildKey(part.Key)
				group, children = false, true
			default:
				path = path.ChildKey("children")
				if err := requireSelectorPath(doc, selector, path); err != nil {
					return nil, err
				}
				path = path.ChildKey(part.Key)
				group = true
			}
		} else {
			path = path.ChildKey(part.Key)
		}
		if err := requireSelectorPath(doc, selector, path); err != nil {
			return nil, err
		}
	}
	return path, nil
}

func resolvePointer(doc *Document, selector string) (Path, error) {
	if selector == "/" {
		return nil, nil
	}
	var path Path
	for _, encoded := range strings.Split(strings.TrimPrefix(selector, "/"), "/") {
		segment := strings.ReplaceAll(strings.ReplaceAll(encoded, "~1", "/"), "~0", "~")
		current, ok := doc.Lookup(path)
		if !ok {
			return nil, selectorFailure(SelectorNotFound, selector, "path %s does not exist", path.String())
		}
		switch current.Kind {
		case MappingValue:
			path = path.ChildKey(segment)
		case SequenceValue:
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 {
				return nil, selectorFailure(SelectorType, selector, "%q is not a sequence index at %s", segment, path.String())
			}
			path = path.ChildIndex(index)
		default:
			return nil, selectorFailure(SelectorType, selector, "cannot descend through %s at %s", current.Kind, path.String())
		}
		if err := requireSelectorPath(doc, selector, path); err != nil {
			return nil, err
		}
	}
	return path, nil
}

func requireSelectorPath(doc *Document, selector string, path Path) error {
	if _, ok := doc.Lookup(path); ok {
		return nil
	}
	return selectorFailure(SelectorNotFound, selector, "path %s does not exist", path.String())
}

func parseSelectorParts(selector string) ([]Segment, error) {
	parts := make([]Segment, 0, 4)
	for cursor := 0; cursor < len(selector); {
		if selector[cursor] == '.' {
			return nil, selectorFailure(SelectorSyntax, selector, "empty selector segment at byte %d", cursor)
		}
		if selector[cursor] == '[' {
			part, next, err := parseBracketPart(selector, cursor)
			if err != nil {
				return nil, err
			}
			parts = append(parts, part)
			cursor = next
		} else {
			start := cursor
			for cursor < len(selector) && selector[cursor] != '.' && selector[cursor] != '[' {
				cursor++
			}
			if start == cursor {
				return nil, selectorFailure(SelectorSyntax, selector, "empty selector segment at byte %d", start)
			}
			parts = append(parts, Key(selector[start:cursor]))
		}
		if cursor == len(selector) {
			break
		}
		if selector[cursor] == '.' {
			cursor++
			if cursor == len(selector) || selector[cursor] == '.' || selector[cursor] == '[' {
				return nil, selectorFailure(SelectorSyntax, selector, "invalid dot separator at byte %d", cursor-1)
			}
			continue
		}
		if selector[cursor] != '[' {
			return nil, selectorFailure(SelectorSyntax, selector, "unexpected byte %q at %d", selector[cursor], cursor)
		}
	}
	return parts, nil
}

func parseBracketPart(selector string, start int) (Segment, int, error) {
	cursor := start + 1
	if cursor >= len(selector) {
		return Segment{}, 0, selectorFailure(SelectorSyntax, selector, "unterminated bracket at byte %d", start)
	}
	if selector[cursor] == '"' {
		quoted := cursor
		cursor++
		escaped := false
		for cursor < len(selector) {
			if escaped {
				escaped = false
				cursor++
				continue
			}
			if selector[cursor] == '\\' {
				escaped = true
				cursor++
				continue
			}
			if selector[cursor] == '"' {
				break
			}
			cursor++
		}
		if cursor >= len(selector) || cursor+1 >= len(selector) || selector[cursor+1] != ']' {
			return Segment{}, 0, selectorFailure(SelectorSyntax, selector, "unterminated quoted key at byte %d", start)
		}
		key, err := strconv.Unquote(selector[quoted : cursor+1])
		if err != nil {
			return Segment{}, 0, selectorFailure(SelectorSyntax, selector, "invalid quoted key at byte %d", start)
		}
		return Key(key), cursor + 2, nil
	}
	end := strings.IndexByte(selector[cursor:], ']')
	if end < 0 {
		return Segment{}, 0, selectorFailure(SelectorSyntax, selector, "unterminated index at byte %d", start)
	}
	end += cursor
	index, err := strconv.Atoi(selector[cursor:end])
	if err != nil || index < 0 {
		return Segment{}, 0, selectorFailure(SelectorSyntax, selector, "bracket value at byte %d must be a quoted key or non-negative index", start)
	}
	return Index(index), end + 1, nil
}

func selectorFailure(code, selector, format string, args ...any) error {
	return &SelectorError{Code: code, Selector: selector, Message: fmt.Sprintf(format, args...)}
}
