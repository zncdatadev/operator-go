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

package testutil_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/testutil"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

// The fixtures below are built as typed schemas and marshalled, rather than written as YAML
// literals: a CRD's `config` block sits ten levels deep, and hand-indented YAML at that depth
// silently produces a schema that means something else — which is how the first draft of these
// specs "passed" against a fixture the finder correctly found nothing in.

func str(v string) *apiextensionsv1.JSON { return &apiextensionsv1.JSON{Raw: []byte(`"` + v + `"`)} }

func object(props map[string]apiextensionsv1.JSONSchemaProps) apiextensionsv1.JSONSchemaProps {
	return apiextensionsv1.JSONSchemaProps{Type: "object", Properties: props}
}

func mapOf(value apiextensionsv1.JSONSchemaProps) apiextensionsv1.JSONSchemaProps {
	return apiextensionsv1.JSONSchemaProps{
		Type:                 "object",
		AdditionalProperties: &apiextensionsv1.JSONSchemaPropsOrBool{Schema: &value},
	}
}

// role returns a role node: a `config` block plus the `roleGroups` map carrying the same block,
// which is what makes it a role as far as the finder is concerned.
func role(configProps map[string]apiextensionsv1.JSONSchemaProps) apiextensionsv1.JSONSchemaProps {
	return object(map[string]apiextensionsv1.JSONSchemaProps{
		"config":     object(configProps),
		"roleGroups": mapOf(object(map[string]apiextensionsv1.JSONSchemaProps{"config": object(configProps)})),
	})
}

// writeCRD marshals a CRD whose `spec` carries the given properties, and returns a glob for it.
func writeCRD(specProps map[string]apiextensionsv1.JSONSchemaProps) string {
	return writeCRDWithStatus(specProps, nil)
}

// writeCRDWithStatus is writeCRD plus a `status` schema, for proving what is out of scope.
func writeCRDWithStatus(
	specProps, statusProps map[string]apiextensionsv1.JSONSchemaProps,
) string {
	// Every fixture carries a legitimate default OUTSIDE any config block, so a spec asserting
	// "found 2" also proves this one was not counted.
	specProps["image"] = object(map[string]apiextensionsv1.JSONSchemaProps{
		"pullPolicy": {Type: "string", Default: str("IfNotPresent")},
	})

	root := map[string]apiextensionsv1.JSONSchemaProps{"spec": object(specProps)}
	if statusProps != nil {
		root["status"] = object(statusProps)
	}

	crd := &apiextensionsv1.CustomResourceDefinition{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "apiextensions.k8s.io/v1",
			Kind:       "CustomResourceDefinition",
		},
		ObjectMeta: metav1.ObjectMeta{Name: "probes.test.kubedoop.dev"},
		Spec: apiextensionsv1.CustomResourceDefinitionSpec{
			Group: "test.kubedoop.dev",
			Names: apiextensionsv1.CustomResourceDefinitionNames{Kind: "Probe", Plural: "probes"},
			Scope: apiextensionsv1.NamespaceScoped,
			Versions: []apiextensionsv1.CustomResourceDefinitionVersion{{
				Name: "v1alpha1", Served: true, Storage: true,
				Schema: &apiextensionsv1.CustomResourceValidation{
					OpenAPIV3Schema: &apiextensionsv1.JSONSchemaProps{
						Type:       "object",
						Properties: root,
					},
				},
			}},
		},
	}

	raw, err := yaml.Marshal(crd)
	Expect(err).NotTo(HaveOccurred())
	dir := GinkgoT().TempDir()
	Expect(os.WriteFile(filepath.Join(dir, "crd.yaml"), raw, 0o600)).To(Succeed())
	return filepath.Join(dir, "*.yaml")
}

