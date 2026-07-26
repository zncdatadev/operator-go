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
	"context"
	"encoding/base64"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/authentication/v1alpha1"
	"github.com/zncdatadev/operator-go/pkg/sidecar"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
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
			keycloakProvider(), "oidc-credentials", 18080, sidecar.WithOAuth2ProxyAllowAllEmails())

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
		Expect(envs["OAUTH2_PROXY_EMAIL_DOMAINS"].Value).To(Equal("*"),
			"this provider opted into WithOAuth2ProxyAllowAllEmails")

		// Unset, oauth2-proxy permits redirects to its own host only. "*" would make the `rd`
		// parameter an open redirect, so the key must be absent rather than permissive.
		Expect(envs).NotTo(HaveKey("OAUTH2_PROXY_WHITELIST_DOMAINS"))

		Expect(envs["OAUTH2_PROXY_CLIENT_ID"].ValueFrom.SecretKeyRef.Name).To(Equal("oidc-credentials"))
		Expect(envs["OAUTH2_PROXY_CLIENT_ID"].ValueFrom.SecretKeyRef.Key).To(Equal("CLIENT_ID"))
		Expect(envs["OAUTH2_PROXY_CLIENT_SECRET"].ValueFrom.SecretKeyRef.Key).To(Equal("CLIENT_SECRET"))

		// The session cookie secret signs every session the proxy trusts. An inline Value would
		// put a full authentication bypass in the PodSpec, readable with `get pod`.
		cookie := envs["OAUTH2_PROXY_COOKIE_SECRET"]
		Expect(cookie.Value).To(BeEmpty(), "the cookie secret must never be inlined")
		Expect(cookie.ValueFrom).NotTo(BeNil())
		Expect(cookie.ValueFrom.SecretKeyRef.Name).To(Equal("oidc-credentials"))
		Expect(cookie.ValueFrom.SecretKeyRef.Key).To(Equal(sidecar.OIDCCookieSecretKey))

		Expect(container.Ports).To(HaveLen(1))
		Expect(container.Ports[0].ContainerPort).To(Equal(int32(4180)))
	})

	It("hardens the container by default so restricted Pod Security admits it", func() {
		// The product's own container is hardened by the base handler; a sidecar shipping a nil
		// security context is what makes the whole pod inadmissible in a namespace enforcing the
		// restricted profile. These four fields are exactly the container-level requirements.
		provider := sidecar.NewOAuth2ProxySidecarProvider(
			keycloakProvider(), "oidc-credentials", 18080, sidecar.WithOAuth2ProxyAllowAllEmails())

		Expect(provider.Inject(podSpec, nil)).To(Succeed())

		sc := podSpec.InitContainers[0].SecurityContext
		Expect(sc).NotTo(BeNil())
		Expect(*sc.RunAsNonRoot).To(BeTrue())
		Expect(*sc.AllowPrivilegeEscalation).To(BeFalse())
		Expect(sc.Capabilities.Drop).To(ConsistOf(corev1.Capability("ALL")))
		Expect(sc.SeccompProfile.Type).To(Equal(corev1.SeccompProfileTypeRuntimeDefault))

		// Not pinned: oauth2-proxy is a third-party image with its own USER, and the pod-level
		// security context already supplies the identity.
		Expect(sc.RunAsUser).To(BeNil())
	})

	It("lets an explicit SidecarConfig.SecurityContext replace the default wholesale", func() {
		provider := sidecar.NewOAuth2ProxySidecarProvider(
			keycloakProvider(), "oidc-credentials", 18080, sidecar.WithOAuth2ProxyAllowAllEmails())

		Expect(provider.Inject(podSpec, &sidecar.SidecarConfig{
			SecurityContext: &corev1.SecurityContext{RunAsUser: ptr.To(int64(2000))},
		})).To(Succeed())

		sc := podSpec.InitContainers[0].SecurityContext
		Expect(*sc.RunAsUser).To(Equal(int64(2000)))
		// Replaced, not merged: a half-merged security context is how a container ends up with a
		// setting nobody chose.
		Expect(sc.Capabilities).To(BeNil())
	})

	It("is idempotent", func() {
		provider := sidecar.NewOAuth2ProxySidecarProvider(
			keycloakProvider(), "oidc-credentials", 18080, sidecar.WithOAuth2ProxyAllowAllEmails())

		Expect(provider.Inject(podSpec, nil)).To(Succeed())
		Expect(provider.Inject(podSpec, nil)).To(Succeed())
		Expect(podSpec.InitContainers).To(HaveLen(1))
	})

	It("appends extra scopes after the provider scopes", func() {
		provider := sidecar.NewOAuth2ProxySidecarProvider(
			keycloakProvider(), "oidc-credentials", 18080, sidecar.WithOAuth2ProxyAllowAllEmails(),
			sidecar.WithOAuth2ProxyExtraScopes("groups"))

		Expect(provider.Inject(podSpec, nil)).To(Succeed())
		envs := envByName(&podSpec.InitContainers[0])
		Expect(envs["OAUTH2_PROXY_SCOPE"].Value).To(Equal("openid email profile groups"))
	})

	It("falls back to the default scopes when the provider declares none", func() {
		oidc := keycloakProvider()
		oidc.Scopes = nil
		provider := sidecar.NewOAuth2ProxySidecarProvider(oidc, "oidc-credentials", 18080, sidecar.WithOAuth2ProxyAllowAllEmails())

		Expect(provider.Inject(podSpec, nil)).To(Succeed())
		envs := envByName(&podSpec.InitContainers[0])
		Expect(envs["OAUTH2_PROXY_SCOPE"].Value).To(Equal("openid email profile"))
	})

	It("lets SidecarConfig.EnvVars override built-in defaults (replace semantics)", func() {
		provider := sidecar.NewOAuth2ProxySidecarProvider(
			keycloakProvider(), "oidc-credentials", 18080, sidecar.WithOAuth2ProxyAllowAllEmails())

		Expect(provider.Inject(podSpec, &sidecar.SidecarConfig{
			EnvVars: map[string]string{
				"OAUTH2_PROXY_COOKIE_SECURE": "true",
				"OAUTH2_PROXY_EXTRA_KNOB":    "custom",
			},
		})).To(Succeed())

		envs := envByName(&podSpec.InitContainers[0])
		Expect(envs["OAUTH2_PROXY_COOKIE_SECURE"].Value).To(Equal("true"),
			"a same-named default must be replaced, not silently kept")
		Expect(envs["OAUTH2_PROXY_EXTRA_KNOB"].Value).To(Equal("custom"))

		names := map[string]int{}
		for _, e := range podSpec.InitContainers[0].Env {
			names[e.Name]++
		}
		Expect(names["OAUTH2_PROXY_COOKIE_SECURE"]).To(Equal(1), "no duplicate env entries")
	})

	It("keeps the declared port and HTTP_ADDRESS aligned under a SidecarConfig.Ports override", func() {
		provider := sidecar.NewOAuth2ProxySidecarProvider(
			keycloakProvider(), "oidc-credentials", 18080, sidecar.WithOAuth2ProxyAllowAllEmails())

		Expect(provider.Inject(podSpec, &sidecar.SidecarConfig{
			Ports: []corev1.ContainerPort{{Name: "auth", ContainerPort: 9999}},
		})).To(Succeed())

		container := &podSpec.InitContainers[0]
		Expect(container.Ports[0].ContainerPort).To(Equal(int32(9999)))
		envs := envByName(container)
		Expect(envs["OAUTH2_PROXY_HTTP_ADDRESS"].Value).To(Equal("0.0.0.0:9999"),
			"the proxy must actually listen on the declared port")
	})

	It("keeps its pinned image when the manager propagates the product image", func() {
		manager := sidecar.NewSidecarManager()
		provider := sidecar.NewOAuth2ProxySidecarProvider(
			keycloakProvider(), "oidc-credentials", 18080, sidecar.WithOAuth2ProxyAllowAllEmails())
		manager.Register(provider, &sidecar.SidecarConfig{Enabled: true})

		Expect(manager.SetProductImage("quay.io/zncdatadev/spark-k8s:3.5.5", corev1.PullIfNotPresent)).To(Succeed())
		Expect(manager.InjectAll(podSpec)).To(Succeed())

		Expect(podSpec.InitContainers).To(HaveLen(1))
		Expect(podSpec.InitContainers[0].Image).To(Equal(sidecar.DefaultOAuth2ProxyImage),
			"OwnsImage must shield the provider from product-image propagation")
	})

	It("honors port and image overrides", func() {
		provider := sidecar.NewOAuth2ProxySidecarProvider(
			keycloakProvider(), "oidc-credentials", 8080, sidecar.WithOAuth2ProxyAllowAllEmails(),
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

var _ = Describe("GenerateCookieSecret", func() {
	It("URL-safe-decodes to 32 bytes, as oauth2-proxy requires", func() {
		secret, err := sidecar.GenerateCookieSecret()
		Expect(err).NotTo(HaveOccurred())

		// oauth2-proxy's SecretBytes attempts ONLY unpadded URL-safe base64; a value with
		// '+' or '/' would fall back to the raw string (invalid length) and crash the proxy.
		Expect(secret).NotTo(ContainSubstring("+"))
		Expect(secret).NotTo(ContainSubstring("/"))
		Expect(secret).NotTo(ContainSubstring("="))
		raw, err := base64.RawURLEncoding.DecodeString(secret)
		Expect(err).NotTo(HaveOccurred())
		Expect(raw).To(HaveLen(32))
	})

	It("stays URL-safe across many draws", func() {
		for i := 0; i < 256; i++ {
			secret, err := sidecar.GenerateCookieSecret()
			Expect(err).NotTo(HaveOccurred(), "draw %d", i)
			_, err = base64.RawURLEncoding.DecodeString(secret)
			Expect(err).NotTo(HaveOccurred(), "draw %d", i)
		}
	})

	It("is random, not derived", func() {
		// A value derived from the CR (UID, name) is forgeable by anyone who can read the CR,
		// which is the whole reason the previous deterministic derivation was removed.
		seen := map[string]struct{}{}
		for i := 0; i < 64; i++ {
			secret, err := sidecar.GenerateCookieSecret()
			Expect(err).NotTo(HaveOccurred())
			Expect(seen).NotTo(HaveKey(secret), "draw %d repeated a previous secret", i)
			seen[secret] = struct{}{}
		}
	})
})

var _ = Describe("OAuth2ProxySidecarProvider authorization policy", func() {
	var podSpec *corev1.PodSpec

	BeforeEach(func() {
		podSpec = &corev1.PodSpec{Containers: []corev1.Container{{Name: "node"}}}
	})

	It("refuses to build a proxy with no authorization policy", func() {
		// Authenticating against an IdP is not authorization for this cluster. Defaulting to "*"
		// would admit every account the IdP can issue a token for — on a shared realm, that is
		// everyone. The product has to say which it wants.
		provider := sidecar.NewOAuth2ProxySidecarProvider(
			keycloakProvider(), "oidc-credentials", 18080)

		Expect(provider.Inject(podSpec, nil)).To(
			MatchError(ContainSubstring("no authorization policy configured")))
		Expect(podSpec.InitContainers).To(BeEmpty(), "no container may be built")
	})

	It("rejects a conflicting authorization policy", func() {
		// "*" would win over the domain list, so a caller adding WithOAuth2ProxyEmailDomains to an
		// existing WithOAuth2ProxyAllowAllEmails call — the exact shape of "let me tighten this" —
		// would change nothing and be told nothing.
		provider := sidecar.NewOAuth2ProxySidecarProvider(
			keycloakProvider(), "oidc-credentials", 18080,
			sidecar.WithOAuth2ProxyAllowAllEmails(),
			sidecar.WithOAuth2ProxyEmailDomains("example.com"))

		Expect(provider.Inject(podSpec, nil)).To(
			MatchError(ContainSubstring("conflicting authorization policy")))
		Expect(podSpec.InitContainers).To(BeEmpty(), "no container may be built")
	})

	It("restricts logins to the declared email domains", func() {
		provider := sidecar.NewOAuth2ProxySidecarProvider(
			keycloakProvider(), "oidc-credentials", 18080,
			sidecar.WithOAuth2ProxyEmailDomains("example.com", "corp.example.com"))

		Expect(provider.Inject(podSpec, nil)).To(Succeed())
		envs := envByName(&podSpec.InitContainers[0])
		Expect(envs["OAUTH2_PROXY_EMAIL_DOMAINS"].Value).To(Equal("example.com,corp.example.com"))
	})

	It("emits the redirect whitelist only when domains are declared", func() {
		provider := sidecar.NewOAuth2ProxySidecarProvider(
			keycloakProvider(), "oidc-credentials", 18080,
			sidecar.WithOAuth2ProxyAllowAllEmails(),
			sidecar.WithOAuth2ProxyWhitelistDomains("app.example.com"))

		Expect(provider.Inject(podSpec, nil)).To(Succeed())
		envs := envByName(&podSpec.InitContainers[0])
		Expect(envs["OAUTH2_PROXY_WHITELIST_DOMAINS"].Value).To(Equal("app.example.com"))
	})

	It("reads the cookie secret from an overridden Secret and key", func() {
		provider := sidecar.NewOAuth2ProxySidecarProvider(
			keycloakProvider(), "oidc-credentials", 18080,
			sidecar.WithOAuth2ProxyAllowAllEmails(),
			sidecar.WithOAuth2ProxyCookieSecretRef(&corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "session-keys"},
				Key:                  "current",
			}))

		Expect(provider.Inject(podSpec, nil)).To(Succeed())
		cookie := envByName(&podSpec.InitContainers[0])["OAUTH2_PROXY_COOKIE_SECRET"]
		Expect(cookie.Value).To(BeEmpty())
		Expect(cookie.ValueFrom.SecretKeyRef.Name).To(Equal("session-keys"))
		Expect(cookie.ValueFrom.SecretKeyRef.Key).To(Equal("current"))
	})
})

var _ = Describe("OAuth2ProxySidecarProvider readiness coupling", func() {
	It("injects no readiness probe (an unready native sidecar would gate the whole pod)", func() {
		podSpec := &corev1.PodSpec{Containers: []corev1.Container{{Name: "node"}}}
		provider := sidecar.NewOAuth2ProxySidecarProvider(
			keycloakProvider(), "oidc-credentials", 18080, sidecar.WithOAuth2ProxyAllowAllEmails())

		Expect(provider.Inject(podSpec, nil)).To(Succeed())
		Expect(podSpec.InitContainers[0].ReadinessProbe).To(BeNil())
	})

	It("injects a startup probe so the pod is not Ready before the proxy can serve", func() {
		// This container is the one framework sidecar in the request path: the Service targets
		// its port and it forwards to the upstream. Pod readiness, meanwhile, is decided by the
		// MAIN container's probe on the product's own port — so without a gate here the pod is
		// Ready, and receiving traffic, while the proxy has merely been launched. Every rollout
		// produced a window of refused connections.
		//
		// A startupProbe is what closes it: the kubelet starts the next init container (and so
		// the main container) only once this one has started, which for a container with a
		// startupProbe means once the probe SUCCEEDS. Unlike readiness it stops applying once
		// satisfied, so a later IdP outage cannot evict the pod.
		podSpec := &corev1.PodSpec{Containers: []corev1.Container{{Name: "node"}}}
		provider := sidecar.NewOAuth2ProxySidecarProvider(
			keycloakProvider(), "oidc-credentials", 18080, sidecar.WithOAuth2ProxyAllowAllEmails())

		Expect(provider.Inject(podSpec, nil)).To(Succeed())

		container := podSpec.InitContainers[0]
		probe := container.StartupProbe
		Expect(probe).NotTo(BeNil(), "without this the pod serves traffic before the proxy listens")
		Expect(probe.HTTPGet).NotTo(BeNil())
		// The literal, deliberately not sidecar.OAuth2ProxyPingPath: the guarantee is the VALUE.
		// oauth2-proxy documents /ready as a DEEP health check, so pointing the constant there
		// would reintroduce exactly the IdP coupling the absent readiness probe exists to avoid —
		// and an assertion against the constant would follow it there without complaining.
		Expect(probe.HTTPGet.Path).To(Equal("/ping"))
		Expect(probe.HTTPGet.Port.IntValue()).To(Equal(sidecar.OAuth2ProxyPort))
		// OIDC discovery against a cold IdP is the slow step, and failing startup only restarts
		// the container to re-run it.
		Expect(probe.PeriodSeconds * probe.FailureThreshold).To(BeNumerically(">=", 60))

		Expect(container.LivenessProbe).NotTo(BeNil(), "a wedged proxy must be restarted")
		Expect(container.LivenessProbe.HTTPGet.Path).To(Equal("/ping"))
	})

	It("lets a product replace or remove the framework probes", func() {
		podSpec := &corev1.PodSpec{Containers: []corev1.Container{{Name: "node"}}}
		provider := sidecar.NewOAuth2ProxySidecarProvider(
			keycloakProvider(), "oidc-credentials", 18080, sidecar.WithOAuth2ProxyAllowAllEmails())

		config := &sidecar.SidecarConfig{}
		config.Probes.Startup = &corev1.Probe{
			ProbeHandler:  corev1.ProbeHandler{Exec: &corev1.ExecAction{Command: []string{"true"}}},
			PeriodSeconds: 7,
		}
		config.Probes.DisableLiveness = true
		Expect(provider.Inject(podSpec, config)).To(Succeed())

		container := podSpec.InitContainers[0]
		Expect(container.StartupProbe).NotTo(BeNil())
		Expect(container.StartupProbe.Exec).NotTo(BeNil())
		Expect(container.StartupProbe.HTTPGet).To(BeNil(), "the override replaces wholesale, never merges")
		Expect(container.LivenessProbe).To(BeNil())
	})

	It("targets the overridden port with both probes", func() {
		// The probes are built from the effective port, so a SidecarConfig.Ports override cannot
		// leave them pointing at a port nothing listens on.
		podSpec := &corev1.PodSpec{Containers: []corev1.Container{{Name: "node"}}}
		provider := sidecar.NewOAuth2ProxySidecarProvider(
			keycloakProvider(), "oidc-credentials", 18080, sidecar.WithOAuth2ProxyAllowAllEmails())

		Expect(provider.Inject(podSpec, &sidecar.SidecarConfig{
			Ports: []corev1.ContainerPort{{Name: "proxy", ContainerPort: 9999}},
		})).To(Succeed())

		container := podSpec.InitContainers[0]
		Expect(container.StartupProbe.HTTPGet.Port.IntValue()).To(Equal(9999))
		Expect(container.LivenessProbe.HTTPGet.Port.IntValue()).To(Equal(9999))
	})

	It("rejects injection without an OIDC provider", func() {
		podSpec := &corev1.PodSpec{Containers: []corev1.Container{{Name: "node"}}}
		provider := sidecar.NewOAuth2ProxySidecarProvider(nil, "oidc-credentials", 18080, sidecar.WithOAuth2ProxyAllowAllEmails())
		Expect(provider.Inject(podSpec, nil)).To(MatchError(ContainSubstring("OIDC provider is required")))
	})
})

var _ = Describe("OAuth2ProxySidecarProvider Validate", func() {
	var ctx context.Context

	BeforeEach(func() { ctx = context.Background() })

	// newClient builds a fake client holding the client-credentials Secret with the given keys.
	newClient := func(keys ...string) *fake.ClientBuilder {
		scheme := runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		data := map[string][]byte{}
		for _, k := range keys {
			data[k] = []byte("value")
		}
		return fake.NewClientBuilder().WithScheme(scheme).WithObjects(&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "oidc-credentials", Namespace: "test-ns"},
			Data:       data,
		})
	}

	It("accepts a Secret carrying all three keys", func() {
		c := newClient(sidecar.OIDCClientIDKey, sidecar.OIDCClientSecretKey, sidecar.OIDCCookieSecretKey).Build()
		provider := sidecar.NewOAuth2ProxySidecarProvider(
			keycloakProvider(), "oidc-credentials", 18080,
			sidecar.WithOAuth2ProxyAllowAllEmails())

		Expect(provider.Validate(ctx, c, "test-ns")).To(Succeed())
	})

	It("rejects a Secret missing the cookie secret key", func() {
		// Without the key the proxy starts with an empty OAUTH2_PROXY_COOKIE_SECRET and refuses
		// to serve. Caught here, the operator reports which Secret and which key; caught by the
		// kubelet, it is an opaque crash loop.
		c := newClient(sidecar.OIDCClientIDKey, sidecar.OIDCClientSecretKey).Build()
		provider := sidecar.NewOAuth2ProxySidecarProvider(
			keycloakProvider(), "oidc-credentials", 18080,
			sidecar.WithOAuth2ProxyAllowAllEmails())

		err := provider.Validate(ctx, c, "test-ns")
		Expect(err).To(MatchError(ContainSubstring(sidecar.OIDCCookieSecretKey)))
		Expect(err).To(MatchError(ContainSubstring("oidc-credentials")))
	})

	It("rejects a missing authorization policy", func() {
		c := newClient(sidecar.OIDCClientIDKey, sidecar.OIDCClientSecretKey, sidecar.OIDCCookieSecretKey).Build()
		provider := sidecar.NewOAuth2ProxySidecarProvider(
			keycloakProvider(), "oidc-credentials", 18080)

		Expect(provider.Validate(ctx, c, "test-ns")).To(
			MatchError(ContainSubstring("no authorization policy configured")))
	})

	It("reports the overridden cookie Secret by name when it does not exist", func() {
		c := newClient(sidecar.OIDCClientIDKey, sidecar.OIDCClientSecretKey).Build()
		provider := sidecar.NewOAuth2ProxySidecarProvider(
			keycloakProvider(), "oidc-credentials", 18080,
			sidecar.WithOAuth2ProxyAllowAllEmails(),
			sidecar.WithOAuth2ProxyCookieSecretRef(&corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "session-keys"},
				Key:                  "current",
			}))

		Expect(provider.Validate(ctx, c, "test-ns")).To(
			MatchError(ContainSubstring(`cookie secret "session-keys" not found`)))
	})
})
