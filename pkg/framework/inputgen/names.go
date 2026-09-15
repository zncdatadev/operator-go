package inputgen

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"path"
	"strconv"
)

// validateGeneratedNames checks the declarations and imports of this generator's
// fixed template. Formatting validates syntax but does not detect a requested CR
// kind that shadows ConfigInput, a nested input type, or an imported package.
func validateGeneratedNames(source []byte) error {
	file, err := parser.ParseFile(token.NewFileSet(), "generated.go", source, parser.SkipObjectResolution)
	if err != nil {
		return fmt.Errorf("parse generated input: %w", err)
	}
	names := make(map[string]string)
	for _, spec := range file.Imports {
		importPath, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return err
		}
		// All unaliased imports in the fixed template use their final path
		// segment as package name. Package declarations themselves bind no name.
		name := path.Base(importPath)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		if err := reserveGeneratedName(names, name, "import "+importPath); err != nil {
			return err
		}
	}
	for _, declaration := range file.Decls {
		switch item := declaration.(type) {
		case *ast.FuncDecl:
			// Methods have receiver scope. Go also permits multiple init functions.
			if item.Recv != nil || item.Name.Name == "init" {
				continue
			}
			if err := reserveGeneratedName(names, item.Name.Name, "function "+item.Name.Name); err != nil {
				return err
			}
		case *ast.GenDecl:
			if err := reserveGeneratedSpecs(names, item.Specs); err != nil {
				return err
			}
		}
	}
	return nil
}

func reserveGeneratedSpecs(names map[string]string, specs []ast.Spec) error {
	for _, spec := range specs {
		switch item := spec.(type) {
		case *ast.TypeSpec:
			if err := reserveGeneratedName(names, item.Name.Name, "type "+item.Name.Name); err != nil {
				return err
			}
		case *ast.ValueSpec:
			for _, name := range item.Names {
				if err := reserveGeneratedName(names, name.Name, "value "+name.Name); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func reserveGeneratedName(names map[string]string, name, source string) error {
	if name == "_" {
		return nil
	}
	if previous, exists := names[name]; exists {
		return fmt.Errorf("generated name %q conflicts between %s and %s", name, previous, source)
	}
	names[name] = source
	return nil
}
