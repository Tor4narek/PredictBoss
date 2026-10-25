package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/predictboss/predictor/esb/internal/config"
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
	configurationPath := filepath.Join(filepath.Dir(*outputDir), "configuration.md")
	configurationDoc := config.RenderDocumentation()
	if *check {
		current, err := os.ReadFile(configurationPath)
		if err != nil || !bytes.Equal(current, configurationDoc) {
			fmt.Fprintf(os.Stderr, "generated configuration reference %s is stale; run make docs\n", configurationPath)
			os.Exit(1)
		}
		return
	}
	if err := os.MkdirAll(filepath.Dir(configurationPath), 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := os.WriteFile(configurationPath, configurationDoc, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
