package pipeline

import (
	"fmt"
	"maps"
	"path"
	"slices"
	"strings"
)

// ValidateFileOverride checks action syntax, not a file's encoding capability.
func ValidateFileOverride(override FileOverride) error {
	modes := 0
	for _, present := range []bool{
		override.Properties != nil, override.Lines != nil, override.Text != nil, override.Remove != nil,
	} {
		if present {
			modes++
		}
	}
	if modes != 1 {
		return fmt.Errorf("exactly one of properties, lines, text or remove is required")
	}
	if override.Remove != nil && !*override.Remove {
		return fmt.Errorf("remove must be true")
	}
	if override.Lines != nil {
		if *override.Lines == nil {
			return fmt.Errorf("lines cannot be null")
		}
		if err := validateContent(Lines(*override.Lines)); err != nil {
			return err
		}
	}
	if override.Properties != nil {
		return validatePropertyOverride(*override.Properties)
	}
	return nil
}

func validatePropertyOverride(override PropertyOverride) error {
	if override.Set != nil && *override.Set == nil {
		return fmt.Errorf("properties.set cannot be null")
	}
	if override.Remove != nil && *override.Remove == nil {
		return fmt.Errorf("properties.remove cannot be null")
	}
	if override.Replace != nil && *override.Replace == nil {
		return fmt.Errorf("properties.replace cannot be null")
	}
	if override.Replace != nil && (override.Set != nil || override.Remove != nil) {
		return fmt.Errorf("properties.replace cannot be combined with set or remove")
	}
	if override.Set != nil && override.Remove != nil {
		for _, key := range *override.Remove {
			if _, exists := (*override.Set)[key]; exists {
				return fmt.Errorf("properties key %q is both set and removed", key)
			}
		}
	}
	return nil
}

type overrideFileKey struct{ directory, path string }

// ApplyFileOverrides executes each layer as actions against current contents.
// Only configDirectory is addressable. Other directories are independently
// copied, and the result is sorted by directory/path. Codecs are retained as
// stateless capabilities; deleting a file never resurrects its original values.
func ApplyFileOverrides(
	original []File, configDirectory string, layers ...map[string]FileOverride,
) ([]File, error) {
	if configDirectory == "" {
		return nil, fmt.Errorf("config directory is required")
	}
	current, codecs, err := initialOverrideFiles(original, configDirectory)
	if err != nil {
		return nil, err
	}
	for index, layer := range layers {
		for _, name := range slices.Sorted(maps.Keys(layer)) {
			override := layer[name]
			if !relativeFile(name) {
				return nil, fmt.Errorf("layer %d file %q: invalid relative path", index+1, name)
			}
			if err := ValidateFileOverride(override); err != nil {
				return nil, fmt.Errorf("layer %d file %q: %w", index+1, name, err)
			}
			key := overrideFileKey{configDirectory, name}
			if err := applyFileOverride(current, key, codecs[name], override); err != nil {
				return nil, fmt.Errorf("layer %d file %q: %w", index+1, name, err)
			}
		}
		// A layer is a map, so deleting a parent and creating a child in that
		// same layer must not depend on the lexical order of the file names.
		if err := validateOverrideFilePaths(current); err != nil {
			return nil, fmt.Errorf("layer %d: %w", index+1, err)
		}
	}
	return sortedOverrideFiles(current), nil
}

func initialOverrideFiles(
	original []File, configDirectory string,
) (map[overrideFileKey]File, map[string]PropertyCodec, error) {
	current := make(map[overrideFileKey]File, len(original))
	codecs := map[string]PropertyCodec{}
	for _, file := range original {
		key := overrideFileKey{file.Directory, file.Path}
		if file.Directory == "" || !relativeFile(file.Path) {
			return nil, nil, fmt.Errorf("original file %q/%q: invalid directory or relative path", file.Directory, file.Path)
		}
		if _, exists := current[key]; exists {
			return nil, nil, fmt.Errorf("original file %q/%q: duplicate path", file.Directory, file.Path)
		}
		if err := validateContent(file.Content); err != nil {
			return nil, nil, fmt.Errorf("original file %q/%q: %w", file.Directory, file.Path, err)
		}
		current[key] = cloneFileForOverride(file)
		if kv, ok := file.Content.(KeyValues); ok && file.Directory == configDirectory {
			codecs[file.Path] = kv.Codec
		}
	}
	if err := validateOverrideFilePaths(current); err != nil {
		return nil, nil, fmt.Errorf("original files: %w", err)
	}
	return current, codecs, nil
}

func cloneFileForOverride(file File) File {
	switch content := file.Content.(type) {
	case KeyValues:
		content.Values = maps.Clone(content.Values)
		file.Content = content
	case Lines:
		file.Content = slices.Clone(content)
	}
	return file
}

func applyFileOverride(
	current map[overrideFileKey]File, key overrideFileKey, codec PropertyCodec, override FileOverride,
) error {
	file := File{Directory: key.directory, Path: key.path}
	switch {
	case override.Remove != nil:
		delete(current, key)
	case override.Text != nil:
		file.Content = Text(*override.Text)
		current[key] = file
	case override.Lines != nil:
		file.Content = Lines(slices.Clone(*override.Lines))
		current[key] = file
	case override.Properties != nil:
		return applyPropertyOverride(current, key, codec, *override.Properties)
	}
	return nil
}

func applyPropertyOverride(
	current map[overrideFileKey]File, key overrideFileKey, codec PropertyCodec, override PropertyOverride,
) error {
	if nilCodec(codec) {
		return fmt.Errorf("properties require an original structured encoding declaration")
	}
	content := KeyValues{Codec: codec, Values: map[string]PropertyValue{}}
	if override.Replace != nil {
		for name, value := range *override.Replace {
			content.Values[name] = Literal(value)
		}
		current[key] = File{Directory: key.directory, Path: key.path, Content: content}
		return nil
	}
	if file, exists := current[key]; exists {
		var structured bool
		content, structured = file.Content.(KeyValues)
		if !structured {
			return fmt.Errorf("properties patch cannot edit text or lines; use properties.replace")
		}
	} else if override.Set == nil || len(*override.Set) == 0 {
		// An empty patch or removal of keys cannot recreate a deleted file.
		return nil
	}
	if content.Values == nil {
		content.Values = map[string]PropertyValue{}
	}
	if override.Set != nil {
		for name, value := range *override.Set {
			content.Values[name] = Literal(value)
		}
	}
	if override.Remove != nil {
		for _, name := range *override.Remove {
			delete(content.Values, name)
		}
	}
	current[key] = File{Directory: key.directory, Path: key.path, Content: content}
	return nil
}

func validateOverrideFilePaths(current map[overrideFileKey]File) error {
	for _, file := range sortedOverrideFiles(current) {
		for parent := path.Dir(file.Path); parent != "."; parent = path.Dir(parent) {
			if _, exists := current[overrideFileKey{file.Directory, parent}]; exists {
				return fmt.Errorf("file %q/%q conflicts with parent file %q", file.Directory, file.Path, parent)
			}
		}
	}
	return nil
}

func sortedOverrideFiles(current map[overrideFileKey]File) []File {
	files := slices.Collect(maps.Values(current))
	slices.SortFunc(files, func(a, b File) int {
		if directory := strings.Compare(a.Directory, b.Directory); directory != 0 {
			return directory
		}
		return strings.Compare(a.Path, b.Path)
	})
	return files
}
