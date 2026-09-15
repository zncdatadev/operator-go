package product

import (
	"context"
	"fmt"
	"unicode/utf8"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
	kubernetesjson "sigs.k8s.io/json"

	"github.com/zncdatadev/operator-go/pkg/framework"
)

const trinoCatalogSourceKey = "catalogs.json"

// ResolveFacts is the sample product adapter used by generated registration.
// It is not a generic framework policy or a plugin/credential health check.
func resolveCatalogFacts(ctx context.Context, reader framework.FactsReader,
	in framework.FactInput[TrinoConfig, TrinoClusterConfig, TrinoFacts],
) (framework.FactResult[TrinoFacts], error) {
	facts := cloneFacts(in.Shared)
	name := in.Config.Product.CatalogConfigMapName
	if name == "" {
		return resolvedTrinoFacts(facts, "NoCatalogReference"), nil
	}
	if len(validation.IsDNS1123Subdomain(name)) != 0 {
		return framework.FactResult[TrinoFacts]{Diagnostic: framework.FactDiagnostic{State: framework.FactsInvalid,
			Reason: "InvalidCatalogReference", Message: "Catalog ConfigMap name is invalid"}}, nil
	}
	cm := &corev1.ConfigMap{}
	if err := reader.Get(ctx, types.NamespacedName{Namespace: in.Group.Namespace, Name: name}, cm); err != nil {
		if apierrors.IsNotFound(err) {
			return framework.FactResult[TrinoFacts]{Diagnostic: framework.FactDiagnostic{State: framework.FactsPending,
				Reason: "CatalogSourceMissing", Message: "Referenced catalog ConfigMap does not exist"}}, nil
		}
		return framework.FactResult[TrinoFacts]{}, err
	}
	if !cm.DeletionTimestamp.IsZero() {
		return framework.FactResult[TrinoFacts]{Diagnostic: framework.FactDiagnostic{State: framework.FactsPending,
			Reason: "CatalogSourceDeleting", Message: "Referenced catalog ConfigMap is deleting"}}, nil
	}
	raw, found := cm.Data[trinoCatalogSourceKey]
	if !found {
		return framework.FactResult[TrinoFacts]{Diagnostic: framework.FactDiagnostic{State: framework.FactsInvalid,
			Reason: "CatalogSourceKeyMissing", Message: "Referenced ConfigMap has no catalogs.json data key"}}, nil
	}
	catalogs, err := parseTrinoCatalogs(raw)
	if err != nil {
		// Catalog properties may contain credentials. Diagnostics identify the
		// dependency separately; they never echo values or parser excerpts.
		return framework.FactResult[TrinoFacts]{Diagnostic: framework.FactDiagnostic{State: framework.FactsInvalid,
			Reason: "InvalidCatalogSource", Message: "Referenced catalogs.json is not a valid catalog document"}}, nil
	}
	facts.Catalogs = catalogs
	return resolvedTrinoFacts(facts, "CatalogSourceResolved"), nil
}

func resolvedTrinoFacts(facts TrinoFacts, reason string) framework.FactResult[TrinoFacts] {
	return framework.FactResult[TrinoFacts]{Value: &facts, Diagnostic: framework.FactDiagnostic{
		State: framework.FactsResolved, Reason: reason, Message: "Catalog generation facts are resolved; loading is not observed",
	}}
}

func parseTrinoCatalogs(raw string) (map[string]map[string]string, error) {
	// Pointer values distinguish JSON null from a valid empty string. Strict
	// decoding also rejects duplicate catalog/property keys instead of taking last.
	var document map[string]map[string]*string
	strict, err := kubernetesjson.UnmarshalStrict([]byte(raw), &document)
	if err != nil || len(strict) != 0 || document == nil || !utf8.ValidString(raw) {
		return nil, fmt.Errorf("catalog document must be a strict JSON object")
	}
	catalogs := make(map[string]map[string]string, len(document))
	for name, properties := range document {
		if properties == nil {
			return nil, fmt.Errorf("catalog properties cannot be null")
		}
		catalogs[name] = make(map[string]string, len(properties))
		for key, value := range properties {
			if value == nil {
				return nil, fmt.Errorf("catalog property value cannot be null")
			}
			catalogs[name][key] = *value
		}
	}
	if err := ValidateTrinoCatalogs(catalogs); err != nil {
		return nil, err
	}
	return catalogs, nil
}

// ResolveFacts composes independent, exact platform/product resolutions. Pending
// or invalid authentication never produces a partial execution fact set.
func ResolveFacts(ctx context.Context, reader framework.FactsReader,
	in framework.FactInput[TrinoConfig, TrinoClusterConfig, TrinoFacts],
) (framework.FactResult[TrinoFacts], error) {
	result, err := resolveCatalogFacts(ctx, reader, in)
	if err != nil || result.Diagnostic.State != framework.FactsResolved {
		return result, err
	}
	diagnostic, err := resolveTrinoAuthentication(ctx, reader, in, result.Value)
	if err != nil || diagnostic.State != framework.FactsResolved {
		return framework.FactResult[TrinoFacts]{Diagnostic: diagnostic}, err
	}
	diagnostic, err = resolveTrinoS3(ctx, reader, in, result.Value)
	if err != nil || diagnostic.State != framework.FactsResolved {
		return framework.FactResult[TrinoFacts]{Diagnostic: diagnostic}, err
	}
	return result, nil
}