var _ = Describe("FindInheritedConfigDefaults", func() {
	// A leaf directly under `config`: any role group that writes `config:` for ANY reason gets the
	// default stamped in, so the role's value can never win. This is the shape trino-operator
	// carries on queryMaxMemory today.
	leafUnderConfig := func() map[string]apiextensionsv1.JSONSchemaProps {
		return map[string]apiextensionsv1.JSONSchemaProps{
			"queryMaxMemory": {Type: "string", Default: str("5GB")},
		}
	}

	// Two levels down, under an object a role group only declares deliberately. Reported all the
	// same: LogLevelSpec.Level lived exactly here and was a live defect for months — `console: {}`
	// in a group beat a role asking for DEBUG. Depth makes the failure rarer, not safer.
	nestedUnderConfig := func() map[string]apiextensionsv1.JSONSchemaProps {
		return map[string]apiextensionsv1.JSONSchemaProps{
			"logging": object(map[string]apiextensionsv1.JSONSchemaProps{
				"containers": mapOf(object(map[string]apiextensionsv1.JSONSchemaProps{
					"console": object(map[string]apiextensionsv1.JSONSchemaProps{
						"level": {Type: "string", Default: str("INFO")},
					}),
				})),
			}),
		}
	}

	It("flags a default on a leaf directly under config", func() {
		found, err := testutil.FindInheritedConfigDefaults(writeCRD(
			map[string]apiextensionsv1.JSONSchemaProps{"roles": mapOf(role(leafUnderConfig()))}))
		Expect(err).NotTo(HaveOccurred())

		Expect(found).To(HaveLen(2), "once at the role level and once at the role group level")
		Expect([]string{found[0].Path, found[1].Path}).To(ConsistOf(
			".spec.roles[*].config.queryMaxMemory",
			".spec.roles[*].roleGroups[*].config.queryMaxMemory",
		))
		Expect(found[0].Default).To(Equal(`"5GB"`))
		Expect(found[0].CRD).To(Equal("probes.test.kubedoop.dev"))
		Expect(found[0].Version).To(Equal("v1alpha1"))
	})

	It("flags a default nested deep inside config, with no depth heuristic", func() {
		found, err := testutil.FindInheritedConfigDefaults(writeCRD(
			map[string]apiextensionsv1.JSONSchemaProps{"roles": mapOf(role(nestedUnderConfig()))}))
		Expect(err).NotTo(HaveOccurred())

		Expect(found).To(HaveLen(2))
		Expect([]string{found[0].Path, found[1].Path}).To(ConsistOf(
			".spec.roles[*].config.logging.containers[*].console.level",
			".spec.roles[*].roleGroups[*].config.logging.containers[*].console.level",
		))
	})

	It("finds roles a product flattened into named fields", func() {
		// Detection keys on a node declaring `roleGroups`, not on a literal `spec.roles` path, so a
		// product whose CRD says `spec.coordinators` / `spec.workers` is covered too — which is the
		// shape examples/trino-operator actually generates.
		found, err := testutil.FindInheritedConfigDefaults(writeCRD(
			map[string]apiextensionsv1.JSONSchemaProps{"coordinators": role(leafUnderConfig())}))
		Expect(err).NotTo(HaveOccurred())

		Expect([]string{found[0].Path, found[1].Path}).To(ConsistOf(
			".spec.coordinators.config.queryMaxMemory",
			".spec.coordinators.roleGroups[*].config.queryMaxMemory",
		))
	})

	It("ignores defaults outside a config block", func() {
		// Every fixture also carries `spec.image.pullPolicy: IfNotPresent`, a perfectly legitimate
		// default. Flagging it would make the guard unusable — nothing outside the folded block is
		// subject to the Role -> RoleGroup ambiguity.
		found, err := testutil.FindInheritedConfigDefaults(writeCRD(
			map[string]apiextensionsv1.JSONSchemaProps{"roles": mapOf(role(leafUnderConfig()))}))
		Expect(err).NotTo(HaveOccurred())
		for _, f := range found {
			Expect(f.Path).NotTo(ContainSubstring("image"))
		}
	})

	It("returns nothing for a clean CRD", func() {
		found, err := testutil.FindInheritedConfigDefaults(writeCRD(
			map[string]apiextensionsv1.JSONSchemaProps{"roles": mapOf(role(
				map[string]apiextensionsv1.JSONSchemaProps{"queryMaxMemory": {Type: "string"}}))}))
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeEmpty())
	})

	It("ignores a roleGroups/config shape under status", func() {
		// The contract covers the DESIRED state the framework folds Role -> RoleGroup. A `config`
		// under `.status` is written by the operator and never merged, so a default there means
		// nothing to this rule. Not hypothetical: GenericClusterStatus already carries a
		// `roleGroups` field, so an unscoped walk treats `.status` as a role node.
		found, err := testutil.FindInheritedConfigDefaults(writeCRDWithStatus(
			map[string]apiextensionsv1.JSONSchemaProps{"roles": mapOf(role(
				map[string]apiextensionsv1.JSONSchemaProps{"queryMaxMemory": {Type: "string"}}))},
			map[string]apiextensionsv1.JSONSchemaProps{"roles": mapOf(role(
				map[string]apiextensionsv1.JSONSchemaProps{
					"observed": {Type: "string", Default: str("whatever")},
				}))},
		))
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeEmpty())
	})

	It("still reads a CRD whose description contains a YAML document separator", func() {
		// A `---` inside a block scalar is ordinary text, and CRD descriptions are block scalars
		// carrying whatever a Go doc comment said. Splitting the file on "\n---" cuts such a
		// document in half; the halves fail to parse, and a skipped CRD makes the guard report
		// success over a file it never read — the exact failure mode this package exists to
		// prevent.
		props := map[string]apiextensionsv1.JSONSchemaProps{
			"queryMaxMemory": {
				Type:        "string",
				Default:     str("5GB"),
				Description: "Query memory cap.\n\n---\n\nSee the tuning guide.",
			},
		}
		found, err := testutil.FindInheritedConfigDefaults(writeCRD(
			map[string]apiextensionsv1.JSONSchemaProps{"roles": mapOf(role(props))}))
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(HaveLen(2), "the document must survive splitting")
	})

	It("errors on a malformed CustomResourceDefinition instead of skipping it", func() {
		dir := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(dir, "crd.yaml"),
			[]byte("apiVersion: apiextensions.k8s.io/v1\nkind: CustomResourceDefinition\nspec: [this is not a spec]\n"),
			0o600)).To(Succeed())

		_, err := testutil.FindInheritedConfigDefaults(filepath.Join(dir, "*.yaml"))
		Expect(err).To(MatchError(ContainSubstring("CustomResourceDefinition")))
	})

	It("errors when the arguments match no files", func() {
		// The failure mode that matters most for a guard: passing because it inspected nothing.
		// A typo in the glob must be loud, not green.
		_, err := testutil.FindInheritedConfigDefaults(filepath.Join(GinkgoT().TempDir(), "*.yaml"))
		Expect(err).To(MatchError(ContainSubstring("matched no files")))
	})

	It("skips documents that are not CRDs", func() {
		glob := writeCRD(map[string]apiextensionsv1.JSONSchemaProps{"roles": mapOf(role(leafUnderConfig()))})
		Expect(os.WriteFile(filepath.Join(filepath.Dir(glob), "kustomization.yaml"),
			[]byte("apiVersion: kustomize.config.k8s.io/v1beta1\nkind: Kustomization\nresources:\n- crd.yaml\n"),
			0o600)).To(Succeed())

		found, err := testutil.FindInheritedConfigDefaults(glob)
		Expect(err).NotTo(HaveOccurred(), "a glob over a real crd/bases directory catches these")
		Expect(found).To(HaveLen(2))
	})
})

