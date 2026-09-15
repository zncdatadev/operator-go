package product

import (
	"context"
	"fmt"

	"github.com/zncdatadev/operator-go/pkg/framework"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
)

const (
	trinoInternalSecretKey   = "shared-secret"
	trinoTLSRuntimeDirectory = "tls-runtime"
	trinoTLSSourceDirectory  = "tls-source"
	trinoHTTPSEndpoint       = "https"
)

// AuthenticationFacts contains references only. Secret data stays in Kubernetes
// Secret volumes/env selectors, never in generated config, shared facts or status.
type AuthenticationFacts struct {
	PasswordSecret string `json:"passwordSecret"`
}

func resolveTrinoAuthentication(ctx context.Context, reader framework.FactsReader,
	in framework.FactInput[TrinoConfig, TrinoClusterConfig, TrinoFacts], facts *TrinoFacts,
) (framework.FactDiagnostic, error) {
	facts.Authentication = AuthenticationFacts{}
	references := in.Platform.Authentication
	if in.ClusterConfig.TLSSecret != "" && in.ClusterConfig.TLSSecretClass != "" {
		return authDiagnostic(framework.FactsInvalid, "MultipleTLSCertificateSources"), nil
	}
	for _, source := range []struct {
		name string
		keys []string
	}{{in.ClusterConfig.InternalSecret, []string{trinoInternalSecretKey}}, {in.ClusterConfig.TLSSecret, []string{"tls.crt", "tls.key"}}} {
		if source.name == "" {
			continue
		}
		if _, diagnostic, err := authenticationSecret(ctx, reader, in.Group.Namespace, source.name, source.keys); err != nil || diagnostic.State != framework.FactsResolved {
			return diagnostic, err
		}
	}
	if len(references) == 0 {
		return framework.FactDiagnostic{State: framework.FactsResolved}, nil
	}
	if len(references) != 1 {
		return authDiagnostic(framework.FactsInvalid, "UnsupportedAuthenticationCombination"), nil
	}
	if references[0].OIDC.ClientCredentialsSecret != "" || len(references[0].OIDC.ExtraScopes) != 0 {
		return authDiagnostic(framework.FactsInvalid, "UnsupportedTrinoOIDCSettings"), nil
	}
	result, err := framework.ResolveAuthenticationClass(ctx, reader, references[0].AuthenticationClass)
	if err != nil || result.Diagnostic.State != framework.FactsResolved {
		return result.Diagnostic, err
	}
	if result.Value.Static == nil {
		return authDiagnostic(framework.FactsInvalid, "UnsupportedTrinoAuthenticationProvider"), nil
	}
	if in.ClusterConfig.InternalSecret == "" || (in.ClusterConfig.TLSSecret == "") == (in.ClusterConfig.TLSSecretClass == "") {
		return authDiagnostic(framework.FactsInvalid, "AuthenticationRequiresTLSAndInternalSecret"), nil
	}
	name := result.Value.Static.UserCredentialsSecret.Name
	password, diagnostic, err := authenticationSecret(ctx, reader, in.Group.Namespace, name, []string{"password.db"})
	if err != nil || diagnostic.State != framework.FactsResolved {
		return diagnostic, err
	}
	// Read only the key's shape; never retain or echo password hashes in facts.
	if len(password.Data["password.db"]) == 0 {
		return authDiagnostic(framework.FactsInvalid, "EmptyPasswordDatabase"), nil
	}
	facts.Authentication.PasswordSecret = name
	return framework.FactDiagnostic{State: framework.FactsResolved, Reason: "TrinoPasswordAuthenticationResolved"}, nil
}

func authDiagnostic(state framework.FactState, reason string) framework.FactDiagnostic {
	return framework.FactDiagnostic{State: state, Reason: reason, Message: "Trino authentication configuration is unavailable or invalid; secret values are not reported"}
}

func authenticationSecret(ctx context.Context, reader framework.FactsReader, namespace, name string, keys []string) (
	*corev1.Secret, framework.FactDiagnostic, error,
) {
	object := &corev1.Secret{}
	if err := reader.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, object); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, authDiagnostic(framework.FactsPending, "AuthenticationSecretMissing"), nil
		}
		return nil, framework.FactDiagnostic{}, err
	}
	if !object.DeletionTimestamp.IsZero() {
		return nil, authDiagnostic(framework.FactsPending, "AuthenticationSecretDeleting"), nil
	}
	for _, key := range keys {
		if len(object.Data[key]) == 0 {
			return nil, authDiagnostic(framework.FactsInvalid, "AuthenticationSecretKeyMissing"), nil
		}
	}
	return object, framework.FactDiagnostic{State: framework.FactsResolved}, nil
}

