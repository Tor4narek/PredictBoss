package postgres

import (
	"context"
	"testing"

	"github.com/predictboss/predictor/esb/internal/domain"
)

func TestLoadHistoryZeroSkipsDatabaseQuery(t *testing.T) {
	repository := NewCalls(nil)

	messages, err := repository.LoadHistory(context.Background(), "call-id", 0)

	if err != nil {
		t.Fatalf("LoadHistory returned error: %v", err)
	}
	if messages == nil || len(messages) != 0 {
		t.Fatalf("LoadHistory(0) = %#v, want an empty non-nil slice", messages)
	}
}

func TestLoadHistoryRejectsNegativeLimit(t *testing.T) {
	repository := NewCalls(nil)

	if _, err := repository.LoadHistory(context.Background(), "call-id", -1); err != domain.ErrConflict {
		t.Fatalf("LoadHistory(-1) error = %v, want %v", err, domain.ErrConflict)
	}
}
