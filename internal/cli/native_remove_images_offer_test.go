package cli

import (
	"bytes"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
)

// imageOffer stubs the image store, the removal and the question, and returns
// what was removed, whether it asked, and what the command printed.
type imageOffer struct {
	removed []string
	asked   bool
	swept   int
	out     *bytes.Buffer
}

func stubImageOffer(t *testing.T, onDisk []string, terminal, answer bool) *imageOffer {
	t.Helper()
	o := &imageOffer{out: &bytes.Buffer{}}
	origExists, origRemove, origConfirm, origMode, origSweep := fpmImageExistsFn, removeFPMImageFn, confirmImageRemovalFn, currentPHPRuntimeFn, sweepOrphanedImagesFn
	sweepOrphanedImagesFn = func() { o.swept++ }
	currentPHPRuntimeFn = func() (string, error) { return config.PHPRuntimeNative, nil }
	fpmImageExistsFn = func(v string) bool { return slices.Contains(onDisk, v) }
	removeFPMImageFn = func(v string) error { o.removed = append(o.removed, v); return nil }
	confirmImageRemovalFn = func(string, bool) bool { o.asked = true; return answer }
	restoreTTY := feedback.SetStdinInteractive(terminal)
	restoreOut := feedback.Redirect(o.out)
	t.Cleanup(func() {
		fpmImageExistsFn, removeFPMImageFn, confirmImageRemovalFn, currentPHPRuntimeFn, sweepOrphanedImagesFn = origExists, origRemove, origConfirm, origMode, origSweep
		restoreTTY()
		restoreOut()
	})
	return o
}

func TestOfferFPMImageRemovalRemovesTheImagesLeftWhenTheAnswerIsYes(t *testing.T) {
	o := stubImageOffer(t, []string{"8.4", "8.5"}, true, true)

	offerFPMImageRemoval(config.PHPRuntimeNative, []string{"8.3", "8.4", "8.5"}, false)

	if !o.asked {
		t.Fatal("a terminal should be asked")
	}
	// 8.3 has no image any more, so it is neither offered nor removed.
	if !slices.Equal(o.removed, []string{"8.4", "8.5"}) {
		t.Errorf("removed %v, want the two images still on disk", o.removed)
	}
	if hint := reclaimedHostHint(); hint != "" && !strings.Contains(o.out.String(), "lerd machine reclaim") {
		t.Errorf("on macOS the space stays in the VM until reclaimed, so say so; got %q", o.out.String())
	}
}

func TestOfferFPMImageRemovalKeepsTheImagesWhenTheAnswerIsNo(t *testing.T) {
	o := stubImageOffer(t, []string{"8.4"}, true, false)

	offerFPMImageRemoval(config.PHPRuntimeNative, []string{"8.4"}, false)

	if !o.asked || len(o.removed) != 0 {
		t.Errorf("asked=%v removed=%v, want asked and nothing removed", o.asked, o.removed)
	}
}

// With no terminal a prompt would answer itself, so it names the flag instead.
func TestOfferFPMImageRemovalPrintsAHintWithNoTerminal(t *testing.T) {
	o := stubImageOffer(t, []string{"8.4", "8.5"}, false, true)

	offerFPMImageRemoval(config.PHPRuntimeNative, []string{"8.4", "8.5"}, false)

	if o.asked || len(o.removed) != 0 {
		t.Errorf("asked=%v removed=%v, want neither without a terminal", o.asked, o.removed)
	}
	got := o.out.String()
	if !strings.Contains(got, "8.4, 8.5") || !strings.Contains(got, "lerd php:runtime native --remove-images") {
		t.Errorf("the hint should name the versions and the flag, got %q", got)
	}
}

func TestOfferFPMImageRemovalWithTheFlagRemovesWithoutAsking(t *testing.T) {
	o := stubImageOffer(t, []string{"8.4"}, false, false)

	offerFPMImageRemoval(config.PHPRuntimeNative, []string{"8.4"}, true)

	if o.asked {
		t.Error("the flag is the answer, so nothing should be asked")
	}
	if !slices.Equal(o.removed, []string{"8.4"}) {
		t.Errorf("removed %v, want 8.4", o.removed)
	}
}

func TestOfferFPMImageRemovalSaysNothingWhenNoImageIsLeft(t *testing.T) {
	o := stubImageOffer(t, nil, true, true)

	offerFPMImageRemoval(config.PHPRuntimeNative, []string{"8.4", "8.5"}, false)

	if o.asked || len(o.removed) != 0 || o.out.Len() != 0 {
		t.Errorf("asked=%v removed=%v out=%q, want silence", o.asked, o.removed, o.out.String())
	}
}

// On the container runtime the images are serving, so there is nothing to offer.
func TestOfferFPMImageRemovalNeverOffersUnderTheContainerRuntime(t *testing.T) {
	o := stubImageOffer(t, []string{"8.4"}, true, true)

	offerFPMImageRemoval(config.PHPRuntimeContainer, []string{"8.4"}, true)

	if o.asked || len(o.removed) != 0 || o.out.Len() != 0 {
		t.Errorf("asked=%v removed=%v out=%q, want nothing", o.asked, o.removed, o.out.String())
	}
}

