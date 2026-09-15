package product

import (
	"maps"
	"path"
	"slices"
	"strings"
)

const tpchCatalog = "tpch"

// BaseFacts supplies the bundled TPCH catalog when no external catalog is referenced.
func BaseFacts() TrinoFacts {
	return TrinoFacts{Catalogs: map[string]map[string]string{tpchCatalog: {"connector.name": tpchCatalog}}}
}

func cloneFacts(in TrinoFacts) TrinoFacts {
	out := in
	out.Catalogs = make(map[string]map[string]string, len(in.Catalogs))
	for name, properties := range in.Catalogs {
		out.Catalogs[name] = maps.Clone(properties)
	}
	out.S3.Credentials.Scope = slices.Clone(in.S3.Credentials.Scope)
	return out
}

func sortedKeys[V any](values map[string]V) []string { return slices.Sorted(maps.Keys(values)) }

func relativeFile(value string) bool {
	return value != "" && value != "." && value != ".." && !path.IsAbs(value) &&
		path.Clean(value) == value && !strings.HasPrefix(value, "../") && !strings.ContainsRune(value, '\x00')
}
