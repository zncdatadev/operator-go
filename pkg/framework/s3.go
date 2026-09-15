package framework

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"reflect"
	"strconv"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
)

type S3ConnectionType string

const (
	S3Disabled  S3ConnectionType = "disabled"
	S3Inline    S3ConnectionType = "inline"
	S3Reference S3ConnectionType = "reference"
)

// S3Connection is a domain value, not a generic tagged union. A zero default is
// disabled. Changing Type discards the inherited connection branch; omitting it
// or preserving it inherits fields within that branch.
type S3Connection struct {
	Type      S3ConnectionType `json:"type"`
	Inline    S3Endpoint       `json:"inline"`
	Reference string           `json:"reference"`
}

type S3Endpoint struct {
	Host        string        `json:"host"`
	Port        int32         `json:"port"`
	TLS         bool          `json:"tls"`
	Region      string        `json:"region"`
	PathStyle   bool          `json:"pathStyle"`
	Credentials S3Credentials `json:"credentials"`
}

// S3Credentials declares where runtime files ACCESS_KEY and SECRET_KEY come
// from. Values never enter effective configuration, facts or generated files.
type S3Credentials struct {
	SecretName  string   `json:"secretName"`
	SecretClass string   `json:"secretClass"`
	Scope       []string `json:"scope,omitempty"`
}

// ResolvedS3Connection contains only public endpoint data and credential
// references. Authentication and bucket permissions are observed by the product.
type ResolvedS3Connection struct {
	Endpoint    string        `json:"endpoint"`
	Region      string        `json:"region"`
	PathStyle   bool          `json:"pathStyle"`
	Credentials S3Credentials `json:"credentials"`
}

// Validate checks a fully folded value; individual role/group layers can be
// partial. No implicit credential provider or insecure TLS downgrade is selected.
func (s S3Connection) Validate() error {
	switch s.Type {
	case "", S3Disabled:
		if s.Reference != "" || !reflect.DeepEqual(s.Inline, S3Endpoint{}) {
			return fmt.Errorf("disabled S3 cannot carry an inline connection or reference")
		}
	case S3Reference:
		if s.Reference == "" || len(validation.IsDNS1123Subdomain(s.Reference)) != 0 ||
			!reflect.DeepEqual(s.Inline, S3Endpoint{}) {
			return fmt.Errorf("reference S3 requires a DNS resource name and no inline fields")
		}
	case S3Inline:
		if s.Reference != "" {
			return fmt.Errorf("inline S3 cannot carry a reference")
		}
		_, err := resolveS3Endpoint(s.Inline)
		return err
	default:
		return fmt.Errorf("S3 type must be disabled, inline or reference")
	}
	return nil
}

func resolveS3Endpoint(endpoint S3Endpoint) (ResolvedS3Connection, error) {
	if net.ParseIP(endpoint.Host) == nil && len(validation.IsDNS1123Subdomain(endpoint.Host)) != 0 {
		return ResolvedS3Connection{}, fmt.Errorf("S3 host must be a DNS name or IP address")
	}
	if endpoint.Port < 0 || endpoint.Port > 65535 {
		return ResolvedS3Connection{}, fmt.Errorf("S3 port must be between 1 and 65535, or absent")
	}
	protocol := "http"
	if endpoint.TLS {
		protocol = "https"
	}
	if endpoint.Port == 0 {
		endpoint.Port = 80
		if endpoint.TLS {
			endpoint.Port = 443
		}
	}
	if endpoint.Region == "" {
		endpoint.Region = "us-east-1"
	}
	if strings.TrimSpace(endpoint.Region) != endpoint.Region || strings.ContainsAny(endpoint.Region, "\r\n\x00") {
		return ResolvedS3Connection{}, fmt.Errorf("S3 region must be a single nonempty value")
	}
	credentials := endpoint.Credentials
	if (credentials.SecretName == "") == (credentials.SecretClass == "") {
		return ResolvedS3Connection{}, fmt.Errorf("S3 credentials require exactly one SecretName or SecretClass")
	}
	for _, name := range []string{credentials.SecretName, credentials.SecretClass} {
		if name != "" && len(validation.IsDNS1123Subdomain(name)) != 0 {
			return ResolvedS3Connection{}, fmt.Errorf("S3 credential reference is invalid")
		}
	}
	if credentials.SecretName != "" && len(credentials.Scope) != 0 {
		return ResolvedS3Connection{}, fmt.Errorf("S3 credential scope applies only to SecretClass")
	}
	seen := map[string]bool{}
	for _, scope := range credentials.Scope {
		kind, name, qualified := strings.Cut(scope, "=")
		valid := !qualified && (kind == SecretScopePod || kind == SecretScopeNode)
		if qualified && (kind == "service" || kind == "listener-volume") {
			valid = name != "" && len(validation.IsDNS1123Label(name)) == 0
		}
		if !valid || seen[scope] {
			return ResolvedS3Connection{}, fmt.Errorf("S3 credential scope is invalid or duplicated")
		}
		seen[scope] = true
	}
	return ResolvedS3Connection{Endpoint: protocol + "://" + net.JoinHostPort(endpoint.Host, strconv.Itoa(int(endpoint.Port))),
		Region: endpoint.Region, PathStyle: endpoint.PathStyle, Credentials: credentials}, nil
}

