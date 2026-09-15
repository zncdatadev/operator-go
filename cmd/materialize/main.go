// Command materialize writes a validated file plan into an exclusive output tree.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/zncdatadev/operator-go/internal/framework/pipeline"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "materialize:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("materialize", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	planPath := flags.String("plan", "", "JSON materialization plan path")
	outputRoot := flags.String("root", "", "output tree root")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *planPath == "" || *outputRoot == "" || flags.NArg() != 0 {
		return fmt.Errorf("--plan and --root are required; positional arguments are unsupported")
	}
	data, err := os.ReadFile(*planPath)
	if err != nil {
		return err
	}
	plan, err := pipeline.DecodeMaterializationPlan(data)
	if err != nil {
		return err
	}
	return pipeline.Materialize(*outputRoot, plan, os.Getenv("POD_NAME"))
}
