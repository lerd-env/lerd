package cli

import (
	"bytes"
	"errors"
	"testing"

	"github.com/geodro/lerd/internal/feedback"
	"github.com/geodro/lerd/internal/serviceops"
)

// `service preset` registers without starting and points the user at `service
// start`, so a two-step install never had a running service to provision
// against and its linked sites kept pointing at buckets nobody created.
func TestReprovisionOnStart_ReportsWhatItCreated(t *testing.T) {
	var buf bytes.Buffer
	defer feedback.SetTestWriter(&buf)()

	prev := reprovisionOnStartFn
	reprovisionOnStartFn = func(name string, emit func(serviceops.PhaseEvent)) error {
		emit(serviceops.PhaseEvent{Phase: "reprovisioning_site", Message: "shop: created bucket shop"})
		return nil
	}
	t.Cleanup(func() { reprovisionOnStartFn = prev })

	reprovisionOnStart("rustfs")

	if !bytes.Contains(buf.Bytes(), []byte("created bucket shop")) {
		t.Errorf("expected the created bucket to be reported, got %q", buf.String())
	}
}

// A site that cannot be provisioned must not fail the start: the service is up,
// which is what the command was asked to do.
func TestReprovisionOnStart_AFailureIsAWarning(t *testing.T) {
	var buf bytes.Buffer
	defer feedback.SetTestWriter(&buf)()

	prev := reprovisionOnStartFn
	reprovisionOnStartFn = func(string, func(serviceops.PhaseEvent)) error {
		return errors.New("engine unreachable")
	}
	t.Cleanup(func() { reprovisionOnStartFn = prev })

	reprovisionOnStart("mysql")

	if !bytes.Contains(buf.Bytes(), []byte("engine unreachable")) {
		t.Errorf("expected the failure to be surfaced, got %q", buf.String())
	}
}
