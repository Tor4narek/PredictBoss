package mcp

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/predictboss/predictor/esb/internal/application/analytics"
	"github.com/predictboss/predictor/esb/internal/application/companycontext"
	"github.com/predictboss/predictor/esb/internal/domain"
)

const maxMCPTokenBytes = 256

var agentTools = map[domain.AgentType][]string{
	domain.AgentSerezha: {
		"save_news_and_facts",
	},
	domain.AgentSaveliy: {
		"get_company_profile", "get_facts", "save_fact_company_links",
	},
	domain.AgentValera: {
		"get_company_metrics", "save_company_profile_and_metrics",
	},
	domain.AgentKesha: {
		"get_company_profile", "get_company_facts", "get_company_metrics",
		"save_metric_observations", "calculate_all_metric_totals", "save_company_analysis", "get_company_report",
	},
	domain.AgentKonstantin: {
		"get_company_profile", "get_company_metrics", "get_consultant_context",
	},
}

// NewStorageHandler serves role-filtered tools on the existing MCP endpoint.
// Agent identity is derived only from its bearer token; the integration API key
// is intentionally not accepted here.
func NewStorageHandler(service *analytics.Service, companyContext *companycontext.Service,
	tokens map[domain.AgentType]string, declaredTools map[domain.AgentType][]string) (http.Handler, error) {
	principals, err := authenticateAgentTokens(tokens)
	if err != nil {
		return nil, err
	}
	servers := make(map[domain.AgentType]*mcp.Server, len(agentTools))
	for agent, expectedTools := range agentTools {
		declared, ok := declaredTools[agent]
		if !ok || !sameTools(expectedTools, declared) {
			return nil, fmt.Errorf("agent %s MCP tool declaration does not match the ESB policy", agent)
		}
		servers[agent] = newStorageServer(service, companyContext, toolSet(expectedTools))
	}
	streamable := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		principal := auth.TokenInfoFromContext(r.Context())
		if principal == nil {
			return nil
		}
		return servers[domain.AgentType(principal.UserID)]
	}, &mcp.StreamableHTTPOptions{JSONResponse: true})
	verify := func(_ context.Context, token string, _ *http.Request) (*auth.TokenInfo, error) {
		agent, ok := authenticateRequest("Bearer "+token, principals)
		if !ok {
			return nil, auth.ErrInvalidToken
		}
		return &auth.TokenInfo{UserID: string(agent), Expiration: time.Now().Add(time.Hour)}, nil
	}
	return auth.RequireBearerToken(verify, nil)(streamable), nil
}

// NewStorageServer is used by the API documentation generator to discover the
// complete tool schemas without initializing storage connections.
func NewStorageServer(service *analytics.Service, companyContext *companycontext.Service) *mcp.Server {
	return newStorageServer(service, companyContext, nil)
}

func newStorageServer(service *analytics.Service, companyContext *companycontext.Service, allowed map[string]struct{}) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "predictor-esb", Version: "0.3.0"}, nil)
	registerCompanyProfileTools(server, allowed, companyContext)
	registerFactTools(server, allowed, service)
	registerMetricTools(server, allowed, service)
	registerReportTools(server, allowed, service)
	return server
}

func authenticateAgentTokens(tokens map[domain.AgentType]string) (map[domain.AgentType][32]byte, error) {
	principals := make(map[domain.AgentType][32]byte, len(agentTools))
	seen := make(map[[32]byte]struct{}, len(agentTools))
	if len(tokens) != len(agentTools) {
		return nil, errors.New("MCP requires one token per configured agent")
	}
	for agent := range agentTools {
		token := tokens[agent]
		if len(token) < 32 || len(token) > maxMCPTokenBytes {
			return nil, fmt.Errorf("MCP token for agent %s must contain 32 to %d bytes", agent, maxMCPTokenBytes)
		}
		digest := sha256.Sum256([]byte(token))
		if _, exists := seen[digest]; exists {
			return nil, errors.New("MCP agent tokens must be unique")
		}
		seen[digest] = struct{}{}
		principals[agent] = digest
	}
	return principals, nil
}

func authenticateRequest(header string, principals map[domain.AgentType][32]byte) (domain.AgentType, bool) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || len(parts[1]) < 32 || len(parts[1]) > maxMCPTokenBytes {
		return "", false
	}
	candidate := sha256.Sum256([]byte(parts[1]))
	var principal domain.AgentType
	matched := 0
	for agent, digest := range principals {
		equal := subtle.ConstantTimeCompare(candidate[:], digest[:])
		if equal == 1 {
			principal = agent
		}
		matched |= equal
	}
	return principal, matched == 1
}

func toolSet(tools []string) map[string]struct{} {
	result := make(map[string]struct{}, len(tools))
	for _, name := range tools {
		result[name] = struct{}{}
	}
	return result
}

func sameTools(expected, declared []string) bool {
	if len(expected) != len(declared) {
		return false
	}
	set := toolSet(expected)
	for _, name := range declared {
		if _, ok := set[name]; !ok {
			return false
		}
		delete(set, name)
	}
	return len(set) == 0
}
