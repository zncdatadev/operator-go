package pipeline

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

const (
	imageTestProduct = "trino"
	imageTestCustom  = "registry.example/team/custom:fixed"
)

func imageTestDefaults() ImageConfig {
	return ImageConfig{Custom: imageTestCustom, Repo: "quay.io/zncdatadev",
		ProductVersion: "476", KubedoopVersion: "0.2.0", PullPolicy: corev1.PullIfNotPresent, PullSecretName: "pull-secret"}
}

func TestImageSourceSelectionUsesPresence(t *testing.T) {
	for _, item := range []struct{ name, raw, reference string }{
		{"omitted", "", imageTestCustom},
		{"empty object", `{}`, imageTestCustom},
		{"policy only", `{"pullPolicy":"Never"}`, imageTestCustom},
		{"secret only", `{"pullSecretName":"other"}`, imageTestCustom},
		{"custom wins", `{"custom":"busybox:stable","repo":"","productVersion":""}`, "busybox:stable"},
		{"clear custom", `{"custom":""}`, "quay.io/zncdatadev/trino:476-kubedoop0.2.0"},
		{"repo switches source", `{"repo":"localhost:5000/team"}`, "localhost:5000/team/trino:476-kubedoop0.2.0"},
		{"product switches source", `{"productVersion":"477"}`, "quay.io/zncdatadev/trino:477-kubedoop0.2.0"},
		{"kubedoop switches source", `{"kubedoopVersion":"1"}`, "quay.io/zncdatadev/trino:476-kubedoop1"},
		{"clear kubedoop", `{"kubedoopVersion":""}`, "quay.io/zncdatadev/trino:476"},
	} {
		t.Run(item.name, func(t *testing.T) {
			defaults := imageTestDefaults()
			before := defaults
			raw := json.RawMessage(item.raw)
			resolved, err := ResolveImage(imageTestProduct, defaults, raw)
			if err != nil || resolved.Reference != item.reference {
				t.Fatalf("wrong selected source: %+v %v", resolved, err)
			}
			if defaults != before || string(raw) != item.raw {
				t.Fatal("image resolution mutated its inputs")
			}
		})
	}
	defaults := imageTestDefaults()
	defaults.Custom = ""
	resolved, err := ResolveImage(imageTestProduct, defaults, nil)
	if err != nil || resolved.Reference != "quay.io/zncdatadev/trino:476-kubedoop0.2.0" {
		t.Fatalf("default structured source was not inherited: %+v %v", resolved, err)
	}
}

func TestImageExplicitEmptyValuesAreNotOmission(t *testing.T) {
	for _, raw := range []string{`{"repo":""}`, `{"productVersion":""}`, `{"pullPolicy":""}`} {
		if _, err := ResolveImage(imageTestProduct, imageTestDefaults(), json.RawMessage(raw)); err == nil {
			t.Fatalf("explicit empty required value was ignored: %s", raw)
		}
	}
	resolved, err := ResolveImage(imageTestProduct, imageTestDefaults(), json.RawMessage(`{"pullSecretName":""}`))
	if err != nil || resolved.PullSecretName != "" || resolved.Reference != imageTestDefaults().Custom {
		t.Fatalf("pull Secret could not be cleared independently: %+v %v", resolved, err)
	}
	empty := ""
	input := ImageInput{Custom: &empty, KubedoopVersion: &empty, PullSecretName: &empty}
	raw, err := json.Marshal(input)
	if err != nil || string(raw) != `{"custom":"","kubedoopVersion":"","pullSecretName":""}` {
		t.Fatalf("input pointers lost explicit empty strings: %s %v", raw, err)
	}
	var decoded ImageInput
	if err := json.Unmarshal(raw, &decoded); err != nil || !reflect.DeepEqual(decoded, input) {
		t.Fatalf("image presence roundtrip failed: %+v %v", decoded, err)
	}
}

