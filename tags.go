package zon

import (
	"reflect"
	"strings"
	"sync"
)

// structField is a single encodable/decodable field of a Go struct.
type structField struct {
	name      string
	index     int
	omitEmpty bool
}

// structInfo holds a struct type's fields both in declaration order (for
// encoding) and indexed by ZON name (for decoding).
type structInfo struct {
	ordered []structField
	byName  map[string]structField
}

var fieldCache sync.Map // reflect.Type -> structInfo

// typeFields returns the cached field information for a struct type, honoring
// `zon:"name,omitempty"` tags. A name of "-" omits the field.
func typeFields(t reflect.Type) structInfo {
	if c, ok := fieldCache.Load(t); ok {
		return c.(structInfo)
	}
	info := structInfo{byName: map[string]structField{}}
	for i := range t.NumField() {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name, opts := parseTag(f.Tag.Get("zon"))
		if name == "-" {
			continue
		}
		if name == "" {
			name = f.Name
		}
		sf := structField{name: name, index: i, omitEmpty: opts.contains("omitempty")}
		info.ordered = append(info.ordered, sf)
		info.byName[name] = sf
	}
	fieldCache.Store(t, info)
	return info
}

type tagOptions string

func parseTag(tag string) (string, tagOptions) {
	name, opt, _ := strings.Cut(tag, ",")
	return name, tagOptions(opt)
}

func (o tagOptions) contains(opt string) bool {
	s := string(o)
	for s != "" {
		var cur string
		cur, s, _ = strings.Cut(s, ",")
		if cur == opt {
			return true
		}
	}
	return false
}
