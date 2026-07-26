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

package sidecar

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	authv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/authentication/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	// OAuth2ProxySidecarName is the name of the oauth2-proxy sidecar container.
	OAuth2ProxySidecarName = "oauth2-proxy"
	// OAuth2ProxyPort is the default oauth2-proxy listen port.
	OAuth2ProxyPort = 4180
	// OAuth2ProxyPortName is the default name of the oauth2-proxy container port.
	OAuth2ProxyPortName = "oauth2-proxy"
	// DefaultOAuth2ProxyImage is the default (pinned) oauth2-proxy image. Products
	// override it per role group via SidecarConfig.Image.
	DefaultOAuth2ProxyImage = "quay.io/oauth2-proxy/oauth2-proxy:v7.8.2"

	// OIDCClientIDKey and OIDCClientSecretKey are the keys a client-credentials Secret
	// must carry, matching the platform's AuthenticationClass OIDC documentation.
	OIDCClientIDKey     = "CLIENT_ID"
	OIDCClientSecretKey = "CLIENT_SECRET"
	// OIDCCookieSecretKey is the key the session cookie secret is read from. It defaults to
	// living in the same Secret as the client credentials, so a product wiring OIDC creates one
	// Secret with three keys. Generate the value with GenerateCookieSecret.
	OIDCCookieSecretKey = "COOKIE_SECRET"
)

// defaultOIDCScopes are requested when the AuthenticationClass declares no scopes.
var defaultOIDCScopes = []string{"openid", "email", "profile"}

// OAuth2ProxySidecarProvider injects an oauth2-proxy authentication proxy in front of a
// product's HTTP endpoint, wired from an AuthenticationClass OIDC provider. The proxy
// terminates the OIDC login flow on its own port and forwards authenticated requests to the
// product container over localhost; the product's client-facing Service should target the
// proxy port instead of the upstream port.
type OAuth2ProxySidecarProvider struct {
	name                    string
	port                    int32
	upstreamPort            int32
	oidcProvider            *authv1alpha1.OIDCProvider
	clientCredentialsSecret string
	extraScopes             []string
	cookieSecretRef         *corev1.SecretKeySelector
	emailDomains            []string
	allowAllEmails          bool
	whitelistDomains        []string
}

// OAuth2ProxyOption customizes an OAuth2ProxySidecarProvider.
type OAuth2ProxyOption func(*OAuth2ProxySidecarProvider)

// WithOAuth2ProxyPort overrides the proxy listen port (default 4180).
func WithOAuth2ProxyPort(port int32) OAuth2ProxyOption {
	return func(p *OAuth2ProxySidecarProvider) { p.port = port }
}

// WithOAuth2ProxyExtraScopes appends product-requested scopes to the provider scopes.
func WithOAuth2ProxyExtraScopes(scopes ...string) OAuth2ProxyOption {
	return func(p *OAuth2ProxySidecarProvider) { p.extraScopes = append(p.extraScopes, scopes...) }
}

// WithOAuth2ProxyCookieSecretRef points the session cookie secret at a different Secret and/or
// key than the default (the client-credentials Secret's COOKIE_SECRET key). A nil ref is
// ignored, keeping the default.
//
// The value is only ever referenced, never inlined: a cookie secret in the PodSpec is readable
// by anyone who can `get pod`, and it signs the session cookies the proxy trusts.
func WithOAuth2ProxyCookieSecretRef(ref *corev1.SecretKeySelector) OAuth2ProxyOption {
	return func(p *OAuth2ProxySidecarProvider) {
		if ref != nil {
			p.cookieSecretRef = ref
		}
	}
}

// WithOAuth2ProxyEmailDomains restricts logins to accounts whose email is in one of these
// domains (OAUTH2_PROXY_EMAIL_DOMAINS). Authenticating against an identity provider is not the
// same as being authorized for this cluster: on a shared IdP — a public provider, or a
// corporate Keycloak carrying a customer realm — every account it can issue a token for would
// otherwise reach the product. Either this or WithOAuth2ProxyAllowAllEmails is required.
func WithOAuth2ProxyEmailDomains(domains ...string) OAuth2ProxyOption {
	return func(p *OAuth2ProxySidecarProvider) { p.emailDomains = append(p.emailDomains, domains...) }
}

