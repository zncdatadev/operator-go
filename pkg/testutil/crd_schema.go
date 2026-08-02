/*
Copyright 2024 ZNCDataDev.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package testutil

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/onsi/gomega/types"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"sigs.k8s.io/yaml"
)

// A `+kubebuilder:default` on any field inside a role's or role group's `config` block is a defect,
// and this file is the executable form of that rule.
//
// The mechanism: structural defaulting fills a leaf as soon as its ENCLOSING OBJECT exists. The
// framework folds `config` Role -> RoleGroup, so a default inside it lands in every role group that
// declared the enclosing object for any reason at all — and from that moment "the group did not set
// this" and "the group asked for exactly the default" are the same bytes. The role's value can
// never win again. Defaults for these fields belong at consumption time, where the code can still
// tell the two apart (`StorageResource.GetCapacity`,
// `RoleGroupConfigSpec.GetGracefulShutdownTimeout`).
//
// **The rule holds at every depth, and that is not a theoretical position.** The obvious case is a
// leaf directly under `config`, where any group writing `config:` at all is hit. But
// `LogLevelSpec.Level` sat two levels down, under `logging.containers[*].console` — an object a
// role group only ever declares deliberately — and it was a live defect for months anyway: a role
// asking for DEBUG plus a role group writing an empty `console: {}` produced INFO, and the guard
// written in `productlogging` to prevent exactly that could never fire, because the API server had
// filled the field before the merge saw it. Depth does not make a default in this subtree safe; it
// only makes the failure rarer and therefore harder to attribute. So the check reports every
// default under a folded `config`, with no depth heuristic.

// InheritedConfigDefault names one offending leaf: the CRD it lives in, the JSON path to it, and
// the default that was found there.
type InheritedConfigDefault struct {
	// CRD is the `metadata.name` of the CustomResourceDefinition, e.g. "trinoclusters.trino.kubedoop.dev".
	CRD string
	// Version is the schema version the default was found in, e.g. "v1alpha1". A CRD serving several
	// versions is reported once per version, because they carry independent schemas.
	Version string
	// Path is the dotted JSON path from `.spec`, e.g.
	// ".spec.roles[*].roleGroups[*].config.queryMaxMemory". Map-valued nodes render as `[*]`.
	Path string
	// Default is the default value as it appears in the schema.
	Default any
	// File is the CRD YAML the finding came from.
	File string
}

// String renders one finding for a failure message.
func (d InheritedConfigDefault) String() string {
	return fmt.Sprintf("%s (%s) %s = %v  [%s]", d.CRD, d.Version, d.Path, d.Default, d.File)
}

// FindInheritedConfigDefaults walks every schema in the given CRD YAML files and returns each
// property declaring a `default` while sitting inside a role or role group `config` subtree — the
// block whose value is folded Role -> RoleGroup. See the package-level explanation above for why
// that is a defect at any depth.
//
// Arguments are file paths or globs (`config/crd/bases/*.yaml`). Documents that are not
// CustomResourceDefinitions are skipped, so a glob over a directory holding a kustomization file is
// fine. It is an ERROR for the arguments to match no files at all: a guard that silently inspects
// nothing is worse than no guard, because it reports success.
//
// Detection is product-agnostic and structural rather than path-literal. Any schema node that
// declares a `roleGroups` property is treated as a role, which makes both shapes work: the generic
// `spec.roles[*]` map and a product that flattens its roles into named fields
// (`spec.coordinators`, `spec.workers`). Both the role's own `config` and the role group's are
// walked, because a single Go type generates both and a default appears at both paths or neither.
func FindInheritedConfigDefaults(pathsOrGlobs ...string) ([]InheritedConfigDefault, error) {
	files, err := resolveCRDFiles(pathsOrGlobs)
	if err != nil {
		return nil, err
	}

	var found []InheritedConfigDefault
	for _, file := range files {
		raw, err := os.ReadFile(file) //nolint:gosec // test helper, path supplied by the caller
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", file, err)
		}
		docs, err := splitYAMLDocuments(raw)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", file, err)
		}
		for _, doc := range docs {
			// Check the kind before the full decode, so a malformed CRD is an ERROR while an
			// unrelated document (a kustomization.yaml caught by a glob) is skipped. Swallowing
			// both would reopen the failure mode this package exists to prevent: a guard that
			// passes because it inspected fewer CRDs than the caller believes.
			var meta metav1.TypeMeta
			if err := yaml.Unmarshal(doc, &meta); err != nil || meta.Kind != "CustomResourceDefinition" {
				continue
			}
			crd := &apiextensionsv1.CustomResourceDefinition{}
			if err := yaml.Unmarshal(doc, crd); err != nil {
				return nil, fmt.Errorf("parsing a CustomResourceDefinition in %s: %w", file, err)
			}
			for _, version := range crd.Spec.Versions {
				if version.Schema == nil || version.Schema.OpenAPIV3Schema == nil {
					continue
				}
				// Only `.spec`. The contract is about the DESIRED state the framework folds Role ->
				// RoleGroup; a `config` under `.status` is written by the operator and never merged,
				// so a default there means nothing to this rule. Scoping is not just noise
				// reduction — the generic status already carries a `roleGroups` field, so an
				// unscoped walk treats `.status` as a role node today and would fire on any product
				// that later adds a `config` alongside it.
				spec, ok := version.Schema.OpenAPIV3Schema.Properties["spec"]
				if !ok {
					continue
				}
				found = append(found, walkForRoles(&spec, ".spec", crd.Name, version.Name, file)...)
			}
		}
	}

	sort.Slice(found, func(i, j int) bool {
		if found[i].CRD != found[j].CRD {
			return found[i].CRD < found[j].CRD
		}
		if found[i].Version != found[j].Version {
			return found[i].Version < found[j].Version
		}
		return found[i].Path < found[j].Path
	})
	return found, nil
}

// resolveCRDFiles expands globs and plain paths into a deduplicated, sorted file list.
func resolveCRDFiles(pathsOrGlobs []string) ([]string, error) {
	if len(pathsOrGlobs) == 0 {
		return nil, fmt.Errorf("no CRD paths given")
	}
	seen := map[string]struct{}{}
	var files []string
	for _, arg := range pathsOrGlobs {
		matches, err := filepath.Glob(arg)
		if err != nil {
			return nil, fmt.Errorf("bad pattern %q: %w", arg, err)
		}
		if len(matches) == 0 {
			// A plain path that does not exist is a typo, and so is a glob that matches nothing.
			// Either way the caller believes something is being checked.
			return nil, fmt.Errorf("%q matched no files", arg)
		}
		for _, m := range matches {
			if _, dup := seen[m]; dup {
				continue
			}
			seen[m] = struct{}{}
			files = append(files, m)
		}
	}
	sort.Strings(files)
	return files, nil
}

// splitYAMLDocuments splits a multi-document YAML file into its documents.
//
// It uses apimachinery's reader rather than splitting on "\n---": a `---` inside a block scalar is
// ordinary text, and CRD descriptions are block scalars carrying whatever a Go doc comment said. A
// naive split cuts such a document in half, the halves fail to parse, and — before this was
// tightened — the CRD was silently skipped, so the guard reported success over a file it never
// read. Line-ending handling comes for free with the same change.
func splitYAMLDocuments(raw []byte) ([][]byte, error) {
	reader := k8syaml.NewYAMLReader(bufio.NewReader(bytes.NewReader(raw)))
	var docs [][]byte
	for {
		doc, err := reader.Read()
		if errors.Is(err, io.EOF) {
			return docs, nil
		}
		if err != nil {
			return nil, err
		}
		if len(bytes.TrimSpace(doc)) > 0 {
			docs = append(docs, doc)
		}
	}
}

// walkForRoles descends the schema looking for role nodes — those declaring a `roleGroups`
// property — and reports the defaults inside the `config` blocks they own.
func walkForRoles(
	schema *apiextensionsv1.JSONSchemaProps,
	path, crd, version, file string,
) []InheritedConfigDefault {
	if schema == nil {
		return nil
	}

	var found []InheritedConfigDefault
	if roleGroups, isRole := schema.Properties["roleGroups"]; isRole {
		if roleConfig, ok := schema.Properties["config"]; ok {
			found = append(found,
				collectDefaults(&roleConfig, path+".config", crd, version, file)...)
		}
		// roleGroups is a map, so the per-group schema hangs off additionalProperties.
		if group := mapValueSchema(&roleGroups); group != nil {
			if groupConfig, ok := group.Properties["config"]; ok {
				found = append(found,
					collectDefaults(&groupConfig, path+".roleGroups[*].config", crd, version, file)...)
			}
		}
	}

	for _, child := range sortedProperties(schema) {
		sub := schema.Properties[child]
		found = append(found, walkForRoles(&sub, path+"."+child, crd, version, file)...)
	}
	if value := mapValueSchema(schema); value != nil {
		found = append(found, walkForRoles(value, path+"[*]", crd, version, file)...)
	}
	if schema.Items != nil && schema.Items.Schema != nil {
		found = append(found, walkForRoles(schema.Items.Schema, path+"[]", crd, version, file)...)
	}
	return found
}

// collectDefaults reports every `default` at or below a folded config subtree.
func collectDefaults(
	schema *apiextensionsv1.JSONSchemaProps,
	path, crd, version, file string,
) []InheritedConfigDefault {
	if schema == nil {
		return nil
	}

	var found []InheritedConfigDefault
	if schema.Default != nil {
		found = append(found, InheritedConfigDefault{
			CRD:     crd,
			Version: version,
			Path:    path,
			Default: strings.TrimSpace(string(schema.Default.Raw)),
			File:    file,
		})
	}
	for _, child := range sortedProperties(schema) {
		sub := schema.Properties[child]
		found = append(found, collectDefaults(&sub, path+"."+child, crd, version, file)...)
	}
	if value := mapValueSchema(schema); value != nil {
		found = append(found, collectDefaults(value, path+"[*]", crd, version, file)...)
	}
	if schema.Items != nil && schema.Items.Schema != nil {
		found = append(found, collectDefaults(schema.Items.Schema, path+"[]", crd, version, file)...)
	}
	return found
}

// mapValueSchema returns the schema of a map's values, or nil when the node is not a typed map.
// `additionalProperties` carries either a bool or a schema; only the schema form has children.
func mapValueSchema(schema *apiextensionsv1.JSONSchemaProps) *apiextensionsv1.JSONSchemaProps {
	if schema.AdditionalProperties == nil || schema.AdditionalProperties.Schema == nil {
		return nil
	}
	return schema.AdditionalProperties.Schema
}

// sortedProperties returns a node's property names in a stable order, so a failure message does not
// reshuffle between runs of the same unchanged CRD.
func sortedProperties(schema *apiextensionsv1.JSONSchemaProps) []string {
	names := make([]string, 0, len(schema.Properties))
	for name := range schema.Properties {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// HaveNoInheritedConfigDefaults asserts that no CRD declares a `+kubebuilder:default` inside a role
// or role group `config` block. The actual value is a path or glob, or a []string of them:
//
//	It("declares no CRD default inside a role config block", func() {
//		Expect("config/crd/bases/*.yaml").To(testutil.HaveNoInheritedConfigDefaults())
//	})
//
// Run it against the CRDs `make manifests` generates, in any operator built on this SDK. It needs
// no envtest, no CR fixture and no cluster — the defect is visible in the generated schema.
func HaveNoInheritedConfigDefaults() types.GomegaMatcher {
	return &inheritedConfigDefaultsMatcher{}
}

type inheritedConfigDefaultsMatcher struct {
	found []InheritedConfigDefault
}

func (m *inheritedConfigDefaultsMatcher) Match(actual any) (bool, error) {
	var paths []string
	switch v := actual.(type) {
	case string:
		paths = []string{v}
	case []string:
		paths = v
	default:
		return false, fmt.Errorf(
			"HaveNoInheritedConfigDefaults expects a path/glob string or []string, got %T", actual)
	}

	found, err := FindInheritedConfigDefaults(paths...)
	if err != nil {
		return false, err
	}
	m.found = found
	return len(found) == 0, nil
}

func (m *inheritedConfigDefaultsMatcher) FailureMessage(actual any) string {
	lines := make([]string, 0, len(m.found))
	for _, f := range m.found {
		lines = append(lines, "  "+f.String())
	}
	return fmt.Sprintf(`Expected no CRD default inside a role or role group "config" block, found %d:
%s

A "config" block is folded Role -> RoleGroup, and structural defaulting fills a leaf as soon as its
enclosing object exists. So each default above lands in every role group that declared the enclosing
object for any reason, making "the group did not set this" and "the group asked for the default"
indistinguishable — the role's value can never win.

Remove the +kubebuilder:default and apply the value where it is consumed instead, e.g. a
GetX() accessor that substitutes the default when the field is unset (see
commonsv1alpha1.StorageResource.GetCapacity).`, len(m.found), strings.Join(lines, "\n"))
}

func (m *inheritedConfigDefaultsMatcher) NegatedFailureMessage(actual any) string {
	return "Expected at least one CRD default inside a role or role group \"config\" block, found none"
}
