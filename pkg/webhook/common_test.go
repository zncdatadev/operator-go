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

package webhook_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"

	authv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/authentication/v1alpha1"
	commonsv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/commons/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/webhook"
)

var _ = Describe("ValidateGenericClusterSpec", func() {
	It("should return no errors for nil spec", func() {
		errs := webhook.ValidateGenericClusterSpec(nil, field.NewPath("spec"))
		Expect(errs).To(BeEmpty())
	})

	It("should return no errors for valid spec with Custom image", func() {
		spec := &commonsv1alpha1.GenericClusterSpec{
			Image: &commonsv1alpha1.ImageSpec{
				Custom: "quay.io/zncdatadev/hdfs:3.3.6",
			},
		}
		errs := webhook.ValidateGenericClusterSpec(spec, field.NewPath("spec"))
		Expect(errs).To(BeEmpty())
	})

	It("should return no errors for valid non-custom image", func() {
		spec := &commonsv1alpha1.GenericClusterSpec{
			Image: &commonsv1alpha1.ImageSpec{
				Repo:            "quay.io/zncdatadev",
				ProductVersion:  "3.3.6",
				KubedoopVersion: "0.2.0",
				PullPolicy:      corev1.PullIfNotPresent,
			},
		}
		errs := webhook.ValidateGenericClusterSpec(spec, field.NewPath("spec"))
		Expect(errs).To(BeEmpty())
	})

	It("should report error when productVersion is empty without Custom", func() {
		spec := &commonsv1alpha1.GenericClusterSpec{
			Image: &commonsv1alpha1.ImageSpec{
				Repo:            "quay.io/zncdatadev",
				KubedoopVersion: "0.2.0",
			},
		}
		errs := webhook.ValidateGenericClusterSpec(spec, field.NewPath("spec"))
		Expect(errs).NotTo(BeEmpty())
		Expect(errs[0].Field).To(ContainSubstring("productVersion"))
	})

	It("should report error when kubedoopVersion is empty without Custom", func() {
		spec := &commonsv1alpha1.GenericClusterSpec{
			Image: &commonsv1alpha1.ImageSpec{
				Repo:           "quay.io/zncdatadev",
				ProductVersion: "3.3.6",
			},
		}
		errs := webhook.ValidateGenericClusterSpec(spec, field.NewPath("spec"))
		Expect(errs).NotTo(BeEmpty())
		Expect(errs[0].Field).To(ContainSubstring("kubedoopVersion"))
	})

	It("should report error when repo is empty without Custom", func() {
		spec := &commonsv1alpha1.GenericClusterSpec{
			Image: &commonsv1alpha1.ImageSpec{
				ProductVersion:  "3.3.6",
				KubedoopVersion: "0.2.0",
			},
		}
		errs := webhook.ValidateGenericClusterSpec(spec, field.NewPath("spec"))
		Expect(errs).NotTo(BeEmpty())
		Expect(errs[0].Field).To(ContainSubstring("repo"))
	})

	It("should report error for invalid pullPolicy", func() {
		spec := &commonsv1alpha1.GenericClusterSpec{
			Image: &commonsv1alpha1.ImageSpec{
				Repo:            "quay.io/zncdatadev",
				ProductVersion:  "3.3.6",
				KubedoopVersion: "0.2.0",
				PullPolicy:      corev1.PullPolicy("Invalid"),
			},
		}
		errs := webhook.ValidateGenericClusterSpec(spec, field.NewPath("spec"))
		Expect(errs).NotTo(BeEmpty())
		Expect(errs[0].Field).To(ContainSubstring("pullPolicy"))
	})
})

