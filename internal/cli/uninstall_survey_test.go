package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// answers feeds a fixed list of replies to the survey, one per question.
func answers(replies ...string) func(string) string {
	return func(string) string {
		if len(replies) == 0 {
			return ""
		}
		r := replies[0]
		replies = replies[1:]
		return r
	}
}

func TestUninstallSurveyPicksTheNumberedReason(t *testing.T) {
	got, ok := uninstallSurvey(answers("2", "  uses too much RAM "))
	if !ok || got.Reason != "heavy" || got.Comment != "uses too much RAM" {
		t.Fatalf("survey = %+v, %v", got, ok)
	}
}

func TestUninstallSurveySkippedOnEnter(t *testing.T) {
	if got, ok := uninstallSurvey(answers("")); ok {
		t.Fatalf("an empty answer must send nothing, got %+v", got)
	}
}

// Someone who types their reason instead of a number has still told us why;
// keep the words as an "other" answer rather than dropping them.
func TestUninstallSurveyKeepsTextTypedInsteadOfANumber(t *testing.T) {
	got, ok := uninstallSurvey(answers("moving to docker", "unused"))
	if !ok || got.Reason != "other" || got.Comment != "moving to docker" {
		t.Fatalf("survey = %+v, %v", got, ok)
	}
}

func TestUninstallSurveyRejectsAnOutOfRangeNumber(t *testing.T) {
	got, ok := uninstallSurvey(answers("9", ""))
	if !ok || got.Reason != "other" || got.Comment != "9" {
		t.Fatalf("survey = %+v, %v", got, ok)
	}
}

func TestUninstallSurveyCapsTheComment(t *testing.T) {
	long := make([]byte, 2000)
	for i := range long {
		long[i] = 'x'
	}
	got, _ := uninstallSurvey(answers("5", string(long)))
	if len(got.Comment) != maxUninstallComment {
		t.Fatalf("comment length %d, want %d", len(got.Comment), maxUninstallComment)
	}
}

func TestSendUninstallFeedbackPostsJSON(t *testing.T) {
	var body map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("got %s %s", r.Method, r.Header.Get("Content-Type"))
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	err := sendUninstallFeedback(srv.URL, uninstallFeedback{Reason: "broken", Comment: "dns", Version: "1.38.0", OS: "linux"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"reason": "broken", "comment": "dns", "version": "1.38.0", "os": "linux"}
	for k, v := range want {
		if body[k] != v {
			t.Errorf("%s = %q, want %q (body %v)", k, body[k], v, body)
		}
	}
}

func TestSendUninstallFeedbackReportsAServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if err := sendUninstallFeedback(srv.URL, uninstallFeedback{Reason: "other"}); err == nil {
		t.Fatal("a 500 must come back as an error")
	}
}

func TestSendUninstallFeedbackReportsAnUnreachableEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()

	if err := sendUninstallFeedback(url, uninstallFeedback{Reason: "other"}); err == nil {
		t.Fatal("a closed endpoint must come back as an error")
	}
}