// WithOAuth2ProxyAllowAllEmails admits every account the identity provider authenticates
// ("*"). This is the correct choice for an IdP realm dedicated to this cluster, and a serious
// mistake for a shared one — it is spelled out as its own option so that it is a decision in
// the product's code, greppable at review time, rather than a default nobody chose.
func WithOAuth2ProxyAllowAllEmails() OAuth2ProxyOption {
	return func(p *OAuth2ProxySidecarProvider) { p.allowAllEmails = true }
}

// WithOAuth2ProxyWhitelistDomains permits post-login redirects to these domains
// (OAUTH2_PROXY_WHITELIST_DOMAINS) in addition to the proxy's own host. Leave unset unless a
// redirect off-host is actually needed: every entry widens what an attacker-supplied `rd`
// parameter can point at.
func WithOAuth2ProxyWhitelistDomains(domains ...string) OAuth2ProxyOption {
	return func(p *OAuth2ProxySidecarProvider) {
		p.whitelistDomains = append(p.whitelistDomains, domains...)
	}
}

// WithOAuth2ProxyContainerName overrides the sidecar container name.
func WithOAuth2ProxyContainerName(name string) OAuth2ProxyOption {
	return func(p *OAuth2ProxySidecarProvider) { p.name = name }
}

// NewOAuth2ProxySidecarProvider creates an oauth2-proxy sidecar provider.
//
// oidcProvider is the resolved AuthenticationClass OIDC provider (the product fetches the
// class and asserts the provider type). clientCredentialsSecret names a Secret carrying
// CLIENT_ID/CLIENT_SECRET and, by default, the COOKIE_SECRET used to sign session cookies
// (override the location with WithOAuth2ProxyCookieSecretRef; generate the value with
// GenerateCookieSecret). upstreamPort is the product container port the proxy forwards to on
// localhost.
//
// An authorization policy is mandatory: pass WithOAuth2ProxyEmailDomains or, deliberately,
// WithOAuth2ProxyAllowAllEmails. Inject fails without one rather than admitting everybody.
func NewOAuth2ProxySidecarProvider(oidcProvider *authv1alpha1.OIDCProvider, clientCredentialsSecret string, upstreamPort int32, opts ...OAuth2ProxyOption) *OAuth2ProxySidecarProvider {
	p := &OAuth2ProxySidecarProvider{
		name:                    OAuth2ProxySidecarName,
		port:                    OAuth2ProxyPort,
		upstreamPort:            upstreamPort,
		oidcProvider:            oidcProvider,
		clientCredentialsSecret: clientCredentialsSecret,
		cookieSecretRef: &corev1.SecretKeySelector{
			LocalObjectReference: corev1.LocalObjectReference{Name: clientCredentialsSecret},
			Key:                  OIDCCookieSecretKey,
		},
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Name returns the sidecar name.
func (p *OAuth2ProxySidecarProvider) Name() string {
	return p.name
}

// OwnsImage implements OwnImageProvider: oauth2-proxy ships as its own upstream image, so
// SetProductImage must not fill an empty SidecarConfig.Image with the product image —
// DefaultOAuth2ProxyImage stays reachable for a plain `&SidecarConfig{Enabled: true}`
// registration.
func (p *OAuth2ProxySidecarProvider) OwnsImage() bool {
	return true
}

// Validate validates that the client credentials Secret exists and that the Secret holding the
// session cookie secret carries the referenced key.
//
// The key check is not pedantry: with the key absent the proxy starts with an empty
// OAUTH2_PROXY_COOKIE_SECRET and refuses to serve, so the failure would otherwise surface as a
// crash-looping container instead of an error naming the Secret and the key. This mirrors the
// Vector provider, which likewise validates that its ConfigMap carries vector.yaml rather than
// merely that the ConfigMap exists.
func (p *OAuth2ProxySidecarProvider) Validate(ctx context.Context, c client.Client, namespace string) error {
	if p.oidcProvider == nil {
		return fmt.Errorf("oauth2-proxy: OIDC provider is required")
	}
	if err := ValidateSecretExists(ctx, c, namespace, p.clientCredentialsSecret); err != nil {
		return fmt.Errorf("oauth2-proxy client credentials secret %q not found: %w", p.clientCredentialsSecret, err)
	}
	if err := p.validateAuthorization(); err != nil {
		return err
	}

	ref := p.cookieSecretRef
	cookieSecret := &corev1.Secret{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: ref.Name}, cookieSecret); err != nil {
		return fmt.Errorf("oauth2-proxy cookie secret %q not found: %w", ref.Name, err)
	}
	if _, ok := cookieSecret.Data[ref.Key]; !ok {
		return fmt.Errorf(
			"oauth2-proxy cookie secret %q has no %q key: the session cookie secret signs every session this proxy trusts, so it must be supplied out of band (see GenerateCookieSecret)",
			ref.Name, ref.Key)
	}
	return nil
}

// validateAuthorization enforces that an authorization policy was chosen explicitly.
func (p *OAuth2ProxySidecarProvider) validateAuthorization() error {
	if len(p.emailDomains) == 0 && !p.allowAllEmails {
		return fmt.Errorf(
			"oauth2-proxy: no authorization policy configured; pass WithOAuth2ProxyEmailDomains to restrict logins, or WithOAuth2ProxyAllowAllEmails to admit every account the identity provider authenticates")
	}
	return nil
}

// Inject injects the oauth2-proxy sidecar into the pod spec as a native sidecar (init
// container with restartPolicy Always), following the SidecarManager's single-owner model.
// This method is idempotent — calling it multiple times will not duplicate the container.
func (p *OAuth2ProxySidecarProvider) Inject(podSpec *corev1.PodSpec, config *SidecarConfig) error {
	if p.oidcProvider == nil {
		return fmt.Errorf("oauth2-proxy: OIDC provider is required")
	}
	// Checked here as well as in Validate: Inject runs during BuildResources, before the
	// reconciler's ValidateAll, and this check needs no API client. Failing at the earlier of
	// the two points means a pod that admits everybody is never built in the first place.
	if err := p.validateAuthorization(); err != nil {
		return err
	}
	if config == nil {
		config = &SidecarConfig{Enabled: true}
	}

	image := config.Image
	if image == "" {
		image = DefaultOAuth2ProxyImage
	}
	pullPolicy := corev1.PullIfNotPresent
	if config.ImagePullPolicy != "" {
		pullPolicy = config.ImagePullPolicy
	}

	// The effective listen port: a SidecarConfig.Ports override wins, so the declared port
	// and the OAUTH2_PROXY_HTTP_ADDRESS the proxy actually binds can never diverge.
	port := p.port
	if len(config.Ports) > 0 {
		port = config.Ports[0].ContainerPort
	}

	scopes := p.oidcProvider.Scopes
	if len(scopes) == 0 {
		scopes = defaultOIDCScopes
	}
	scopes = append(append([]string{}, scopes...), p.extraScopes...)

	container := &corev1.Container{
		Name:            p.name,
		Image:           image,
		ImagePullPolicy: pullPolicy,
		Env: []corev1.EnvVar{
			{
				// Referenced, never inlined. This value signs every session cookie the proxy
				// will subsequently trust, so an inline Value would put a full authentication
				// bypass in the PodSpec, readable by anyone who can `get pod`.
				Name: "OAUTH2_PROXY_COOKIE_SECRET",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: p.cookieSecretRef.Name},
						Key:                  p.cookieSecretRef.Key,
					},
				},
			},
			{
				Name: "OAUTH2_PROXY_CLIENT_ID",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: p.clientCredentialsSecret},
						Key:                  OIDCClientIDKey,
					},
				},
			},
			{
				Name: "OAUTH2_PROXY_CLIENT_SECRET",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{
						LocalObjectReference: corev1.LocalObjectReference{Name: p.clientCredentialsSecret},
						Key:                  OIDCClientSecretKey,
					},
				},
			},
			{Name: "OAUTH2_PROXY_OIDC_ISSUER_URL", Value: OIDCIssuerURL(p.oidcProvider)},
			{Name: "OAUTH2_PROXY_SCOPE", Value: strings.Join(scopes, " ")},
			{Name: "OAUTH2_PROXY_PROVIDER", Value: OAuth2ProxyProviderFor(p.oidcProvider.ProviderHint)},
			{Name: "OAUTH2_PROXY_UPSTREAMS", Value: "http://localhost:" + strconv.Itoa(int(p.upstreamPort))},
			{Name: "OAUTH2_PROXY_HTTP_ADDRESS", Value: "0.0.0.0:" + strconv.Itoa(int(port))},
			// The proxy serves plain HTTP inside the pod network; secure cookies would be
			// dropped by browsers on http:// service URLs. Products terminating TLS in
			// front override via SidecarConfig.EnvVars (applied with replace semantics below).
			{Name: "OAUTH2_PROXY_COOKIE_SECURE", Value: "false"},
			{Name: "OAUTH2_PROXY_CODE_CHALLENGE_METHOD", Value: "S256"},
			{Name: "OAUTH2_PROXY_EMAIL_DOMAINS", Value: p.emailDomainsValue()},
		},
		Ports: []corev1.ContainerPort{
			{Name: OAuth2ProxyPortName, ContainerPort: port, Protocol: corev1.ProtocolTCP},
		},
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("600m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		},
		// Deliberately no readiness probe: as a native sidecar, an unready oauth2-proxy
		// would gate readiness of the WHOLE pod — an issuer (IdP) outage would take the
		// product's non-auth ports out of every Service, not just the login flow.
	}

	// Emitted only when the product asked for off-host redirects. Unset, oauth2-proxy permits
	// redirects to its own host only, which is the closed default; "*" would turn the `rd`
	// parameter into an open redirect usable to bounce an authenticated user anywhere.
	if len(p.whitelistDomains) > 0 {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  "OAUTH2_PROXY_WHITELIST_DOMAINS",
			Value: strings.Join(p.whitelistDomains, ","),
		})
	}

	if config.Resources != nil {
		container.Resources = *config.Resources
	}
	if config.SecurityContext != nil {
		container.SecurityContext = config.SecurityContext
	}
	// Replace semantics, not add-only: oauth2-proxy's whole configuration surface IS env
	// vars, so SidecarConfig.EnvVars must be able to change the built-in defaults (e.g.
	// OAUTH2_PROXY_COOKIE_SECURE=true behind TLS, tightening OAUTH2_PROXY_EMAIL_DOMAINS).
	if len(config.EnvVars) > 0 {
		AddOrReplaceEnvVars(container, config.EnvVars)
	}
	if len(config.VolumeMounts) > 0 {
		AddVolumeMounts(container, config.VolumeMounts)
	}
	if len(config.Ports) > 0 {
		container.Ports = config.Ports
	}

	// oauth2-proxy is a long-running traffic-serving sidecar: inject it as a native sidecar
	// so kubelet starts it before (and stops it after) the main container.
	container.RestartPolicy = SidecarRestartPolicy()
	AddOrReplaceInitContainer(podSpec, container)

	AddVolumes(podSpec, config.Volumes)

	return nil
}