const prepareTrinoTLS = `import os,pathlib,tempfile
source=pathlib.Path('/kubedoop/tls-source'); destination=pathlib.Path('/kubedoop/tls-runtime')
content=(source/'tls.key').read_bytes()+b'\n'+(source/'tls.crt').read_bytes()
fd,name=tempfile.mkstemp(dir=destination)
try:
 with os.fdopen(fd,'wb') as output:
  output.write(content);output.flush();os.fsync(output.fileno())
 os.chmod(name,0o600);os.replace(name,destination/'server.pem')
finally:
 if os.path.exists(name):os.unlink(name)
`

func configureTrinoAuthentication(r *framework.RuntimeDescription,
	in framework.EffectiveInput[TrinoConfig, TrinoClusterConfig, TrinoFacts],
) error {
	authenticated := in.Facts.Authentication.PasswordSecret != ""
	if !authenticated && len(in.Platform.Authentication) > 0 {
		return fmt.Errorf("authentication input has no resolved Trino consumer")
	}
	configuration := findFile(r.Files, trinoConfigDirectory, "config.properties").Content.(framework.KeyValues)
	if in.ClusterConfig.InternalSecret != "" {
		r.Main.Env = append(r.Main.Env, corev1.EnvVar{Name: "TRINO_INTERNAL_SHARED_SECRET", ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{LocalObjectReference: corev1.LocalObjectReference{Name: in.ClusterConfig.InternalSecret}, Key: trinoInternalSecretKey}}})
		configuration.Values["internal-communication.shared-secret"] = framework.Literal("${ENV:TRINO_INTERNAL_SHARED_SECRET}")
	}
	if in.Group.Role != trinoCoordinatorRole || (in.ClusterConfig.TLSSecret == "" && in.ClusterConfig.TLSSecretClass == "") {
		return nil
	}
	if authenticated {
		configuration.Values["http-server.authentication.type"] = framework.Literal("PASSWORD")
	}
	configuration.Values["http-server.https.enabled"] = framework.Literal("true")
	configuration.Values["http-server.https.port"] = framework.Literal("8443")
	configuration.Values["http-server.https.keystore.path"] = framework.Literal("/kubedoop/tls-runtime/server.pem")
	secret := &framework.SecretVolume{SecretName: in.ClusterConfig.TLSSecret}
	if in.ClusterConfig.TLSSecretClass != "" {
		scope := []string{"pod", "service=" + in.Group.ServiceName()}
		if in.ClusterConfig.ListenerClass != "" {
			// The published Listener address may differ from the group Service DNS.
			scope = append(scope, "listener-volume="+trinoListenerDirectory)
		}
		secret = &framework.SecretVolume{SecretClass: in.ClusterConfig.TLSSecretClass, Format: "tls-pem", Scope: scope}
	}
	r.Directories = append(r.Directories, framework.Directory{Name: trinoTLSSourceDirectory, Secret: secret}, framework.Directory{Name: trinoTLSRuntimeDirectory})
	r.Main.Access = append(r.Main.Access, framework.DirectoryAccess{Directory: trinoTLSRuntimeDirectory, MountPath: "/kubedoop/tls-runtime", ReadOnly: true})
	r.Initializers = append(r.Initializers, framework.Process{Name: "initialize-tls", Command: []string{trinoPython, "-c", prepareTrinoTLS}, Identity: r.Main.Identity.DeepCopy(),
		Access: []framework.DirectoryAccess{{Directory: trinoTLSSourceDirectory, MountPath: "/kubedoop/tls-source", ReadOnly: true}, {Directory: trinoTLSRuntimeDirectory, MountPath: "/kubedoop/tls-runtime"}}})
	if authenticated {
		r.Directories = append(r.Directories, framework.Directory{Name: "authentication", Secret: &framework.SecretVolume{SecretName: in.Facts.Authentication.PasswordSecret}})
		r.Main.Access = append(r.Main.Access, framework.DirectoryAccess{Directory: "authentication", MountPath: "/kubedoop/authentication", ReadOnly: true})
		r.Files = append(r.Files, trinoFile("password-authenticator.properties", map[string]framework.PropertyValue{
			"password-authenticator.name": framework.Literal("file"), "file.password-file": framework.Literal("/kubedoop/authentication/password.db"),
			"file.refresh-period": framework.Literal("5s"),
		}))
	}
	r.Endpoints = append(r.Endpoints, framework.Endpoint{Name: trinoHTTPSEndpoint, Port: 8443})
	return nil
}
