package contextmemory

import (
	"strings"
	"testing"
)

func TestBuildQuery_IncludesIdentityHints(t *testing.T) {
	q := buildQuery(BucketRequest{
		Kind:         BucketLatestShortTerm,
		SessionKey:   "nm_sess:abc",
		ThreadKey:    "nm_thread:def",
		ClientFamily: "ampcode",
		Model:        "claude-opus-4-6",
		Query:        "fix auth",
	})
	if q == "" {
		t.Fatalf("query should not be empty")
	}
	for _, part := range []string{"fix auth", "session:nm_sess:abc", "thread:nm_thread:def", "client:ampcode", "model:claude-opus-4-6"} {
		if !containsInsensitive(q, part) {
			t.Fatalf("query missing part %q: %s", part, q)
		}
	}
}

func TestParseRecallJSON(t *testing.T) {
	ans, score := parseRecallJSON([]byte(`{"answer":"hello","confidence":0.91}`))
	if ans != "hello" {
		t.Fatalf("answer = %q, want hello", ans)
	}
	if score < 0.9 {
		t.Fatalf("score = %f, want >= 0.9", score)
	}
}

func containsInsensitive(s, part string) bool {
	if len(part) == 0 {
		return true
	}
	ls := []rune(strings.ToLower(s))
	lp := []rune(strings.ToLower(part))
	for i := 0; i+len(lp) <= len(ls); i++ {
		ok := true
		for j := 0; j < len(lp); j++ {
			if ls[i+j] != lp[j] {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}
