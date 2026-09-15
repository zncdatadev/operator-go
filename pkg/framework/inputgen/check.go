package inputgen

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"

	"github.com/zncdatadev/operator-go/pkg/framework/input"
)

const contractVersionName = "InputContractVersion"

// Check rejects unsupported generated contracts before comparing the current
// generated source and CRD. The caller owns reading and writing artifact files.
func Check(expected Artifacts, actualGo, actualCRD []byte, actualRegistration ...[]byte) error {
	if err := checkRegistration(expected.RegistrationSource, actualRegistration); err != nil {
		return err
	}
	for _, source := range [][]byte{expected.GoSource, actualGo} {
		version, err := generatedVersion(source)
		if err != nil {
			return err
		}
		if err := input.CheckVersion(version); err != nil {
			return err
		}
	}
	if !bytes.Equal(expected.GoSource, actualGo) {
		return fmt.Errorf("generated Go source differs; regenerate with the current inputgen")
	}
	if !bytes.Equal(expected.CRD, actualCRD) {
		return fmt.Errorf("generated CRD differs; regenerate with the current inputgen")
	}
	return nil
}

func generatedVersion(source []byte) (int, error) {
	file, err := parser.ParseFile(token.NewFileSet(), "generated.go", source, parser.SkipObjectResolution)
	if err != nil {
		return 0, fmt.Errorf("generated input contract: %w", err)
	}
	for _, declaration := range file.Decls {
		decl, ok := declaration.(*ast.GenDecl)
		if !ok || decl.Tok != token.CONST {
			continue
		}
		for _, spec := range decl.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok || len(value.Names) != 1 || value.Names[0].Name != contractVersionName || len(value.Values) != 1 {
				continue
			}
			literal, ok := value.Values[0].(*ast.BasicLit)
			if ok && literal.Kind == token.INT {
				version, err := strconv.Atoi(literal.Value)
				if err == nil {
					return version, nil
				}
			}
			return 0, fmt.Errorf("generated InputContractVersion must be an integer literal")
		}
	}
	return 0, fmt.Errorf("generated input is missing InputContractVersion; regenerate legacy artifacts")
}

// Optional only for API-only generation. A requested companion must be supplied,
// version-compatible and byte-identical; callers cannot accidentally omit it.
func checkRegistration(expected []byte, actual [][]byte) error {
	if len(actual) > 1 {
		return fmt.Errorf("at most one generated registration source is allowed")
	}
	if len(expected) == 0 {
		if len(actual) == 1 && len(actual[0]) != 0 {
			return fmt.Errorf("unexpected registration source for API-only generation")
		}
		return nil
	}
	if len(actual) != 1 || len(actual[0]) == 0 {
		return fmt.Errorf("generated registration source is required; regenerate the companion")
	}
	for _, source := range [][]byte{expected, actual[0]} {
		version, err := generatedVersion(source)
		if err != nil {
			return fmt.Errorf("registration: %w", err)
		}
		if err := input.CheckVersion(version); err != nil {
			return fmt.Errorf("registration: %w", err)
		}
	}
	if !bytes.Equal(expected, actual[0]) {
		return fmt.Errorf("generated registration source differs; regenerate with the current inputgen")
	}
	return nil
}
