package pipeline

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/validation"
	kubernetesjson "sigs.k8s.io/json"
)

// ResolveImage chooses a source by presence, then validates its effective value.
// It does not contact a registry, infer versions from custom, or add a latest tag.
func ResolveImage(productName string, defaults ImageConfig, raw json.RawMessage) (ResolvedImage, error) {
	input, err := decodeImageInput(raw)
	if err != nil {
		return ResolvedImage{}, err
	}
	resolved := ResolvedImage{PullPolicy: defaults.PullPolicy, PullSecretName: defaults.PullSecretName}
	if input.PullPolicy != nil {
		resolved.PullPolicy = *input.PullPolicy
	}
	if input.PullSecretName != nil {
		resolved.PullSecretName = *input.PullSecretName
	}
	switch resolved.PullPolicy {
	case corev1.PullAlways, corev1.PullIfNotPresent, corev1.PullNever:
	default:
		return ResolvedImage{}, fmt.Errorf("image.pullPolicy must explicitly be Always, IfNotPresent or Never")
	}
	if resolved.PullSecretName != "" && len(validation.IsDNS1123Subdomain(resolved.PullSecretName)) != 0 {
		return ResolvedImage{}, fmt.Errorf("image.pullSecretName must be empty or a valid Secret name")
	}
	resolved.Reference, err = resolveImageSource(productName, defaults, input)
	if err != nil {
		return ResolvedImage{}, err
	}
	if err := validateImageReference(resolved.Reference); err != nil {
		return ResolvedImage{}, fmt.Errorf("image reference: %w", err)
	}
	return resolved, nil
}

func decodeImageInput(raw json.RawMessage) (ImageInput, error) {
	var input ImageInput
	if len(raw) == 0 {
		return input, nil
	}
	if err := validateJSON(raw, reflect.TypeFor[ImageConfig](), "image"); err != nil {
		return input, err
	}
	strict, err := kubernetesjson.UnmarshalStrict(raw, &input)
	if failure := errors.Join(err, errors.Join(strict...)); failure != nil {
		return ImageInput{}, fmt.Errorf("image: %w", failure)
	}
	return input, nil
}

func resolveImageSource(productName string, defaults ImageConfig, input ImageInput) (string, error) {
	if input.Custom != nil && *input.Custom != "" {
		return *input.Custom, nil
	}
	structured := input.Custom != nil || input.Repo != nil || input.ProductVersion != nil || input.KubedoopVersion != nil
	if !structured && defaults.Custom != "" {
		return defaults.Custom, nil
	}
	repo, productVersion, kubedoopVersion := defaults.Repo, defaults.ProductVersion, defaults.KubedoopVersion
	for _, field := range []struct {
		input     *string
		effective *string
	}{
		{input.Repo, &repo}, {input.ProductVersion, &productVersion}, {input.KubedoopVersion, &kubedoopVersion},
	} {
		if field.input != nil {
			*field.effective = *field.input
		}
	}
	if repo == "" || productVersion == "" {
		return "", fmt.Errorf("structured image requires nonempty repo and productVersion")
	}
	if !imagePathComponent.MatchString(productName) {
		return "", fmt.Errorf("structured image requires a valid product repository component")
	}
	tag := productVersion
	if kubedoopVersion != "" {
		tag += "-kubedoop" + kubedoopVersion
	}
	if !imageTag.MatchString(tag) {
		return "", fmt.Errorf("image versions must produce a valid tag of at most 128 characters")
	}
	return repo + "/" + productName + ":" + tag, nil
}

var (
	imagePathComponent = regexp.MustCompile(`^[a-z0-9]+(?:(?:[._]|__|-+)[a-z0-9]+)*$`)
	imageTag           = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)
	imageDigest        = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
)

// This is a deliberately bounded reference profile, not a registry resolver:
// lowercase names, DNS/IPv4 registries with optional ports, tags, and SHA-256.
// IPv6 registries and other digest algorithms are rejected explicitly. Grammar
// reference: github.com/distribution/reference v0.6.0, reference.go and regexp.go.
func validateImageReference(reference string) error {
	name, digest, hasDigest := strings.Cut(reference, "@")
	if hasDigest && !imageDigest.MatchString(digest) {
		return fmt.Errorf("only a sha256 digest with 64 lowercase hexadecimal digits is supported")
	}
	if colon := strings.LastIndex(name, ":"); colon > strings.LastIndex(name, "/") {
		if !imageTag.MatchString(name[colon+1:]) {
			return fmt.Errorf("invalid or empty image tag")
		}
		name = name[:colon]
	}
	if len(name) == 0 || len(name) > 255 {
		return fmt.Errorf("repository name must contain 1 to 255 characters")
	}
	components := strings.Split(name, "/")
	if len(components) > 1 && (strings.ContainsAny(components[0], ".:") || components[0] == "localhost") {
		if err := validateImageRegistry(components[0]); err != nil {
			return err
		}
		components = components[1:]
	}
	for _, component := range components {
		if !imagePathComponent.MatchString(component) {
			return fmt.Errorf("repository path must contain lowercase image name components")
		}
	}
	return nil
}

func validateImageRegistry(registry string) error {
	host, port, hasPort := strings.Cut(registry, ":")
	if len(validation.IsDNS1123Subdomain(host)) != 0 {
		return fmt.Errorf("image registry must be a lowercase DNS name or IPv4 address; IPv6 is unsupported")
	}
	if hasPort {
		for _, char := range port {
			if char < '0' || char > '9' {
				return fmt.Errorf("image registry port must be numeric")
			}
		}
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return fmt.Errorf("image registry port must be between 1 and 65535")
		}
	}
	return nil
}
