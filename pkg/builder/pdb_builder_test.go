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

package builder_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/zncdatadev/operator-go/pkg/builder"
	"k8s.io/apimachinery/pkg/util/intstr"
)

var _ = Describe("PDBBuilder", func() {
	const (
		name      = "test-pdb"
		namespace = "test-namespace"
	)

	var pdbBuilder *builder.PDBBuilder

	BeforeEach(func() {
		pdbBuilder = builder.NewPDBBuilder(name, namespace)
	})

	Describe("NewPDBBuilder", func() {
		It("should create a builder with default values", func() {
			Expect(pdbBuilder.Name).To(Equal(name))
			Expect(pdbBuilder.Namespace).To(Equal(namespace))
			Expect(pdbBuilder.Enabled).To(BeTrue())
		})
	})

	Describe("WithLabels", func() {
		It("should add labels to the builder", func() {
			labels := map[string]string{"app": "test"}
			result := pdbBuilder.WithLabels(labels)

			Expect(result).To(Equal(pdbBuilder))
			Expect(pdbBuilder.Labels).To(HaveKeyWithValue("app", "test"))
		})
	})

	Describe("WithAnnotations", func() {
		It("should add annotations to the builder", func() {
			annotations := map[string]string{"description": "test"}
			result := pdbBuilder.WithAnnotations(annotations)

			Expect(result).To(Equal(pdbBuilder))
			Expect(pdbBuilder.Annotations).To(HaveKeyWithValue("description", "test"))
		})
	})

	Describe("WithSelector", func() {
		It("should set the selector", func() {
			selector := map[string]string{"app": "test-app"}
			result := pdbBuilder.WithSelector(selector)

			Expect(result).To(Equal(pdbBuilder))
			Expect(pdbBuilder.Selector).To(HaveKeyWithValue("app", "test-app"))
		})
	})

	Describe("WithMaxUnavailable", func() {
		It("should set the max unavailable", func() {
			max := intstr.FromInt(1)
			result := pdbBuilder.WithMaxUnavailable(max)

			Expect(result).To(Equal(pdbBuilder))
			Expect(*pdbBuilder.MaxUnavailable).To(Equal(max))
		})
	})

	Describe("WithEnabled", func() {
		It("should set enabled to false", func() {
			result := pdbBuilder.WithEnabled(false)

			Expect(result).To(Equal(pdbBuilder))
			Expect(pdbBuilder.Enabled).To(BeFalse())
		})
	})

	Describe("Build", func() {
		It("should build a valid PDB with default maxUnavailable", func() {
			pdb := pdbBuilder.
				WithLabels(map[string]string{"app": "test"}).
				WithSelector(map[string]string{"app": "test-app"}).
				Build()

			Expect(pdb).NotTo(BeNil())
			Expect(pdb.Name).To(Equal(name))
			Expect(pdb.Namespace).To(Equal(namespace))
			Expect(pdb.Spec.MaxUnavailable).NotTo(BeNil())
			Expect(pdb.Spec.Selector.MatchLabels).To(HaveKeyWithValue("app", "test-app"))
		})

		It("should build a PDB with custom maxUnavailable", func() {
			max := intstr.FromInt(2)
			pdb := pdbBuilder.
				WithMaxUnavailable(max).
				Build()

			Expect(pdb.Spec.MaxUnavailable).NotTo(BeNil())
			Expect(*pdb.Spec.MaxUnavailable).To(Equal(max))
		})
	})

	Describe("IsEnabled", func() {
		It("should return true when enabled", func() {
			Expect(pdbBuilder.IsEnabled()).To(BeTrue())
		})

		It("should return false when disabled", func() {
			pdbBuilder.WithEnabled(false)
			Expect(pdbBuilder.IsEnabled()).To(BeFalse())
		})
	})

	Describe("NamespacedName", func() {
		It("should return the correct NamespacedName", func() {
			nn := pdbBuilder.NamespacedName()

			Expect(nn.Name).To(Equal(name))
			Expect(nn.Namespace).To(Equal(namespace))
		})
	})
})
