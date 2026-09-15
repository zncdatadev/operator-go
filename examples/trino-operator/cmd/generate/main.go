// Command generate writes or checks the generated input, schema and registration.
package main

import (
	"flag"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/zncdatadev/operator-go/examples/trino-operator/internal/product"
	"github.com/zncdatadev/operator-go/pkg/framework/inputgen"
)

func main() {
	check := flag.Bool("check", false, "check generated files without writing")
	flag.Parse()
	if err := run(*check); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(check bool) error {
	artifacts, err := inputgen.Generate[product.TrinoConfig, product.TrinoClusterConfig](inputgen.Names{
		Package: "v1alpha1", Group: "trino.kubedoop.dev", Version: "v1alpha1", Kind: "TrinoCluster", Plural: "trinoclusters",
		ImportPath: "github.com/zncdatadev/operator-go/examples/trino-operator/api/v1alpha1",
	}, slices.Sorted(maps.Keys(product.Definition().Roles)))
	if err != nil {
		return err
	}
	paths := []string{"api/v1alpha1/zz_generated.input.go", "config/crd/bases/trino.kubedoop.dev_trinoclusters.yaml",
		"api/v1alpha1/registration/zz_generated.register.go"}
	contents := [][]byte{artifacts.GoSource, artifacts.CRD, artifacts.RegistrationSource}
	if check {
		actual := make([][]byte, len(paths))
		for index, name := range paths {
			actual[index], err = os.ReadFile(name)
			if err != nil {
				return err
			}
		}
		return inputgen.Check(artifacts, actual[0], actual[1], actual[2])
	}
	for index, name := range paths {
		if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(name, contents[index], 0644); err != nil {
			return err
		}
	}
	return nil
}
