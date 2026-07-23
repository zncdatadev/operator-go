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
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	authv1alpha1 "github.com/zncdatadev/operator-go/pkg/apis/authentication/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
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
	cookieSecret            string
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

// WithOAuth2ProxyCookieSecret replaces the derived cookie secret with an explicit one
// (e.g. sourced from a Secret by the product).
func WithOAuth2ProxyCookieSecret(secret string) OAuth2ProxyOption {
	return func(p *OAuth2ProxySidecarProvider) { p.cookieSecret = secret }
}

// WithOAuth2ProxyContainerName overrides the sidecar container name.
func WithOAuth2ProxyContainerName(name string) OAuth2ProxyOption {
	return func(p *OAuth2ProxySidecarProvider) { p.name = name }
}

// NewOAuth2ProxySidecarProvider creates an oauth2-proxy sidecar provider.
//
// oidcProvider is the resolved AuthenticationClass OIDC provider (the product fetches the
// class and asserts the provider type). clientCredentialsSecret names a Secret carrying
// CLIENT_ID/CLIENT_SECRET. upstreamPort is the product container port the proxy forwards
// to on localhost. cookieSeed must be stable per product instance across reconciles —
// typically the CR's UID — so the derived session cookie secret does not churn pods; see
// DeterministicCookieSecret.
func NewOAuth2ProxySidecarProvider(oidcProvider *authv1alpha1.OIDCProvider, clientCredentialsSecret string, upstreamPort int32, cookieSeed string, opts ...OAuth2ProxyOption) *OAuth2ProxySidecarProvider {
	p := &OAuth2ProxySidecarProvider{
		name:                    OAuth2ProxySidecarName,
		port:                    OAuth2ProxyPort,
		upstreamPort:            upstreamPort,
		oidcProvider:            oidcProvider,
		clientCredentialsSecret: clientCredentialsSecret,
		cookieSecret:            DeterministicCookieSecret(cookieSeed),
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

// Validate validates that the client credentials Secret exists.
func (p *OAuth2ProxySidecarProvider) Validate(ctx context.Context, c client.Client, namespace string) error {
	if p.oidcProvider == nil {
		return fmt.Errorf("oauth2-proxy: OIDC provider is required")
	}
	if err := ValidateSecretExists(ctx, c, namespace, p.clientCredentialsSecret); err != nil {
		return fmt.Errorf("oauth2-proxy client credentials secret %q not found: %w", p.clientCredentialsSecret, err)
	}
	return nil
}

// Inject injects the oauth2-proxy sidecar into the pod spec as a native sidecar (init
// container with restartPolicy Always), following the SidecarManager's single-owner model.
// This method is idempotent — calling it multiple times will not duplicate the container.
func (p *OAuth2ProxySidecarProvider) Inject(podSpec *corev1.PodSpec, config *SidecarConfig) error {
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
			{Name: "OAUTH2_PROXY_COOKIE_SECRET", Value: p.cookieSecret},
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
			{Name: "OAUTH2_PROXY_HTTP_ADDRESS", Value: "0.0.0.0:" + strconv.Itoa(int(p.port))},
			// The proxy serves plain HTTP inside the pod network; secure cookies would be
			// dropped by browsers on http:// service URLs. Products terminating TLS in
			// front can override via SidecarConfig.EnvVars.
			{Name: "OAUTH2_PROXY_COOKIE_SECURE", Value: "false"},
			{Name: "OAUTH2_PROXY_WHITELIST_DOMAINS", Value: "*"},
			{Name: "OAUTH2_PROXY_CODE_CHALLENGE_METHOD", Value: "S256"},
			{Name: "OAUTH2_PROXY_EMAIL_DOMAINS", Value: "*"},
		},
		Ports: []corev1.ContainerPort{
			{Name: OAuth2ProxyPortName, ContainerPort: p.port, Protocol: corev1.ProtocolTCP},
		},
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("600m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: "/ping",
					Port: intstr.FromInt(int(p.port)),
				},
			},
			InitialDelaySeconds: 5,
			TimeoutSeconds:      5,
			PeriodSeconds:       10,
		},
	}

	if config.Resources != nil {
		container.Resources = *config.Resources
	}
	if config.SecurityContext != nil {
		container.SecurityContext = config.SecurityContext
	}
	if len(config.EnvVars) > 0 {
		AddEnvVars(container, config.EnvVars)
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

// DeterministicCookieSecret derives a stable oauth2-proxy cookie secret from a seed
// (typically the CR UID): base64(sha256(seed)), which oauth2-proxy decodes to a 32-byte
// secret. Deriving instead of generating keeps reconciles idempotent — a random secret
// would roll every pod on every reconcile.
func DeterministicCookieSecret(seed string) string {
	hash := sha256.Sum256([]byte(seed))
	return base64.StdEncoding.EncodeToString(hash[:])
}
