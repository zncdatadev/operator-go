package product

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"

	"github.com/zncdatadev/operator-go/pkg/framework"
)

type TrinoFacts struct {
	Catalogs map[string]map[string]string `json:"catalogs"`
}

// Definition exercises the existing Trino declaration and ConfigMap catalog
// reference through the formal external registration. Envtest has no Trino JVM.
func Definition() framework.ProductDefinition[TrinoConfig, TrinoClusterConfig, TrinoFacts] {
	role := framework.RoleDefinition[TrinoConfig]{
		Config: framework.Config[TrinoConfig]{Product: TrinoConfig{HTTPPort: 8080},
			Common: framework.CommonConfig{Resources: framework.Resources{
				CPU:    framework.CPU{Min: resource.MustParse("100m"), Max: resource.MustParse("1")},
				Memory: framework.Memory{Limit: resource.MustParse("1Gi")},
			}},
		},
	}
	return framework.ProductDefinition[TrinoConfig, TrinoClusterConfig, TrinoFacts]{
		Name: "trino", ClusterConfigDefaults: TrinoClusterConfig{NodeEnvironment: "consumer"},
		ImageDefaults: framework.ImageConfig{Custom: "trinodb/trino:476", PullPolicy: corev1.PullIfNotPresent},
		Roles:         map[string]framework.RoleDefinition[TrinoConfig]{"coordinators": role, "workers": role},
		GenerateGroup: generate,
	}
}

func generate(in framework.EffectiveInput[TrinoConfig, TrinoClusterConfig, TrinoFacts]) (
	framework.RuntimeDescription, error,
) {
	r := framework.RuntimeDescription{
		ConfigDirectory: "config", Directories: []framework.Directory{{Name: "config"}},
		Main: framework.Process{Name: "trino", Image: in.Image.Reference, Command: []string{"launcher", "run"},
			Access: []framework.DirectoryAccess{{Directory: "config", MountPath: "/etc/trino", ReadOnly: true}}},
		Endpoints: []framework.Endpoint{{Name: "http", Port: in.Config.Product.HTTPPort}},
	}
	names := make([]string, 0, len(in.Facts.Catalogs))
	for name := range in.Facts.Catalogs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		values := make(map[string]framework.PropertyValue, len(in.Facts.Catalogs[name]))
		for key, value := range in.Facts.Catalogs[name] {
			values[key] = framework.Literal(value)
		}
		r.Files = append(r.Files, framework.File{Directory: "config", Path: "catalog/" + name + ".properties",
			Content: framework.KeyValues{Codec: framework.PropertiesCodec{}, Values: values}})
	}
	return r, nil
}

// ResolveCatalogs is a bounded test adapter for the fixed same-namespace reference.
// The framework supplies read provenance and refresh; no writable client is held.
func ResolveCatalogs(ctx context.Context, reader framework.FactsReader,
	in framework.FactInput[TrinoConfig, TrinoClusterConfig, TrinoFacts],
) (framework.FactResult[TrinoFacts], error) {
	facts := in.Shared
	if name := in.Config.Product.CatalogConfigMapName; name != "" {
		cm := &corev1.ConfigMap{}
		if err := reader.Get(ctx, types.NamespacedName{Namespace: in.Group.Namespace, Name: name}, cm); err != nil {
			if apierrors.IsNotFound(err) {
				return framework.FactResult[TrinoFacts]{Diagnostic: framework.FactDiagnostic{
					State: framework.FactsPending, Reason: "CatalogMissing"}}, nil
			}
			return framework.FactResult[TrinoFacts]{}, err
		}
		if !cm.DeletionTimestamp.IsZero() {
			return framework.FactResult[TrinoFacts]{Diagnostic: framework.FactDiagnostic{
				State: framework.FactsPending, Reason: "CatalogDeleting"}}, nil
		}
		var catalogs map[string]map[string]string
		if err := json.Unmarshal([]byte(cm.Data["catalogs.json"]), &catalogs); err != nil || len(catalogs) == 0 {
			return invalidCatalog(), nil
		}
		facts.Catalogs = catalogs
		for name, entries := range facts.Catalogs {
			if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") ||
				strings.TrimSpace(entries["connector.name"]) == "" {
				return invalidCatalog(), nil
			}
		}
	}
	return framework.FactResult[TrinoFacts]{Value: &facts,
		Diagnostic: framework.FactDiagnostic{State: framework.FactsResolved}}, nil
}
func invalidCatalog() framework.FactResult[TrinoFacts] {
	return framework.FactResult[TrinoFacts]{Diagnostic: framework.FactDiagnostic{
		State: framework.FactsInvalid, Reason: "CatalogInvalid"}}
}
