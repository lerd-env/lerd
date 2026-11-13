package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/version"
)

// uninstallFeedbackURL receives the optional answer to "why are you leaving".
// The Worker behind it lives in workers/uninstall-feedback.
const uninstallFeedbackURL = "https://feedback.lerd.sh/v1/uninstall"

const maxUninstallComment = 500

// uninstallReasons are the numbered choices; the key is what gets sent.
var uninstallReasons = []struct{ key, label string }{
	{"broken", "Something didn't work"},
	{"heavy", "Too slow or too heavy"},
	{"switched", "Switched to another tool"},
	{"trying", "I was only trying it out"},
	{"other", "Something else"},
}

type uninstallFeedback struct {
	Reason  string `json:"reason"`
	Comment string `json:"comment,omitempty"`
	Version string `json:"version"`
	OS      string `json:"os"`
}

// offerUninstallFeedback asks the one optional question of an interactive
// uninstall and sends the answer. It never fails the uninstall.
func offerUninstallFeedback() {
	if !feedback.StdinInteractive() {
		return
	}
	if !version.IsRelease(version.Version) || version.Commit == "none" {
		return
	}
	fmt.Println()
	feedback.Line("before we start: why are you uninstalling? (optional, sent anonymously)")
	for i, r := range uninstallReasons {
		feedback.Note(fmt.Sprintf("%d  %s", i+1, r.label))
	}
	answer, ok := uninstallSurvey(feedback.Ask)
	if !ok {
		return
	}
	answer.Version = version.Version
	answer.OS = runtime.GOOS
	if err := sendUninstallFeedback(uninstallFeedbackURL, answer); err != nil {
		feedback.Note("could not send it, thanks anyway")
		return
	}
	feedback.Note("thank you, it helps")
}

// uninstallSurvey turns the replies into an answer, or false when skipped.
// Text typed instead of a number is kept as an "other" answer rather than lost.
func uninstallSurvey(ask func(string) string) (uninstallFeedback, bool) {
	pick := ask(fmt.Sprintf("Pick 1-%d, or press Enter to skip:", len(uninstallReasons)))
	if pick == "" {
		return uninstallFeedback{}, false
	}
	n, err := strconv.Atoi(pick)
	if err != nil || n < 1 || n > len(uninstallReasons) {
		return uninstallFeedback{Reason: "other", Comment: capComment(pick)}, true
	}
	comment := ask("Anything you'd like to add? (Enter to skip)")
	return uninstallFeedback{Reason: uninstallReasons[n-1].key, Comment: capComment(comment)}, true
}

func capComment(s string) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > maxUninstallComment {
		return string(r[:maxUninstallComment])
	}
	return s
}

// sendUninstallFeedback posts the answer, giving up quickly: nobody should wait
// on a survey before their uninstall runs.
func sendUninstallFeedback(url string, f uninstallFeedback) error {
	body, err := json.Marshal(f)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("feedback endpoint returned %s", resp.Status)
	}
	return nil
}
