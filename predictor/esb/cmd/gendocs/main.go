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
	examplePath := filepath.Join(filepath.Dir(filepath.Dir(*outputDir)), "example.env")
	artifacts := []struct {
		path     string
		contents []byte
	}{
		{configurationPath, configurationDoc},
		{examplePath, config.RenderExampleEnv()},
	}
	for _, artifact := range artifacts {
		if *check {
			current, err := os.ReadFile(artifact.path)
			if err != nil || !bytes.Equal(current, artifact.contents) {
				fmt.Fprintf(os.Stderr, "generated configuration artifact %s is stale; run make docs\n", artifact.path)
				os.Exit(1)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(artifact.path), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		if err := os.WriteFile(artifact.path, artifact.contents, 0o644); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
