package main

import (
	"bytes"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"example.com/framework-consumer/product"
	"github.com/zncdatadev/operator-go/pkg/framework/input"
	"github.com/zncdatadev/operator-go/pkg/framework/inputgen"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	trinoNames := inputgen.Names{Package: "trino", Group: "consumer.example.com", Version: "v1alpha1",
		Kind: "TrinoCluster", Plural: "trinoclusters", ImportPath: "example.com/framework-consumer/generated/trino"}
	roles := slices.Sorted(maps.Keys(product.Definition().Roles))
	trino, err := inputgen.Generate[product.TrinoConfig, product.TrinoClusterConfig](trinoNames, roles)
	if err != nil {
		return err
	}
	slices.Reverse(roles)
	reordered, err := inputgen.Generate[product.TrinoConfig, product.TrinoClusterConfig](trinoNames, roles)
	if err != nil || !bytes.Equal(trino.GoSource, reordered.GoSource) || !bytes.Equal(trino.CRD, reordered.CRD) || !bytes.Equal(trino.RegistrationSource, reordered.RegistrationSource) {
		return fmt.Errorf("generation is not deterministic: %v", err)
	}
	presence, err := inputgen.Generate[product.PresenceConfig, product.PresenceClusterConfig](
		inputgen.Names{Package: "presence", Group: "consumer.example.com", Version: "v1alpha1",
			Kind: "PresenceCluster", Plural: "presenceclusters", ImportPath: "example.com/framework-consumer/generated/presence"}, []string{"workers"})
	if err != nil {
		return err
	}
	for name, artifacts := range map[string]inputgen.Artifacts{"trino": trino, "presence": presence} {
		if bytes.Contains(artifacts.GoSource, []byte("docs/discussions")) ||
			bytes.Contains(artifacts.RegistrationSource, []byte("docs/discussions")) ||
			bytes.Contains(artifacts.RegistrationSource, []byte("internal/framework")) {
			return fmt.Errorf("generated %s imports the prototype", name)
		}
		directory := filepath.Join("generated", name)
		if err := os.MkdirAll(filepath.Join(directory, "registration"), 0o700); err != nil {
			return err
		}
		goPath, crdPath := filepath.Join(directory, "zz_generated.input.go"), filepath.Join(directory, "crd.yaml")
		if err := os.WriteFile(goPath, artifacts.GoSource, 0o600); err != nil {
			return err
		}
		if err := os.WriteFile(crdPath, artifacts.CRD, 0o600); err != nil {
			return err
		}
		registrationPath := filepath.Join(directory, "registration", "zz_generated.register.go")
		if err := os.WriteFile(registrationPath, artifacts.RegistrationSource, 0o600); err != nil {
			return err
		}
		actualRegistration, err := os.ReadFile(registrationPath)
		if err != nil {
			return err
		}
		actualGo, err := os.ReadFile(goPath)
		if err != nil {
			return err
		}
		actualCRD, err := os.ReadFile(crdPath)
		if err != nil {
			return err
		}
		if err := inputgen.Check(artifacts, actualGo, actualCRD, actualRegistration); err != nil {
			return err
		}
		if err := inputgen.Check(artifacts, append(bytes.Clone(actualGo), '\n'), actualCRD, actualRegistration); err == nil {
			return fmt.Errorf("generated-source drift was accepted")
		}
		if err := inputgen.Check(artifacts, actualGo, append(bytes.Clone(actualCRD), '\n'), actualRegistration); err == nil {
			return fmt.Errorf("generated-schema drift was accepted")
		}
		version := fmt.Sprintf("const InputContractVersion = %d", input.ContractVersion)
		unsupported := fmt.Sprintf("const InputContractVersion = %d", input.ContractVersion+1)
		futureGo := bytes.Replace(actualGo, []byte(version), []byte(unsupported), 1)
		future := inputgen.Artifacts{GoSource: futureGo, CRD: actualCRD}
		if bytes.Equal(futureGo, actualGo) || inputgen.Check(future, futureGo, actualCRD) == nil {
			return fmt.Errorf("mutually matching unsupported generated contract was accepted")
		}
	}
	return nil
}