// A switch back to container while the question waits makes these the images
// serving every site, so a yes given after that must not remove them.
func TestOfferFPMImageRemovalRereadsTheRuntimeAfterTheAnswer(t *testing.T) {
	o := stubImageOffer(t, []string{"8.4"}, true, true)
	confirmImageRemovalFn = func(string, bool) bool {
		currentPHPRuntimeFn = func() (string, error) { return config.PHPRuntimeContainer, nil }
		return true
	}

	offerFPMImageRemoval(config.PHPRuntimeNative, []string{"8.4"}, false)

	if len(o.removed) != 0 {
		t.Errorf("removed %v after the runtime moved back to container", o.removed)
	}
	if !strings.Contains(o.out.String(), "switch to the native runtime") {
		t.Errorf("the refusal should be reported, got %q", o.out.String())
	}
}

// With no argument the command only shows the runtime, so the flag would be
// dropped silently; it has to be refused instead.
func TestPHPRuntimeRefusesRemoveImagesWithoutNative(t *testing.T) {
	for _, args := range [][]string{{"--remove-images"}, {"container", "--remove-images"}} {
		cmd := NewPHPRuntimeCmd()
		cmd.SetArgs(args)
		cmd.SetOut(&bytes.Buffer{})
		cmd.SetErr(&bytes.Buffer{})
		err := cmd.Execute()
		if err == nil || !strings.Contains(err.Error(), "--remove-images") {
			t.Errorf("%v: err = %v, want the flag refused", args, err)
		}
	}
}

// The base images the removed ones were built on keep their layers until swept,
// so the disk only comes back once the sweep runs after a removal.
func TestRemoveFPMImagesSweepsOnlyOnceSomethingWent(t *testing.T) {
	o := stubImageOffer(t, nil, false, false)
	if _, err := RemoveFPMImages([]string{"8.4"}); err != nil || o.swept != 1 {
		t.Errorf("err=%v swept=%d, want one sweep after a removal", err, o.swept)
	}

	o.swept = 0
	removeFPMImageFn = func(string) error { return errors.New("in use") }
	if _, err := RemoveFPMImages([]string{"8.4"}); err == nil || o.swept != 0 {
		t.Errorf("err=%v swept=%d, want no sweep when nothing went", err, o.swept)
	}
}

// stubPHPRuntimeCmd lets php:runtime run on any platform: the switch only moves
// the saved runtime, and the installed versions are the ones given.
func stubPHPRuntimeCmd(t *testing.T, from string, installed []string) {
	t.Helper()
	origApply, origUnavailable, origInstalled := applyPHPRuntimeFn, nativeUnavailableFn, installedPHPVersionsFn
	mode := from
	currentPHPRuntimeFn = func() (string, error) { return mode, nil }
	applyPHPRuntimeFn = func(m string) error { mode = m; return nil }
	nativeUnavailableFn = func() string { return "" }
	installedPHPVersionsFn = func() ([]string, error) { return installed, nil }
	t.Cleanup(func() {
		applyPHPRuntimeFn, nativeUnavailableFn, installedPHPVersionsFn = origApply, origUnavailable, origInstalled
	})
}

func runPHPRuntimeCmd(t *testing.T, args ...string) {
	t.Helper()
	cmd := NewPHPRuntimeCmd()
	cmd.SetArgs(args)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("%v: %v", args, err)
	}
}

func TestPHPRuntimeNativeOffersToRemoveTheImagesAfterTheSwitch(t *testing.T) {
	o := stubImageOffer(t, []string{"8.4"}, true, true)
	stubPHPRuntimeCmd(t, config.PHPRuntimeContainer, []string{"8.4"})

	runPHPRuntimeCmd(t, "native")

	if !o.asked || !slices.Equal(o.removed, []string{"8.4"}) {
		t.Errorf("asked=%v removed=%v, want the offer made and taken", o.asked, o.removed)
	}
}

func TestPHPRuntimeNativeWithTheFlagRemovesTheImagesWithoutAsking(t *testing.T) {
	for _, from := range []string{config.PHPRuntimeContainer, config.PHPRuntimeNative} {
		t.Run("from "+from, func(t *testing.T) {
			o := stubImageOffer(t, []string{"8.4", "8.5"}, false, false)
			stubPHPRuntimeCmd(t, from, []string{"8.4", "8.5"})

			runPHPRuntimeCmd(t, "native", "--remove-images")

			if o.asked || !slices.Equal(o.removed, []string{"8.4", "8.5"}) {
				t.Errorf("asked=%v removed=%v, want both removed unasked", o.asked, o.removed)
			}
		})
	}
}

// An install already native is only reported on; the offer belongs to the switch.
func TestPHPRuntimeAlreadyNativeDoesNotAskWithoutTheFlag(t *testing.T) {
	o := stubImageOffer(t, []string{"8.4"}, true, true)
	stubPHPRuntimeCmd(t, config.PHPRuntimeNative, []string{"8.4"})

	runPHPRuntimeCmd(t, "native")

	if o.asked || len(o.removed) != 0 {
		t.Errorf("asked=%v removed=%v, want neither", o.asked, o.removed)
	}
}
