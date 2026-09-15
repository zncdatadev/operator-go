package pipeline

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"slices"
	"strings"
	"unicode/utf8"

	"k8s.io/apimachinery/pkg/util/validation"
)

const (
	materializationVersion = "v1"
	propertiesCodecID      = "properties-v1"
)

// MaterializationPlan is data, never a shell script or interpolation template.
// Files are relative to root/<Directory>; each file has exactly one content form.
type MaterializationPlan struct {
	Version string        `json:"version"`
	Files   []PlannedFile `json:"files"`
}

type PlannedFile struct {
	Directory  string             `json:"directory"`
	Path       string             `json:"path"`
	Encoded    *string            `json:"encoded,omitempty"`
	Properties *PlannedProperties `json:"properties,omitempty"`
}

type PlannedProperties struct {
	Codec  string                     `json:"codec"`
	Values map[string]PlannedProperty `json:"values"`
}

type PlannedProperty struct {
	Literal *string `json:"literal,omitempty"`
	PodName bool    `json:"podName,omitempty"`
}

// PrepareMaterialization encodes all static content. Only built-in PropertiesCodec
// may carry unresolved bindings across the process boundary; arbitrary Go codecs
// remain usable for static files and are never guessed from a file extension.
func PrepareMaterialization(files []File) (MaterializationPlan, error) {
	plan := MaterializationPlan{Version: materializationVersion, Files: make([]PlannedFile, 0, len(files))}
	for _, file := range files {
		if err := validateContent(file.Content); err != nil {
			return MaterializationPlan{}, fmt.Errorf("file %s/%s: %w", file.Directory, file.Path, err)
		}
		item := PlannedFile{Directory: file.Directory, Path: file.Path}
		if err := prepareFileContent(&item, file.Content); err != nil {
			return MaterializationPlan{}, fmt.Errorf("file %s/%s: %w", file.Directory, file.Path, err)
		}
		plan.Files = append(plan.Files, item)
	}
	slices.SortFunc(plan.Files, func(a, b PlannedFile) int {
		return strings.Compare(a.Directory+"/"+a.Path, b.Directory+"/"+b.Path)
	})
	if err := validateMaterializationPlan(plan); err != nil {
		return MaterializationPlan{}, err
	}
	return plan, nil
}

func prepareFileContent(item *PlannedFile, content FileContent) error {
	var encoded string
	switch c := content.(type) {
	case Text:
		encoded = string(c)
	case Lines:
		if len(c) > 0 {
			encoded = strings.Join(c, "\n") + "\n"
		}
	case KeyValues:
		values := make(map[string]string, len(c.Values))
		deferred := make(map[string]PlannedProperty, len(c.Values))
		bound := false
		for key, value := range c.Values {
			switch v := value.(type) {
			case Literal:
				literal := string(v)
				values[key] = literal
				deferred[key] = PlannedProperty{Literal: &literal}
			case PodNameBinding:
				deferred[key] = PlannedProperty{PodName: true}
				bound = true
			}
		}
		if bound {
			if _, ok := c.Codec.(PropertiesCodec); !ok {
				return fmt.Errorf("PodNameBinding requires built-in PropertiesCodec, got %T", c.Codec)
			}
			item.Properties = &PlannedProperties{Codec: propertiesCodecID, Values: deferred}
			return nil
		}
		var err error
		encoded, err = c.Codec.Encode(values)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported file content %T", content)
	}
	item.Encoded = &encoded
	return nil
}

func validatePlannedFile(file PlannedFile) error {
	if len(validation.IsDNS1123Label(file.Directory)) != 0 || !relativeFile(file.Path) ||
		strings.Contains(file.Path, "\\") {
		return fmt.Errorf("invalid materialization path %q/%q", file.Directory, file.Path)
	}
	if (file.Encoded == nil) == (file.Properties == nil) {
		return fmt.Errorf("file %s/%s requires exactly one content form", file.Directory, file.Path)
	}
	if file.Properties == nil {
		if !utf8.ValidString(*file.Encoded) {
			return fmt.Errorf("file %s/%s is not UTF-8 text", file.Directory, file.Path)
		}
		return nil
	}
	if file.Properties.Codec != propertiesCodecID {
		return fmt.Errorf("unsupported materialization codec %q", file.Properties.Codec)
	}
	for _, key := range sortedKeys(file.Properties.Values) {
		value := file.Properties.Values[key]
		if (value.Literal != nil) == value.PodName {
			return fmt.Errorf("property %q requires exactly one literal or PodName source", key)
		}
		if !utf8.ValidString(key) || (value.Literal != nil && !utf8.ValidString(*value.Literal)) {
			return fmt.Errorf("property %q is not UTF-8 text", key)
		}
	}
	return nil
}

