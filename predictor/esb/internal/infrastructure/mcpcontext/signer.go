package mcpcontext

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/predictboss/predictor/esb/internal/domain"
)

const maxTokenBytes = 2048

var errInvalidContext = errors.New("invalid MCP call context")

type signer struct {
	keys map[domain.AgentType][]byte
	now  func() time.Time
}

type claims struct {
	Version   int              `json:"v"`
	Agent     domain.AgentType `json:"agent"`
	CallID    string           `json:"call_id"`
	CompanyID string           `json:"company_id"`
	IssuedAt  int64            `json:"iat"`
	ExpiresAt int64            `json:"exp"`
}

// NewSigner derives separate context-signing keys from the configured agent
// MCP tokens. The raw MCP tokens are never placed in Hermes input.
func NewSigner(tokens map[domain.AgentType]string) (domain.MCPCallContextSigner, error) {
	keys := make(map[domain.AgentType][]byte, len(tokens))
	for agent, token := range tokens {
		if strings.TrimSpace(token) == "" {
			return nil, errors.New("MCP call context requires configured agent tokens")
		}
		key := sha256.Sum256([]byte("predictor/esb/mcp-call-context/v1\x00" + token))
		keys[agent] = key[:]
	}
	return &signer{keys: keys, now: time.Now}, nil
}

func (s *signer) IssueMCPCallContext(value domain.MCPCallContext) (string, error) {
	key, ok := s.keys[value.Agent]
	if !ok || value.CallID == "" || value.CompanyID == "" {
		return "", errInvalidContext
	}
	now := s.now().UTC()
	if value.ExpiresAt.IsZero() || !value.ExpiresAt.After(now) || value.ExpiresAt.Sub(now) > 45*time.Minute {
		return "", errInvalidContext
	}
	encoded, err := json.Marshal(claims{
		Version: 1, Agent: value.Agent, CallID: value.CallID, CompanyID: value.CompanyID,
		IssuedAt: now.Unix(), ExpiresAt: value.ExpiresAt.UTC().Unix(),
	})
	if err != nil {
		return "", err
	}
	body := base64.RawURLEncoding.EncodeToString(encoded)
	mac := hmac.New(sha256.New, key)
	if _, err := mac.Write([]byte(body)); err != nil {
		return "", err
	}
	return body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (s *signer) VerifyMCPCallContext(token string, expectedAgent domain.AgentType) (domain.MCPCallContext, error) {
	if len(token) == 0 || len(token) > maxTokenBytes {
		return domain.MCPCallContext{}, errInvalidContext
	}
	body, signature, ok := strings.Cut(token, ".")
	if !ok || strings.Contains(signature, ".") {
		return domain.MCPCallContext{}, errInvalidContext
	}
	key, ok := s.keys[expectedAgent]
	if !ok {
		return domain.MCPCallContext{}, errInvalidContext
	}
	providedSignature, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return domain.MCPCallContext{}, errInvalidContext
	}
	mac := hmac.New(sha256.New, key)
	if _, err := mac.Write([]byte(body)); err != nil {
		return domain.MCPCallContext{}, err
	}
	if !hmac.Equal(providedSignature, mac.Sum(nil)) {
		return domain.MCPCallContext{}, errInvalidContext
	}
	encoded, err := base64.RawURLEncoding.DecodeString(body)
	if err != nil {
		return domain.MCPCallContext{}, errInvalidContext
	}
	var value claims
	if err := json.Unmarshal(encoded, &value); err != nil || value.Version != 1 || value.Agent != expectedAgent ||
		value.CallID == "" || value.CompanyID == "" || value.IssuedAt <= 0 || value.ExpiresAt <= value.IssuedAt {
		return domain.MCPCallContext{}, errInvalidContext
	}
	now := s.now().UTC().Unix()
	if value.IssuedAt > now+30 || value.ExpiresAt <= now || value.ExpiresAt-value.IssuedAt > int64(45*time.Minute/time.Second) {
		return domain.MCPCallContext{}, errInvalidContext
	}
	return domain.MCPCallContext{Agent: value.Agent, CallID: value.CallID, CompanyID: value.CompanyID,
		ExpiresAt: time.Unix(value.ExpiresAt, 0).UTC()}, nil
}

var _ domain.MCPCallContextSigner = (*signer)(nil)