var _ = Describe("DefaultGenericClusterSpec", func() {
	It("should do nothing when spec is nil", func() {
		// Should not panic
		webhook.DefaultGenericClusterSpec(nil, nil)
	})

	It("should set default image when spec.Image is nil", func() {
		spec := &commonsv1alpha1.GenericClusterSpec{}
		defaultImage := &commonsv1alpha1.ImageSpec{
			Repo:            "quay.io/zncdatadev",
			ProductVersion:  "3.3.6",
			KubedoopVersion: "0.2.0",
		}
		webhook.DefaultGenericClusterSpec(spec, defaultImage)
		Expect(spec.Image).NotTo(BeNil())
		Expect(spec.Image.Repo).To(Equal("quay.io/zncdatadev"))
		Expect(spec.Image.ProductVersion).To(Equal("3.3.6"))
	})

	It("should not overwrite existing spec.Image", func() {
		existing := &commonsv1alpha1.ImageSpec{
			Custom: "my-registry.io/hdfs:latest",
		}
		spec := &commonsv1alpha1.GenericClusterSpec{Image: existing}
		defaultImage := &commonsv1alpha1.ImageSpec{
			ProductVersion: "3.3.6",
		}
		webhook.DefaultGenericClusterSpec(spec, defaultImage)
		Expect(spec.Image.Custom).To(Equal("my-registry.io/hdfs:latest"))
		Expect(spec.Image.ProductVersion).To(BeEmpty()) // original preserved
	})

	It("should set IfNotPresent when PullPolicy is empty", func() {
		spec := &commonsv1alpha1.GenericClusterSpec{
			Image: &commonsv1alpha1.ImageSpec{
				Custom: "quay.io/foo/bar:latest",
			},
		}
		webhook.DefaultGenericClusterSpec(spec, nil)
		Expect(spec.Image.PullPolicy).To(Equal(corev1.PullIfNotPresent))
	})

	It("should not overwrite explicitly set PullPolicy", func() {
		spec := &commonsv1alpha1.GenericClusterSpec{
			Image: &commonsv1alpha1.ImageSpec{
				Custom:     "quay.io/foo/bar:latest",
				PullPolicy: corev1.PullAlways,
			},
		}
		webhook.DefaultGenericClusterSpec(spec, nil)
		Expect(spec.Image.PullPolicy).To(Equal(corev1.PullAlways))
	})
})

var _ = Describe("ValidateOneOf", func() {
	fldPath := field.NewPath("spec").Child("provider")

	It("should return no errors when exactly one field is set", func() {
		errs := webhook.ValidateOneOf(fldPath, map[string]any{
			"alpha": &struct{}{},
			"beta":  nil,
			"gamma": nil,
		})
		Expect(errs).To(BeEmpty())
	})

	It("should return a Required error when no fields are set", func() {
		errs := webhook.ValidateOneOf(fldPath, map[string]any{
			"alpha": nil,
			"beta":  nil,
		})
		Expect(errs).To(HaveLen(1))
		Expect(errs[0].Type).To(Equal(field.ErrorTypeRequired))
	})

	It("should return an Invalid error when more than one field is set", func() {
		errs := webhook.ValidateOneOf(fldPath, map[string]any{
			"alpha": &struct{}{},
			"beta":  &struct{}{},
			"gamma": nil,
		})
		Expect(errs).To(HaveLen(1))
		Expect(errs[0].Type).To(Equal(field.ErrorTypeInvalid))
	})
})

