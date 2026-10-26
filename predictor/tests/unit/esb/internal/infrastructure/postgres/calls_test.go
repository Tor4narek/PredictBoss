package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
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

func TestKonstantinAssistantTextDoesNotRequireCitationFields(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "JSON text without evidence",
			output: `{"text":"A concise answer."}`,
			want:   "A concise answer.",
		},
		{
			name:   "JSON ignores reasoning fields",
			output: `{"text":"A concise answer.","thoughts":"private reasoning"}`,
			want:   "A concise answer.",
		},
		{
			name:   "JSON with only reasoning has no answer",
			output: `{"thoughts":"private reasoning"}`,
			want:   "",
		},
		{
			name:   "answer alias",
			output: `{"answer":"A concise answer."}`,
			want:   "A concise answer.",
		},
		{
			name:   "plain text",
			output: "A concise answer without a JSON wrapper.",
			want:   "A concise answer without a JSON wrapper.",
		},
		{
			name:   "fenced JSON",
			output: "```json\n{\"text\":\"A concise answer.\"}\n```",
			want:   "A concise answer.",
		},
		{
			name:   "malformed JSON object",
			output: `{"text":`,
			want:   "",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := konstantinAssistantText(test.output); got != test.want {
				t.Fatalf("konstantinAssistantText() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestSanitizedNewsInputRemovesTextAndPreservesRetryHash(t *testing.T) {
	text := "cleaned article"
	digest := sha256.Sum256([]byte(text))
	hash := hex.EncodeToString(digest[:])
	input := []byte(`{"news_id":"news-1","source_id":"source-1","news_text":"cleaned article","title":"Headline"}`)

	sanitized, err := sanitizedNewsInput(input, hash)
	if err != nil {
		t.Fatalf("sanitizedNewsInput returned error: %v", err)
	}
	if strings.Contains(string(sanitized), text) || strings.Contains(string(sanitized), `"news_text"`) {
		t.Fatalf("sanitized input retained the article text: %s", sanitized)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(sanitized, &fields); err != nil {
		t.Fatal(err)
	}
	var gotHash string
	if err := json.Unmarshal(fields["news_text_sha256"], &gotHash); err != nil || gotHash != hash {
		t.Fatalf("sanitized hash = %q, want %q (error %v)", gotHash, hash, err)
	}
	// Completing the same input again after a partial transaction is safe.
	repeated, err := sanitizedNewsInput(sanitized, hash)
	if err != nil {
		t.Fatalf("sanitized retry returned error: %v", err)
	}
	if string(repeated) != string(sanitized) {
		t.Fatalf("sanitization retry changed payload: %s != %s", repeated, sanitized)
	}
}

func TestSanitizedNewsInputRejectsUnexpectedHash(t *testing.T) {
	if _, err := sanitizedNewsInput([]byte(`{"news_text":"different article"}`), strings.Repeat("0", 64)); err == nil {
		t.Fatal("sanitizedNewsInput accepted an unconfirmed content hash")
	}
}

func TestSameNewsRequestSurvivesSourceTextScrubbing(t *testing.T) {
	original := []byte(`{"news_id":"news-1","source_id":"source-1","news_text":"cleaned article","title":"Headline"}`)
	digest := sha256.Sum256([]byte("cleaned article"))
	hash, _ := json.Marshal(hex.EncodeToString(digest[:]))
	scrubbed := []byte(`{"news_id":"news-1","source_id":"source-1","news_text_sha256":` + string(hash) + `,"title":"Headline"}`)
	if !sameNewsRequest(original, scrubbed) || !sameNewsRequest(scrubbed, original) {
		t.Fatal("idempotent retry was not recognized after the original text was scrubbed")
	}
	changed := []byte(`{"news_id":"news-1","source_id":"source-1","news_text":"changed article","title":"Headline"}`)
	if sameNewsRequest(changed, scrubbed) {
		t.Fatal("scrubbed idempotency accepted a different news body")
	}
}
