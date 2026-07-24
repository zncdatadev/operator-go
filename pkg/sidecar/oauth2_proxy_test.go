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

package sidecar_test

import (
	"encoding/base64"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/authentication/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/sidecar"
	corev1 "k8s.io/api/core/v1"
)

func keycloakProvider() *authv1alpha1.OIDCProvider {
	return &authv1alpha1.OIDCProvider{
		Hostname:     "keycloak.test-ns.svc.cluster.local",
		Port:         8080,
		RootPath:     "/realms/kubedoop",
		ProviderHint: "keycloak",
		Scopes:       []string{"openid", "email", "profile"},
	}
}

func envByName(container *corev1.Container) map[string]corev1.EnvVar {
	out := map[string]corev1.EnvVar{}
	for _, e := range container.Env {
		out[e.Name] = e
	}
	return out
}

var _ = Describe("OAuth2ProxySidecarProvider", func() {
	var podSpec *corev1.PodSpec

	BeforeEach(func() {
		podSpec = &corev1.PodSpec{
			Containers: []corev1.Container{{Name: "node"}},
		}
	})

	It("injects a native sidecar with the full OAUTH2_PROXY env set", func() {
		provider := sidecar.NewOAuth2ProxySidecarProvider(
			keycloakProvider(), "oidc-credentials", 18080, "cr-uid")

		Expect(provider.Inject(podSpec, nil)).To(Succeed())

		Expect(podSpec.InitContainers).To(HaveLen(1))
		container := &podSpec.InitContainers[0]
		Expect(container.Name).To(Equal(sidecar.OAuth2ProxySidecarName))
		Expect(container.Image).To(Equal(sidecar.DefaultOAuth2ProxyImage))
		Expect(container.RestartPolicy).NotTo(BeNil())
		Expect(*container.RestartPolicy).To(Equal(corev1.ContainerRestartPolicyAlways))

		envs := envByName(container)
		Expect(envs["OAUTH2_PROXY_OIDC_ISSUER_URL"].Value).To(
			Equal("http://keycloak.test-ns.svc.cluster.local:8080/realms/kubedoop"))
		Expect(envs["OAUTH2_PROXY_PROVIDER"].Value).To(Equal("keycloak-oidc"))
		Expect(envs["OAUTH2_PROXY_SCOPE"].Value).To(Equal("openid email profile"))
		Expect(envs["OAUTH2_PROXY_UPSTREAMS"].Value).To(Equal("http://localhost:18080"))
		Expect(envs["OAUTH2_PROXY_HTTP_ADDRESS"].Value).To(Equal("0.0.0.0:4180"))
		Expect(envs["OAUTH2_PROXY_COOKIE_SECURE"].Value).To(Equal("false"))
		Expect(envs["OAUTH2_PROXY_CODE_CHALLENGE_METHOD"].Value).To(Equal("S256"))
		Expect(envs["OAUTH2_PROXY_EMAIL_DOMAINS"].Value).To(Equal("*"))
		Expect(envs["OAUTH2_PROXY_WHITELIST_DOMAINS"].Value).To(Equal("*"))

		Expect(envs["OAUTH2_PROXY_CLIENT_ID"].ValueFrom.SecretKeyRef.Name).To(Equal("oidc-credentials"))
		Expect(envs["OAUTH2_PROXY_CLIENT_ID"].ValueFrom.SecretKeyRef.Key).To(Equal("CLIENT_ID"))
		Expect(envs["OAUTH2_PROXY_CLIENT_SECRET"].ValueFrom.SecretKeyRef.Key).To(Equal("CLIENT_SECRET"))

		Expect(container.Ports).To(HaveLen(1))
		Expect(container.Ports[0].ContainerPort).To(Equal(int32(4180)))
	})

	It("is idempotent", func() {
		provider := sidecar.NewOAuth2ProxySidecarProvider(
			keycloakProvider(), "oidc-credentials", 18080, "cr-uid")

		Expect(provider.Inject(podSpec, nil)).To(Succeed())
		Expect(provider.Inject(podSpec, nil)).To(Succeed())
		Expect(podSpec.InitContainers).To(HaveLen(1))
	})

	It("appends extra scopes after the provider scopes", func() {
		provider := sidecar.NewOAuth2ProxySidecarProvider(
			keycloakProvider(), "oidc-credentials", 18080, "cr-uid",
			sidecar.WithOAuth2ProxyExtraScopes("groups"))

		Expect(provider.Inject(podSpec, nil)).To(Succeed())
		envs := envByName(&podSpec.InitContainers[0])
		Expect(envs["OAUTH2_PROXY_SCOPE"].Value).To(Equal("openid email profile groups"))
	})

	It("falls back to the default scopes when the provider declares none", func() {
		oidc := keycloakProvider()
		oidc.Scopes = nil
		provider := sidecar.NewOAuth2ProxySidecarProvider(oidc, "oidc-credentials", 18080, "cr-uid")

		Expect(provider.Inject(podSpec, nil)).To(Succeed())
		envs := envByName(&podSpec.InitContainers[0])
		Expect(envs["OAUTH2_PROXY_SCOPE"].Value).To(Equal("openid email profile"))
	})

	It("honors port and image overrides", func() {
		provider := sidecar.NewOAuth2ProxySidecarProvider(
			keycloakProvider(), "oidc-credentials", 8080, "cr-uid",
			sidecar.WithOAuth2ProxyPort(9999))

		Expect(provider.Inject(podSpec, &sidecar.SidecarConfig{Image: "custom/oauth2-proxy:v0.0.1"})).To(Succeed())
		container := &podSpec.InitContainers[0]
		Expect(container.Image).To(Equal("custom/oauth2-proxy:v0.0.1"))
		envs := envByName(container)
		Expect(envs["OAUTH2_PROXY_HTTP_ADDRESS"].Value).To(Equal("0.0.0.0:9999"))
		Expect(container.Ports[0].ContainerPort).To(Equal(int32(9999)))
	})
})