var _ = Describe("ValidateAuthenticationProvider", func() {
	fldPath := field.NewPath("spec").Child("provider")

	It("should return nil when provider is nil", func() {
		errs := webhook.ValidateAuthenticationProvider(nil, fldPath)
		Expect(errs).To(BeNil())
	})

	It("should return no errors when exactly one provider is set (OIDC)", func() {
		provider := &authv1alpha1.AuthenticationProvider{
			OIDC: &authv1alpha1.OIDCProvider{
				Hostname:       "keycloak.example.com",
				PrincipalClaim: "sub",
				ProviderHint:   "keycloak",
			},
		}
		errs := webhook.ValidateAuthenticationProvider(provider, fldPath)
		Expect(errs).To(BeEmpty())
	})

	It("should return no errors when exactly one provider is set (TLS)", func() {
		provider := &authv1alpha1.AuthenticationProvider{
			TLS: &authv1alpha1.TLSProvider{},
		}
		errs := webhook.ValidateAuthenticationProvider(provider, fldPath)
		Expect(errs).To(BeEmpty())
	})

	It("should return no errors when exactly one provider is set (Static)", func() {
		provider := &authv1alpha1.AuthenticationProvider{
			Static: &authv1alpha1.StaticProvider{},
		}
		errs := webhook.ValidateAuthenticationProvider(provider, fldPath)
		Expect(errs).To(BeEmpty())
	})

	It("should return no errors when exactly one provider is set (LDAP)", func() {
		provider := &authv1alpha1.AuthenticationProvider{
			LDAP: &authv1alpha1.LDAPProvider{Hostname: "ldap.example.com"},
		}
		errs := webhook.ValidateAuthenticationProvider(provider, fldPath)
		Expect(errs).To(BeEmpty())
	})

	It("should return no errors when exactly one provider is set (Kerberos)", func() {
		provider := &authv1alpha1.AuthenticationProvider{
			Kerberos: &authv1alpha1.KerberosProvider{},
		}
		errs := webhook.ValidateAuthenticationProvider(provider, fldPath)
		Expect(errs).To(BeEmpty())
	})

	It("should return a Required error when no provider is set", func() {
		provider := &authv1alpha1.AuthenticationProvider{}
		errs := webhook.ValidateAuthenticationProvider(provider, fldPath)
		Expect(errs).To(HaveLen(1))
		Expect(errs[0].Type).To(Equal(field.ErrorTypeRequired))
	})

	It("should return an Invalid error when multiple providers are set", func() {
		provider := &authv1alpha1.AuthenticationProvider{
			OIDC: &authv1alpha1.OIDCProvider{
				Hostname:       "keycloak.example.com",
				PrincipalClaim: "sub",
				ProviderHint:   "keycloak",
			},
			TLS: &authv1alpha1.TLSProvider{},
		}
		errs := webhook.ValidateAuthenticationProvider(provider, fldPath)
		Expect(errs).To(HaveLen(1))
		Expect(errs[0].Type).To(Equal(field.ErrorTypeInvalid))
	})
})

var _ = Describe("ValidationErrors.ToStatusError", func() {
	gvk := schema.GroupVersionKind{Group: "test.example.com", Version: "v1", Kind: "TestCluster"}

	It("should return nil when no errors", func() {
		var errs webhook.ValidationErrors
		err := errs.ToStatusError(gvk, "my-cluster")
		Expect(err).ToNot(HaveOccurred())
	})

	It("should return a StatusError when errors exist", func() {
		errs := webhook.ValidationErrors{}
		errs.Add("spec.name", "name is required")
		err := errs.ToStatusError(gvk, "my-cluster")
		Expect(err).To(HaveOccurred())
		Expect(apierrors.IsInvalid(err)).To(BeTrue())
	})

	It("should include field path in the status error", func() {
		errs := webhook.ValidationErrors{}
		errs.AddWithValue("spec.replicas", "must be positive", -1)
		err := errs.ToStatusError(gvk, "my-cluster")
		Expect(err).To(HaveOccurred())
		statusErr, ok := err.(*apierrors.StatusError)
		Expect(ok).To(BeTrue())
		Expect(statusErr.ErrStatus.Message).To(ContainSubstring("spec.replicas"))
	})

	It("should combine multiple errors", func() {
		errs := webhook.ValidationErrors{}
		errs.Add("spec.name", "required")
		errs.Add("spec.image", "invalid format")
		err := errs.ToStatusError(gvk, "my-cluster")
		statusErr, ok := err.(*apierrors.StatusError)
		Expect(ok).To(BeTrue())
		Expect(statusErr.ErrStatus.Details.Causes).To(HaveLen(2))
	})
})
