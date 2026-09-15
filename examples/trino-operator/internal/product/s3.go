package product

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"

	"github.com/zncdatadev/operator-go/pkg/framework"
	corev1 "k8s.io/api/core/v1"
)

// HiveCatalog is product intent; connection selection and inheritance belong to
// the framework's S3 domain. Its catalog name is fixed so overrides stay explicit.
type HiveCatalog struct {
	MetastoreURI string                 `json:"metastoreURI"`
	S3           framework.S3Connection `json:"s3"`
}

const (
	s3CatalogName       = "hive"
	s3CredentialSlot    = "s3-credentials"
	s3CredentialPath    = "/kubedoop/s3-credentials"
	s3AccessEnvironment = "TRINO_S3_ACCESS_KEY"
	s3SecretEnvironment = "TRINO_S3_SECRET_KEY"
	s3ShellFlags        = "-ec"
	s3Shell             = "/bin/sh"
)

// The script contains only fixed paths and variable names. It does not evaluate
// file content as shell code or write credential bytes into a ConfigMap.
const s3Launcher = `TRINO_S3_ACCESS_KEY="$(cat /kubedoop/s3-credentials/ACCESS_KEY)"
TRINO_S3_SECRET_KEY="$(cat /kubedoop/s3-credentials/SECRET_KEY)"
test -n "$TRINO_S3_ACCESS_KEY"
test -n "$TRINO_S3_SECRET_KEY"
export TRINO_S3_ACCESS_KEY TRINO_S3_SECRET_KEY
exec "$0" "$@"`

func resolveTrinoS3(ctx context.Context, reader framework.FactsReader,
	in framework.FactInput[TrinoConfig, TrinoClusterConfig, TrinoFacts], facts *TrinoFacts,
) (framework.FactDiagnostic, error) {
	hive := in.Config.Product.Hive
	if hive.S3.Type == "" || hive.S3.Type == framework.S3Disabled {
		if hive.MetastoreURI != "" {
			return framework.FactDiagnostic{State: framework.FactsInvalid, Reason: "InvalidHiveS3",
				Message: "Hive metastoreURI requires an enabled S3 connection"}, nil
		}
		facts.S3 = framework.ResolvedS3Connection{}
		return framework.FactDiagnostic{State: framework.FactsResolved, Reason: "HiveS3Disabled"}, nil
	}
	if err := validateMetastoreURI(hive.MetastoreURI); err != nil {
		return framework.FactDiagnostic{State: framework.FactsInvalid, Reason: "InvalidHiveMetastore",
			Message: err.Error()}, nil
	}
	result, err := framework.ResolveS3Connection(ctx, reader, in.Group.Namespace, hive.S3)
	if err != nil || result.Diagnostic.State != framework.FactsResolved {
		return result.Diagnostic, err
	}
	if _, exists := facts.Catalogs[s3CatalogName]; exists {
		return framework.FactDiagnostic{State: framework.FactsInvalid, Reason: "ConflictingHiveCatalog",
			Message: "The typed Hive/S3 input and catalog source both declare hive"}, nil
	}
	facts.S3 = *result.Value
	if facts.Catalogs == nil {
		facts.Catalogs = map[string]map[string]string{}
	}
	// These names are from Trino tag 476, not the current documentation's
	// renamed filesystem gate. The consumer uses Airlift env substitution.
	facts.Catalogs[s3CatalogName] = map[string]string{
		"connector.name": s3CatalogName, "hive.metastore.uri": hive.MetastoreURI,
		"fs.native-s3.enabled": strconv.FormatBool(true), "s3.endpoint": facts.S3.Endpoint, "s3.region": facts.S3.Region,
		"s3.path-style-access": strconv.FormatBool(facts.S3.PathStyle),
		"s3.aws-access-key":    "${ENV:" + s3AccessEnvironment + "}",
		"s3.aws-secret-key":    "${ENV:" + s3SecretEnvironment + "}",
	}
	return result.Diagnostic, nil
}

func validateMetastoreURI(value string) error {
	uri, err := url.Parse(value)
	if err != nil || uri.Scheme != "thrift" || uri.User != nil || uri.Path != "" || uri.RawQuery != "" || uri.Fragment != "" {
		return fmt.Errorf("hive.metastoreURI must be a single thrift://host:port endpoint")
	}
	host, port, err := net.SplitHostPort(uri.Host)
	if err != nil || host == "" {
		return fmt.Errorf("hive.metastoreURI requires host and port")
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return fmt.Errorf("hive.metastoreURI has an invalid port")
	}
	return nil
}

func configureTrinoS3(runtime *framework.RuntimeDescription,
	in framework.EffectiveInput[TrinoConfig, TrinoClusterConfig, TrinoFacts],
) error {
	if in.Facts.S3.Endpoint == "" {
		if in.Config.Product.Hive.S3.Type != "" && in.Config.Product.Hive.S3.Type != framework.S3Disabled {
			return fmt.Errorf("enabled Hive/S3 input has no resolved connection facts")
		}
		return nil
	}
	credentials := in.Facts.S3.Credentials
	runtime.Directories = append(runtime.Directories, framework.Directory{Name: s3CredentialSlot,
		Secret: &framework.SecretVolume{SecretName: credentials.SecretName, SecretClass: credentials.SecretClass,
			Scope: credentials.Scope}})
	runtime.Main.Access = append(runtime.Main.Access,
		framework.DirectoryAccess{Directory: s3CredentialSlot, MountPath: s3CredentialPath, ReadOnly: true})
	// The fixed launcher remains part of Command ($0), so replacing CLI Args
	// cannot turn the first launcher option into the executable.
	runtime.Main.Command = append([]string{s3Shell, s3ShellFlags, s3Launcher}, runtime.Main.Command...)
	// Endpoint/region/addressing changes must reach a fresh native process even
	// when an installation has not opted into general ConfigMap restarts.
	runtime.Main.Env = append(runtime.Main.Env, corev1.EnvVar{Name: "TRINO_S3_CONNECTION",
		Value: in.Facts.S3.Endpoint + "/" + in.Facts.S3.Region + "/" + strconv.FormatBool(in.Facts.S3.PathStyle)})
	return nil
}