var _ = Describe("OIDCIssuerURL", func() {
	It("includes a non-default port", func() {
		Expect(sidecar.OIDCIssuerURL(keycloakProvider())).To(
			Equal("http://keycloak.test-ns.svc.cluster.local:8080/realms/kubedoop"))
	})

	It("omits the scheme-default port", func() {
		oidc := keycloakProvider()
		oidc.Port = 80
		Expect(sidecar.OIDCIssuerURL(oidc)).To(
			Equal("http://keycloak.test-ns.svc.cluster.local/realms/kubedoop"))
	})

	It("uses https when the provider declares TLS", func() {
		oidc := keycloakProvider()
		oidc.TLS = &authv1alpha1.OIDCTls{}
		oidc.Port = 443
		Expect(sidecar.OIDCIssuerURL(oidc)).To(
			Equal("https://keycloak.test-ns.svc.cluster.local/realms/kubedoop"))
	})
})

var _ = Describe("OAuth2ProxyProviderFor", func() {
	It("maps keycloak to keycloak-oidc", func() {
		Expect(sidecar.OAuth2ProxyProviderFor("keycloak")).To(Equal("keycloak-oidc"))
	})
	It("defaults an empty hint to the generic oidc provider", func() {
		Expect(sidecar.OAuth2ProxyProviderFor("")).To(Equal("oidc"))
	})
	It("passes other hints through", func() {
		Expect(sidecar.OAuth2ProxyProviderFor("oidc")).To(Equal("oidc"))
	})
})

var _ = Describe("DeterministicCookieSecret", func() {
	It("is stable and URL-safe-decodes to 32 bytes, as oauth2-proxy requires", func() {
		first := sidecar.DeterministicCookieSecret("cr-uid")
		second := sidecar.DeterministicCookieSecret("cr-uid")
		Expect(first).To(Equal(second))

		// oauth2-proxy's SecretBytes attempts ONLY unpadded URL-safe base64; a value with
		// '+' or '/' would fall back to the raw string (invalid length) and crash the proxy.
		Expect(first).NotTo(ContainSubstring("+"))
		Expect(first).NotTo(ContainSubstring("/"))
		Expect(first).NotTo(ContainSubstring("="))
		raw, err := base64.RawURLEncoding.DecodeString(first)
		Expect(err).NotTo(HaveOccurred())
		Expect(raw).To(HaveLen(32))
	})

	It("stays URL-safe across many seeds", func() {
		for i := 0; i < 256; i++ {
			secret := sidecar.DeterministicCookieSecret(fmt.Sprintf("seed-%d", i))
			_, err := base64.RawURLEncoding.DecodeString(secret)
			Expect(err).NotTo(HaveOccurred(), "seed-%d", i)
		}
	})

	It("differs per seed", func() {
		Expect(sidecar.DeterministicCookieSecret("a")).NotTo(
			Equal(sidecar.DeterministicCookieSecret("b")))
	})
})

var _ = Describe("OAuth2ProxySidecarProvider readiness coupling", func() {
	It("injects no readiness probe (an unready native sidecar would gate the whole pod)", func() {
		podSpec := &corev1.PodSpec{Containers: []corev1.Container{{Name: "node"}}}
		provider := sidecar.NewOAuth2ProxySidecarProvider(
			keycloakProvider(), "oidc-credentials", 18080, "cr-uid")

		Expect(provider.Inject(podSpec, nil)).To(Succeed())
		Expect(podSpec.InitContainers[0].ReadinessProbe).To(BeNil())
	})

	It("rejects injection without an OIDC provider", func() {
		podSpec := &corev1.PodSpec{Containers: []corev1.Container{{Name: "node"}}}
		provider := sidecar.NewOAuth2ProxySidecarProvider(nil, "oidc-credentials", 18080, "cr-uid")
		Expect(provider.Inject(podSpec, nil)).To(MatchError(ContainSubstring("OIDC provider is required")))
	})
})
