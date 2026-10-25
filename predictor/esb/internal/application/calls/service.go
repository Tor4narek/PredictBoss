package calls

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/predictboss/predictor/esb/internal/domain"
)

var ErrInvalidCall = errors.New("invalid call")
var scopePattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

type Service struct {
	repository domain.CallRepository
	runtime    domain.AgentRuntime
	agents     map[domain.AgentType]domain.Agent
	pollEvery  time.Duration
}

func NewService(repository domain.CallRepository, runtime domain.AgentRuntime, agents []domain.Agent, pollEvery time.Duration) *Service {
	byType := make(map[domain.AgentType]domain.Agent, len(agents))
	for _, agent := range agents {
		byType[agent.Type] = agent
	}
	if pollEvery <= 0 {
		pollEvery = 2 * time.Second
	}
	return &Service{repository: repository, runtime: runtime, agents: byType, pollEvery: pollEvery}
}

func (s *Service) Create(ctx context.Context, agentType string, input json.RawMessage, idempotencyKey string) (domain.Call, bool, error) {
	typeKey := domain.AgentType(strings.ToLower(strings.TrimSpace(agentType)))
	agent, ok := s.agents[typeKey]
	if !ok || len(input) == 0 || !json.Valid(input) {
		return domain.Call{}, false, ErrInvalidCall
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(input, &fields); err != nil || fields == nil {
		return domain.Call{}, false, ErrInvalidCall
	}
	var scopeID string
	if json.Unmarshal(fields[agent.ScopeField], &scopeID) != nil {
		return domain.Call{}, false, fmt.Errorf("%w: %s is required", ErrInvalidCall, agent.ScopeField)
	}
	scopeID = strings.TrimSpace(scopeID)
	if !scopePattern.MatchString(scopeID) {
		return domain.Call{}, false, fmt.Errorf("%w: invalid %s", ErrInvalidCall, agent.ScopeField)
	}
	for _, field := range agent.RequiredFields {
		value, ok := fields[field]
		if !ok || len(value) == 0 || string(value) == "null" || string(value) == `""` || string(value) == "{}" {
			return domain.Call{}, false, fmt.Errorf("%w: input field %s is required", ErrInvalidCall, field)
		}
	}
	call, reused, err := s.repository.Enqueue(ctx, typeKey, scopeID, input, idempotencyKey)
	return call, reused, err
}

func (s *Service) Get(ctx context.Context, id string) (domain.Call, error) {
	return s.repository.Get(ctx, strings.TrimSpace(id))
}

func (s *Service) GetForAgent(ctx context.Context, id, agent string) (domain.Call, error) {
	typeKey := domain.AgentType(strings.ToLower(strings.TrimSpace(agent)))
	if _, ok := s.agents[typeKey]; !ok {
		return domain.Call{}, ErrInvalidCall
	}
	return s.repository.GetForAgent(ctx, strings.TrimSpace(id), typeKey)
}

// Run owns the asynchronous queue dispatch and Hermes polling loop.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(s.pollEvery)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return
		}
		s.dispatchQueued(ctx)
		s.pollActive(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) dispatchQueued(ctx context.Context) {
	calls, err := s.repository.ClaimQueued(ctx, 100)
	if err != nil {
		return
	}
	for _, call := range calls {
		call := call
		go s.submit(ctx, call)
	}
}

func (s *Service) submit(ctx context.Context, call domain.Call) {
	agent, ok := s.agents[call.Agent]
	if !ok {
		_ = s.repository.Fail(ctx, call.ID, "unknown_agent")
		return
	}
	history, err := s.repository.LoadHistory(ctx, call.ID, agent.MaxHistory)
	if err != nil {
		_ = s.repository.Fail(ctx, call.ID, "could_not_load_conversation_history")
		return
	}
	idempotencyKey := call.ID
	if call.RetryCount > 0 {
		idempotencyKey = fmt.Sprintf("%s:profile-sync-retry:%d", call.ID, call.RetryCount)
	}
	runID, err := s.runtime.Start(ctx, agent, string(call.Input), history, idempotencyKey)
	if err != nil {
		_ = s.repository.Fail(ctx, call.ID, "hermes_submission_failed")
		return
	}
	_ = s.repository.SetRun(ctx, call.ID, runID)
}

func (s *Service) pollActive(ctx context.Context) {
	active, err := s.repository.ListActive(ctx)
	if err != nil {
		return
	}
	var wait sync.WaitGroup
	for _, call := range active {
		call := call
		agent, ok := s.agents[call.Agent]
		if !ok || call.HermesRunID == nil {
			continue
		}
		wait.Add(1)
		go func() {
			defer wait.Done()
			state, err := s.runtime.Status(ctx, agent, *call.HermesRunID)
			if err != nil {
				return
			}
			switch state.Status {
			case "started", "running", "stopping":
				_ = s.repository.SetStatus(ctx, call.ID, domain.CallRunning, "")
			case "completed":
				if call.Agent != domain.AgentValera {
					_ = s.repository.Complete(ctx, call.ID, state.Output)
					return
				}
				expectedRevision, ok := profileRevision(call.Input)
				if !ok {
					_ = s.repository.SetStatus(ctx, call.ID, domain.CallFailed, "invalid_profile_revision")
					return
				}
				profileState, completed, err := s.repository.CompleteValera(ctx, call.ID, state.Output, call.ScopeID, expectedRevision)
				if err != nil || completed {
					return
				}
				if profileState.Revision > expectedRevision {
					_ = s.repository.SetStatus(ctx, call.ID, domain.CallFailed, "profile_revision_superseded")
					return
				}
				if call.RetryCount > 0 {
					_ = s.repository.SetStatus(ctx, call.ID, domain.CallFailed, "company_profile_not_synced")
					return
				}
				_, _ = s.repository.RetryProfileSave(ctx, call.ID, call.ScopeID, expectedRevision)
			case "failed", "cancelled", "interrupted", "unknown":
				status := domain.CallFailed
				if state.Status == "cancelled" {
					status = domain.CallCancelled
				} else if state.Status == "interrupted" || state.Status == "unknown" {
					status = domain.CallInterrupted
				}
				message := state.Error
				if message == "" {
					message = "hermes_run_" + state.Status
				}
				_ = s.repository.SetStatus(ctx, call.ID, status, message)
			}
		}()
	}
	wait.Wait()
}

func profileRevision(input json.RawMessage) (int64, bool) {
	var payload struct {
		ProfileRevision json.RawMessage `json:"profile_revision"`
	}
	if err := json.Unmarshal(input, &payload); err != nil || len(payload.ProfileRevision) == 0 {
		return 0, false
	}
	var revision int64
	if err := json.Unmarshal(payload.ProfileRevision, &revision); err != nil || revision < 1 {
		return 0, false
	}
	return revision, true
}
