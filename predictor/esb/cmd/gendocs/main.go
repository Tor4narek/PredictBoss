package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/predictboss/predictor/esb/internal/presentation/docsgen"
)

func main() {
	outputDir := flag.String("out", "../docs/api", "directory for generated API documents")
	check := flag.Bool("check", false, "fail if generated API documents are stale")
	flag.Parse()

	if err := docsgen.GenerateFromRoot(*outputDir, *check); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