// ResolveS3Connection consumes inline input or the organization's S3Connection
// resource in the workload namespace. SecretClass materialization remains a
// runtime platform dependency, never a prerequisite requiring generated bytes.
func ResolveS3Connection(ctx context.Context, reader FactsReader, namespace string, connection S3Connection) (
	FactResult[ResolvedS3Connection], error,
) {
	invalid := func(message string) (FactResult[ResolvedS3Connection], error) {
		return FactResult[ResolvedS3Connection]{Diagnostic: FactDiagnostic{State: FactsInvalid,
			Reason: "InvalidS3Connection", Message: message}}, nil
	}
	if err := connection.Validate(); err != nil {
		return invalid(err.Error())
	}
	if connection.Type == "" || connection.Type == S3Disabled {
		return FactResult[ResolvedS3Connection]{Value: &ResolvedS3Connection{},
			Diagnostic: FactDiagnostic{State: FactsResolved, Reason: "S3Disabled"}}, nil
	}
	endpoint := connection.Inline
	if connection.Type == S3Reference {
		object := &unstructured.Unstructured{}
		object.SetGroupVersionKind(schema.GroupVersionKind{Group: "s3.kubedoop.dev", Version: "v1alpha1", Kind: "S3Connection"})
		if err := reader.Get(ctx, types.NamespacedName{Namespace: namespace, Name: connection.Reference}, object); err != nil {
			if apierrors.IsNotFound(err) {
				return FactResult[ResolvedS3Connection]{Diagnostic: FactDiagnostic{State: FactsPending,
					Reason: "S3ConnectionMissing", Message: "Waiting for referenced S3Connection"}}, nil
			}
			return FactResult[ResolvedS3Connection]{}, err
		}
		if !object.GetDeletionTimestamp().IsZero() {
			return FactResult[ResolvedS3Connection]{Diagnostic: FactDiagnostic{State: FactsPending,
				Reason: "S3ConnectionDeleting", Message: "Referenced S3Connection is deleting"}}, nil
		}
		var err error
		endpoint, err = decodeS3Resource(object.Object["spec"])
		if err != nil {
			return invalid("Referenced S3Connection is outside the supported endpoint/credential contract")
		}
	}
	resolved, err := resolveS3Endpoint(endpoint)
	if err != nil {
		return invalid(err.Error())
	}
	return FactResult[ResolvedS3Connection]{Value: &resolved,
		Diagnostic: FactDiagnostic{State: FactsResolved, Reason: "S3ConnectionResolved"}}, nil
}

func decodeS3Resource(value any) (S3Endpoint, error) {
	var spec struct {
		Host, Region string
		Port         int32
		PathStyle    bool
		TLS          json.RawMessage
		Credentials  struct {
			SecretClass string
			Scope       struct {
				Node, Pod                 bool
				Services, ListenerVolumes []string
			}
		}
	}
	data, err := json.Marshal(value)
	if err != nil {
		return S3Endpoint{}, err
	}
	if err := json.Unmarshal(data, &spec); err != nil {
		return S3Endpoint{}, err
	}
	endpoint := S3Endpoint{Host: spec.Host, Port: spec.Port, Region: spec.Region, PathStyle: spec.PathStyle,
		Credentials: S3Credentials{SecretClass: spec.Credentials.SecretClass}}
	if len(spec.TLS) != 0 && string(spec.TLS) != "null" {
		var tls struct {
			Verification *struct {
				None   json.RawMessage
				Server *struct {
					CACert *struct {
						WebPKI      *struct{}
						SecretClass string
					}
				}
			}
		}
		if err := json.Unmarshal(spec.TLS, &tls); err != nil {
			return S3Endpoint{}, err
		}
		if tls.Verification != nil && (len(tls.Verification.None) != 0 || tls.Verification.Server == nil ||
			tls.Verification.Server.CACert == nil || tls.Verification.Server.CACert.SecretClass != "" ||
			tls.Verification.Server.CACert.WebPKI == nil) {
			return S3Endpoint{}, fmt.Errorf("only verified system-CA TLS is supported")
		}
		endpoint.TLS = true
	}
	if spec.Credentials.Scope.Node {
		endpoint.Credentials.Scope = append(endpoint.Credentials.Scope, "node")
	}
	if spec.Credentials.Scope.Pod {
		endpoint.Credentials.Scope = append(endpoint.Credentials.Scope, "pod")
	}
	for _, name := range spec.Credentials.Scope.Services {
		endpoint.Credentials.Scope = append(endpoint.Credentials.Scope, "service="+name)
	}
	for _, name := range spec.Credentials.Scope.ListenerVolumes {
		endpoint.Credentials.Scope = append(endpoint.Credentials.Scope, "listener-volume="+name)
	}
	return endpoint, nil
}
