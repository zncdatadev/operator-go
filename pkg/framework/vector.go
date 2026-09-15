package framework

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation"
)

// VectorDestination is a resolved, non-secret Vector protocol endpoint. It is
// platform data, not an arbitrary fragment of Vector configuration.
type VectorDestination struct {
	Address string
}

// Validate checks the deliberately bounded ADDRESS discovery contract. URLs,
// credentials, paths and environment substitutions are not endpoint addresses.
func (d VectorDestination) Validate() error {
	host, port, err := net.SplitHostPort(d.Address)
	if err != nil || strings.TrimSpace(d.Address) != d.Address || host == "" {
		return fmt.Errorf("ADDRESS must contain a host and port")
	}
	number, err := strconv.Atoi(port)
	if err != nil || strings.Trim(port, "0123456789") != "" || number < 1 || number > 65535 {
		return fmt.Errorf("ADDRESS port must be between 1 and 65535")
	}
	if net.ParseIP(host) == nil && len(validation.IsDNS1123Subdomain(host)) != 0 {
		return fmt.Errorf("ADDRESS host must be a DNS name or IP address")
	}
	return nil
}

// ResolveVectorDestination reads the standard same-namespace ConfigMap's
// ADDRESS. NotFound means Pending; invalid data means Invalid; failed API reads
// remain errors. The controller's tracked reader supplies observed UID/RV.
func ResolveVectorDestination(ctx context.Context, reader FactsReader, namespace, name string) (
	FactResult[VectorDestination], error,
) {
	if len(validation.IsDNS1123Subdomain(name)) != 0 {
		return FactResult[VectorDestination]{Diagnostic: FactDiagnostic{State: FactsInvalid,
			Reason: "InvalidVectorReference", Message: "vectorAgentConfigMap must name a ConfigMap"}}, nil
	}
	var config corev1.ConfigMap
	if err := reader.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, &config); err != nil {
		if apierrors.IsNotFound(err) {
			return FactResult[VectorDestination]{Diagnostic: FactDiagnostic{State: FactsPending,
				Reason: "VectorDestinationMissing", Message: "Waiting for vectorAgentConfigMap " + namespace + "/" + name}}, nil
		}
		return FactResult[VectorDestination]{}, err
	}
	destination := VectorDestination{Address: config.Data["ADDRESS"]}
	if err := destination.Validate(); err != nil {
		return FactResult[VectorDestination]{Diagnostic: FactDiagnostic{State: FactsInvalid,
			Reason: "InvalidVectorDestination", Message: "vectorAgentConfigMap " + namespace + "/" + name + ": " + err.Error()}}, nil
	}
	return FactResult[VectorDestination]{Value: &destination, Diagnostic: FactDiagnostic{State: FactsResolved}}, nil
}