// OIDCIssuerURL renders the issuer URL for an AuthenticationClass OIDC provider:
// scheme from the provider TLS declaration, host from hostname (plus port unless it is the
// scheme default), and path from rootPath.
func OIDCIssuerURL(provider *authv1alpha1.OIDCProvider) string {
	scheme := "http"
	if provider.TLS != nil {
		scheme = "https"
	}
	issuer := url.URL{
		Scheme: scheme,
		Host:   provider.Hostname,
		Path:   provider.RootPath,
	}
	port := provider.Port
	isSchemeDefault := (scheme == "http" && port == 80) || (scheme == "https" && port == 443)
	if port != 0 && !isSchemeDefault {
		issuer.Host += ":" + strconv.Itoa(port)
	}
	return issuer.String()
}

// OAuth2ProxyProviderFor maps an AuthenticationClass providerHint to the oauth2-proxy
// provider name. Hints with a dedicated oauth2-proxy implementation map to it (keycloak →
// keycloak-oidc); anything else, including an empty hint, uses the generic oidc provider.
func OAuth2ProxyProviderFor(providerHint string) string {
	switch providerHint {
	case "keycloak":
		return "keycloak-oidc"
	case "":
		return "oidc"
	default:
		return providerHint
	}
}

// emailDomainsValue renders OAUTH2_PROXY_EMAIL_DOMAINS. validateAuthorization has already
// established that exactly one of the two sources is set.
func (p *OAuth2ProxySidecarProvider) emailDomainsValue() string {
	if p.allowAllEmails {
		return "*"
	}
	return strings.Join(p.emailDomains, ",")
}

// GenerateCookieSecret returns a fresh, cryptographically random oauth2-proxy cookie secret:
// the URL-safe, unpadded base64 of 32 random bytes, which oauth2-proxy's SecretBytes decodes
// back to a 32-byte key.
//
// URL-safe encoding is load-bearing: oauth2-proxy attempts ONLY base64.URLEncoding, so a
// standard-encoded value containing '+' or '/' fails that decode and falls back to the raw
// 43/44-byte string — an invalid key length that crash-loops the proxy.
//
// Call this once when creating the Secret, and store the result. Do NOT call it per reconcile:
// the value must be stable, or every pass would produce a different session key, roll the pods
// and log every user out. It is deliberately not derived from the CR (a UID, a name): anything
// readable through the API is not a secret, and a derived value would be forgeable by anyone
// who can read the CR.
func GenerateCookieSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate an oauth2-proxy cookie secret: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
