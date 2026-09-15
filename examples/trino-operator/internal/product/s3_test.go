package product

import (
	"context"
	"slices"
	"strconv"
	"testing"

	"github.com/zncdatadev/operator-go/pkg/framework"
)

const testTrinoEtcArgument = "--etc-dir=/etc/trino"

func TestTrinoS3CatalogConsumesResolvedConnectionAndCredentialFiles(t *testing.T) {
	in := effectiveInput()
	in.Config.Product.Hive = HiveCatalog{MetastoreURI: "thrift://metastore.test.svc:9083",
		S3: framework.S3Connection{Type: framework.S3Inline, Inline: framework.S3Endpoint{
			Host: "minio.test.svc", Port: 9000, PathStyle: true,
			Credentials: framework.S3Credentials{SecretName: s3CredentialSlot}}}}
	result, err := ResolveFacts(context.Background(), nil,
		framework.FactInput[TrinoConfig, TrinoClusterConfig, TrinoFacts]{
			Group: in.Group, Config: in.Config, ClusterConfig: in.ClusterConfig, Shared: in.Facts})
	if err != nil || result.Value == nil || result.Diagnostic.State != framework.FactsResolved {
		t.Fatalf("inline connection resolution failed: %+v %v", result, err)
	}
	in.Facts = *result.Value
	if err := validateTrinoInput(in); err != nil {
		t.Fatal(err)
	}
	runtime, err := generateTrino(in)
	if err != nil {
		t.Fatal(err)
	}
	catalog := findFile(runtime.Files, trinoConfigDirectory, "catalog/hive.properties")
	for key, want := range map[string]string{
		"fs.native-s3.enabled": strconv.FormatBool(true), "s3.endpoint": "http://minio.test.svc:9000",
		"s3.path-style-access": strconv.FormatBool(true), "s3.region": "us-east-1",
		"s3.aws-access-key": "${ENV:TRINO_S3_ACCESS_KEY}", "s3.aws-secret-key": "${ENV:TRINO_S3_SECRET_KEY}",
	} {
		if got, known := literalProperty(catalog, key); !known || got != want {
			t.Fatalf("Trino 476 catalog %s: got %q want %q", key, got, want)
		}
	}
	if !slices.Equal(runtime.Main.Command, []string{s3Shell, s3ShellFlags, s3Launcher, trinoLauncher}) ||
		!slices.Equal(runtime.Main.Args, []string{testTrinoEtcArgument, trinoRunArgument}) {
		t.Fatal("credential files have no native process consumer")
	}
	index := slices.IndexFunc(runtime.Directories, func(directory framework.Directory) bool {
		return directory.Name == s3CredentialSlot
	})
	if index < 0 || runtime.Directories[index].Secret == nil ||
		runtime.Directories[index].Secret.SecretName != s3CredentialSlot {
		t.Fatal("credential reference was not declared as a platform directory")
	}
	if !slices.ContainsFunc(runtime.Main.Access, func(access framework.DirectoryAccess) bool {
		return access.Directory == s3CredentialSlot && access.MountPath == s3CredentialPath && access.ReadOnly
	}) {
		t.Fatal("native launcher credential path is not mounted read-only")
	}
}