var _ = Describe("HaveNoInheritedConfigDefaults", func() {
	It("fails with a message naming every offending path", func() {
		glob := writeCRD(map[string]apiextensionsv1.JSONSchemaProps{
			"roles": mapOf(role(map[string]apiextensionsv1.JSONSchemaProps{
				"queryMaxMemory": {Type: "string", Default: str("5GB")},
			})),
		})

		matcher := testutil.HaveNoInheritedConfigDefaults()
		ok, err := matcher.Match(glob)
		Expect(err).NotTo(HaveOccurred())
		Expect(ok).To(BeFalse())

		msg := matcher.FailureMessage(glob)
		Expect(msg).To(ContainSubstring(".spec.roles[*].roleGroups[*].config.queryMaxMemory"))
		Expect(msg).To(ContainSubstring("consumed"), "the message must say where the default belongs")
	})

	It("rejects an actual value that is not a path or list of paths", func() {
		_, err := testutil.HaveNoInheritedConfigDefaults().Match(42)
		Expect(err).To(MatchError(ContainSubstring("path/glob string or []string")))
	})

	It("accepts a []string of paths", func() {
		clean := writeCRD(map[string]apiextensionsv1.JSONSchemaProps{
			"roles": mapOf(role(map[string]apiextensionsv1.JSONSchemaProps{
				"queryMaxMemory": {Type: "string"},
			})),
		})
		Expect([]string{clean}).To(testutil.HaveNoInheritedConfigDefaults())
	})

	// The live guard over this repository's own generated CRDs. It is the same three lines a
	// product operator writes, and it is what stops the SDK reintroducing the defect it just spent
	// two changes removing (#544 for resources, #573 for logging).
	It("holds for this repository's generated CRDs", func() {
		Expect("../../config/crd/bases/*.yaml").To(testutil.HaveNoInheritedConfigDefaults())
	})
})
