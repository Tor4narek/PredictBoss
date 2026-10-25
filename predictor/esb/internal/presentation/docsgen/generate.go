package docsgen

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/predictboss/predictor/esb/internal/presentation/httpapi"
	presentationmcp "github.com/predictboss/predictor/esb/internal/presentation/mcp"
)

// Generate writes all machine-readable contracts from the registered Go
// contracts and the live MCP tool registry. In check mode it reports stale
// artifacts without modifying the working tree.
func Generate(ctx context.Context, outDir string, check bool) error {
	openapi, err := httpapi.GenerateOpenAPISpec()
	if err != nil {
		return err
	}
	asyncapi, err := httpapi.GenerateAsyncAPISpec()
	if err != nil {
		return err
	}
	mcpTools, err := generateMCPTools(ctx)
	if err != nil {
		return err
	}
	mcpJSON, err := json.MarshalIndent(mcpTools, "", "  ")
	if err != nil {
		return fmt.Errorf("encode MCP tool catalog: %w", err)
	}

	artifacts := map[string][]byte{
		"openapi.json":   openapi,
		"asyncapi.json":  asyncapi,
		"mcp-tools.json": mcpJSON,
		"mcp-tools.md":   renderMCPToolsMarkdown(mcpTools),
	}
	if !check {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return fmt.Errorf("create API docs directory: %w", err)
		}
	}
	for name, content := range artifacts {
		if !bytes.HasSuffix(content, []byte("\n")) {
			content = append(content, '\n')
		}
		path := filepath.Join(outDir, name)
		if check {
			current, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("read generated artifact %s: %w", path, err)
			}
			if !bytes.Equal(current, content) {
				return fmt.Errorf("generated artifact %s is stale; run make docs", path)
			}
			continue
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return fmt.Errorf("write generated artifact %s: %w", path, err)
		}
	}
	return nil
}

func generateMCPTools(ctx context.Context) (*mcp.ListToolsResult, error) {
	server := presentationmcp.NewStorageServer(nil, nil)
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		return nil, fmt.Errorf("start in-memory MCP server: %w", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "predictor-doc-generator", Version: "1"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		closeErr := serverSession.Close()
		if closeErr != nil && !errors.Is(closeErr, context.Canceled) {
			return nil, errors.Join(fmt.Errorf("connect to MCP tool registry: %w", err), fmt.Errorf("close in-memory MCP server: %w", closeErr))
		}
		return nil, fmt.Errorf("connect to MCP tool registry: %w", err)
	}
	tools, listErr := clientSession.ListTools(ctx, nil)
	clientErr := clientSession.Close()
	serverErr := serverSession.Wait()
	if listErr != nil {
		return nil, fmt.Errorf("list MCP tools: %w", listErr)
	}
	if clientErr != nil && !errors.Is(clientErr, context.Canceled) {
		return nil, fmt.Errorf("close MCP documentation client: %w", clientErr)
	}
	if serverErr != nil && !errors.Is(serverErr, context.Canceled) {
		return nil, fmt.Errorf("close MCP documentation server: %w", serverErr)
	}
	sort.Slice(tools.Tools, func(i, j int) bool { return tools.Tools[i].Name < tools.Tools[j].Name })
	return tools, nil
}

func renderMCPToolsMarkdown(result *mcp.ListToolsResult) []byte {
	var document strings.Builder
	document.WriteString("<!-- Generated from the MCP tools/list registry. Do not edit by hand. -->\n")
	document.WriteString("# Инструменты MCP ESB\n\n")
	document.WriteString("Источник схем запросов и ответов: [`mcp-tools.json`](mcp-tools.json). Список генерируется из тех же зарегистрированных инструментов, которые ESB отдаёт агентам через `tools/list`.\n\n")
	document.WriteString("| Инструмент | Назначение |\n| --- | --- |\n")
	for _, tool := range result.Tools {
		description := strings.ReplaceAll(strings.TrimSpace(tool.Description), "|", "\\|")
		description = strings.ReplaceAll(description, "\n", " ")
		document.WriteString(fmt.Sprintf("| `%s` | %s |\n", tool.Name, description))
	}
	return []byte(document.String())
}

// GenerateFromRoot is used by the repository command when it creates a short
// context for MCP registry discovery.
func GenerateFromRoot(outDir string, check bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return Generate(ctx, outDir, check)
}