func validateMaterializationPlan(plan MaterializationPlan) error {
	if plan.Version != materializationVersion {
		return fmt.Errorf("unsupported materialization version %q", plan.Version)
	}
	names := make(map[string]bool, len(plan.Files))
	for _, file := range plan.Files {
		if err := validatePlannedFile(file); err != nil {
			return err
		}
		name := file.Directory + "/" + file.Path
		if names[name] {
			return fmt.Errorf("duplicate materialization path %q", name)
		}
		names[name] = true
	}
	for _, name := range sortedKeys(names) {
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			if names[parent] {
				return fmt.Errorf("materialization paths %q and %q collide", parent, name)
			}
		}
	}
	return nil
}

func EncodeMaterializationPlan(plan MaterializationPlan) ([]byte, error) {
	if err := validateMaterializationPlan(plan); err != nil {
		return nil, err
	}
	return json.Marshal(plan)
}

func DecodeMaterializationPlan(data []byte) (MaterializationPlan, error) {
	var plan MaterializationPlan
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&plan); err != nil {
		return MaterializationPlan{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return MaterializationPlan{}, fmt.Errorf("materialization plan must contain exactly one JSON document")
	}
	if err := validateMaterializationPlan(plan); err != nil {
		return MaterializationPlan{}, err
	}
	return plan, nil
}

func resolvedMaterializedFiles(plan MaterializationPlan, podName string) (map[string]string, error) {
	if err := validateMaterializationPlan(plan); err != nil {
		return nil, err
	}
	files := make(map[string]string, len(plan.Files))
	for _, file := range plan.Files {
		name := file.Directory + "/" + file.Path
		if file.Encoded != nil {
			files[name] = *file.Encoded
			continue
		}
		values := make(map[string]string, len(file.Properties.Values))
		for key, value := range file.Properties.Values {
			if value.PodName {
				if len(validation.IsDNS1123Subdomain(podName)) != 0 {
					return nil, fmt.Errorf("file %q requires a valid POD_NAME", name)
				}
				values[key] = podName
			} else {
				values[key] = *value.Literal
			}
		}
		encoded, err := (PropertiesCodec{}).Encode(values)
		if err != nil {
			return nil, fmt.Errorf("file %q: %w", name, err)
		}
		files[name] = encoded
	}
	return files, nil
}

// Materialize resolves every value before writing root/<Directory>/<Path>.
// The caller gives it exclusive ownership of the output tree (an init-container
// EmptyDir in the assembly). Files are individually replaced atomically;
// a partial write failure fails the init process, not a running product reload.
// It neither deletes files from older plans nor changes UID/GID or mode globally.
func Materialize(outputRoot string, plan MaterializationPlan, podName string) error {
	files, err := resolvedMaterializedFiles(plan, podName)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outputRoot, 0755); err != nil {
		return err
	}
	root, err := os.OpenRoot(outputRoot)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	for _, name := range sortedKeys(files) {
		if err := writeMaterializedFile(root, name, files[name]); err != nil {
			return fmt.Errorf("materialize %q: %w", name, err)
		}
	}
	return nil
}

func writeMaterializedFile(root *os.Root, name, content string) error {
	if err := root.MkdirAll(path.Dir(name), 0755); err != nil {
		return err
	}
	// A fresh temporary inode plus Rename avoids following an existing file's
	// symlink or overwriting a hard-link target. os.Root confines path traversal.
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	temp := path.Join(path.Dir(name), ".materialize-"+hex.EncodeToString(nonce))
	file, err := root.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return err
	}
	defer func() { _ = root.Remove(temp) }()
	_, writeErr := file.WriteString(content)
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return err
	}
	return root.Rename(temp, name)
}
