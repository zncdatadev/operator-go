package pipeline

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/zncdatadev/operator-go/pkg/framework"
)

type s3DomainConfig struct {
	Hive struct {
		S3 framework.S3Connection `json:"s3"`
	} `json:"hive"`
}

func TestS3DomainInheritanceAndBranchChange(t *testing.T) {
	defaults := framework.Config[s3DomainConfig]{Common: assemblyCommon(false)}
	defaults.Product.Hive.S3 = framework.S3Connection{Type: framework.S3Inline, Inline: framework.S3Endpoint{
		Host: "default.storage.svc", Region: "us-east-1", Credentials: framework.S3Credentials{SecretName: "credentials"}}}
	role := json.RawMessage(`{"hive":{"s3":{"inline":{"host":"role.storage.svc","pathStyle":true}}}}`)
	inherited, err := ResolveConfig(defaults, role, json.RawMessage(`{"hive":{"s3":{"inline":{"port":9000}}}}`))
	if err != nil {
		t.Fatal(err)
	}
	connection := inherited.Product.Hive.S3
	if connection.Type != framework.S3Inline || connection.Inline.Host != "role.storage.svc" ||
		connection.Inline.Port != 9000 || !connection.Inline.PathStyle ||
		connection.Inline.Credentials.SecretName != "credentials" {
		t.Fatalf("S3 field inheritance failed: %+v", connection)
	}
	referenceLayer := json.RawMessage(`{"hive":{"s3":{"type":"reference","reference":"shared-s3"}}}`)
	referenced, err := ResolveConfig(defaults, role, referenceLayer)
	if err != nil {
		t.Fatal(err)
	}
	connection = referenced.Product.Hive.S3
	if connection.Type != framework.S3Reference || connection.Reference != "shared-s3" ||
		!reflect.DeepEqual(connection.Inline, framework.S3Endpoint{}) {
		t.Fatalf("branch change retained the old endpoint/credentials: %+v", connection)
	}
	disabled, err := ResolveConfig(defaults, role, json.RawMessage(`{"hive":{"s3":{"type":"disabled"}}}`))
	if err != nil || disabled.Product.Hive.S3.Type != framework.S3Disabled {
		t.Fatalf("explicit disable did not clear the inherited connection: %+v %v", disabled, err)
	}
	if defaults.Product.Hive.S3.Inline.Host != "default.storage.svc" {
		t.Fatal("folding mutated product defaults")
	}
}