func TestImagePullPolicyIsExplicitAndIndependent(t *testing.T) {
	for _, policy := range []corev1.PullPolicy{corev1.PullAlways, corev1.PullIfNotPresent, corev1.PullNever} {
		defaults := imageTestDefaults()
		defaults.PullPolicy = ""
		raw, err := json.Marshal(ImageInput{PullPolicy: &policy})
		if err != nil {
			t.Fatal(err)
		}
		resolved, err := ResolveImage(imageTestProduct, defaults, raw)
		if err != nil || resolved.PullPolicy != policy || resolved.Reference != defaults.Custom {
			t.Fatalf("explicit policy could not fill an omitted default: %+v %v", resolved, err)
		}
	}
	for _, policy := range []corev1.PullPolicy{"", "always", "Sometimes"} {
		defaults := imageTestDefaults()
		defaults.PullPolicy = policy
		if _, err := ResolveImage(imageTestProduct, defaults, nil); err == nil {
			t.Fatalf("invalid effective policy was guessed or accepted: %q", policy)
		}
	}
	defaults := imageTestDefaults()
	defaults.PullPolicy, defaults.PullSecretName = "invalid", "INVALID"
	resolved, err := ResolveImage(imageTestProduct, defaults,
		json.RawMessage(`{"pullPolicy":"Always","pullSecretName":""}`))
	if err != nil || resolved.PullPolicy != corev1.PullAlways || resolved.PullSecretName != "" {
		t.Fatalf("explicit valid values could not repair defaults: %+v %v", resolved, err)
	}
}

func TestImageInputRejectsUnknownNullAndDuplicate(t *testing.T) {
	for _, raw := range []string{
		`null`, `[]`, `"image"`, `{`, `{"unknown":"x"}`, `{"Custom":"busybox"}`,
		`{"custom":null}`, `{"repo":null}`, `{"productVersion":null}`, `{"kubedoopVersion":null}`,
		`{"pullPolicy":null}`, `{"pullSecretName":null}`, `{"custom":123}`,
		`{"custom":"first:1","custom":"second:2"}`, `{"repo":"one","repo":"two"}`,
		`{"pullSecretName":"first","pullSecretName":""}`,
	} {
		if _, err := ResolveImage(imageTestProduct, imageTestDefaults(), json.RawMessage(raw)); err == nil {
			t.Fatalf("ambiguous or malformed image input was accepted: %s", raw)
		}
	}
}

func TestImageCustomReferencesArePreservedWithoutVersionInference(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	for _, reference := range []string{
		"busybox", "busybox:latest", "quay.io/zncdatadev/trino:476", "localhost:5000/team/trino:v1",
		"127.0.0.1:5000/team/image:Tag_1", "team/part__name--next:stable", "trino@" + digest, "trino:476@" + digest,
	} {
		raw, err := json.Marshal(map[string]string{"custom": reference})
		if err != nil {
			t.Fatal(err)
		}
		resolved, err := ResolveImage("", imageTestDefaults(), raw)
		if err != nil || resolved.Reference != reference {
			t.Fatalf("custom reference was rewritten or rejected: %q %+v %v", reference, resolved, err)
		}
	}
}

func TestImageRejectsInvalidSelectedReferences(t *testing.T) {
	for _, reference := range []string{
		" ", "https://quay.io/team/trino:476", "quay.io/Team/trino:476", "trino:", "trino:bad tag",
		"team//trino:1", "team/../trino:1", "team/trino:1\n", "localhost:0/trino", "localhost:65536/trino",
		"localhost:abc/trino", "trino@sha256:abc", "trino@sha512:" + strings.Repeat("a", 128),
		"trino@sha256:" + strings.Repeat("a", 64) + "@sha256:" + strings.Repeat("b", 64),
		"[::1]:5000/trino:1", "trino:" + strings.Repeat("a", 129), strings.Repeat("a", 256),
	} {
		raw, err := json.Marshal(map[string]string{"custom": reference})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ResolveImage(imageTestProduct, imageTestDefaults(), raw); err == nil {
			t.Fatalf("invalid reference was accepted: %q", reference)
		}
	}
	for _, raw := range []string{
		`{"repo":"quay.io/team/"}`, `{"productVersion":"bad/tag"}`, `{"kubedoopVersion":"bad tag"}`,
		`{"pullSecretName":"Bad_Name"}`,
	} {
		if _, err := ResolveImage(imageTestProduct, imageTestDefaults(), json.RawMessage(raw)); err == nil {
			t.Fatalf("invalid effective image setting was accepted: %s", raw)
		}
	}
	if _, err := ResolveImage("../other", imageTestDefaults(), json.RawMessage(`{"custom":""}`)); err == nil {
		t.Fatal("product name escaped its repository component")
	}
}
