package cli

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	nodeDet "github.com/geodro/lerd/internal/node"
)

// stubNodeUpdates makes the host report statuses and records each update.
func stubNodeUpdates(t *testing.T, statuses []nodeDet.VersionStatus, fail string) *[]string {
	t.Helper()
	origStatuses, origUpdate := nodeStatusesFn, nodeUpdateFn
	t.Cleanup(func() { nodeStatusesFn, nodeUpdateFn = origStatuses, origUpdate })
	nodeStatusesFn = func(context.Context) []nodeDet.VersionStatus { return statuses }
	var updated []string
	nodeUpdateFn = func(_ context.Context, major string) (string, error) {
		updated = append(updated, major)
		if major == fail {
			return "", errors.New("download failed")
		}
		return major + ".9.9", nil
	}
	return &updated
}

var (
	node22Behind  = nodeDet.VersionStatus{Major: "22", Installed: "22.1.0", Latest: "22.2.0", UpdateAvailable: true}
	node24Behind  = nodeDet.VersionStatus{Major: "24", Installed: "24.1.0", Latest: "24.2.0", UpdateAvailable: true}
	node20Current = nodeDet.VersionStatus{Major: "20", Installed: "20.2.0", Latest: "20.2.0"}
)

func TestUpdateNodeVersions_UpdatesOnlyWhatIsBehind(t *testing.T) {
	updated := stubNodeUpdates(t, []nodeDet.VersionStatus{node20Current, node22Behind, node24Behind}, "")

	if err := updateNodeVersions(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if want := []string{"22", "24"}; !reflect.DeepEqual(*updated, want) {
		t.Errorf("updated %v, want %v", *updated, want)
	}
}

func TestUpdateNodeVersions_NamesOneMajor(t *testing.T) {
	updated := stubNodeUpdates(t, []nodeDet.VersionStatus{node22Behind, node24Behind}, "")

	if err := updateNodeVersions(context.Background(), "24"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"24"}; !reflect.DeepEqual(*updated, want) {
		t.Errorf("updated %v, want %v", *updated, want)
	}
}

func TestUpdateNodeVersions_ContinuesPastAFailure(t *testing.T) {
	updated := stubNodeUpdates(t, []nodeDet.VersionStatus{node22Behind, node24Behind}, "22")

	if err := updateNodeVersions(context.Background(), ""); err == nil {
		t.Error("a failed update must still be reported")
	}
	if len(*updated) != 2 {
		t.Errorf("stopped at the first failure: %v", *updated)
	}
}

func TestUpdateNodeVersions_RefusesAMajorThatIsNotInstalled(t *testing.T) {
	updated := stubNodeUpdates(t, []nodeDet.VersionStatus{node22Behind}, "")

	err := updateNodeVersions(context.Background(), "18")
	if err == nil || !strings.Contains(err.Error(), "18") {
		t.Fatalf("err = %v, want one naming 18", err)
	}
	if len(*updated) != 0 {
		t.Errorf("updated %v", *updated)
	}
}

// Offline is not the same as up to date, and saying so would be a false claim.
func TestUpdateNodeVersions_NoReleaseDataIsAnError(t *testing.T) {
	stubNodeUpdates(t, []nodeDet.VersionStatus{{Major: "24", Installed: "24.1.0"}}, "")

	if err := updateNodeVersions(context.Background(), ""); err == nil {
		t.Fatal("reported success without knowing what was published")
	}
}

func TestUpdateNodeVersions_NothingBehindIsFine(t *testing.T) {
	updated := stubNodeUpdates(t, []nodeDet.VersionStatus{node20Current}, "")

	if err := updateNodeVersions(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	if len(*updated) != 0 {
		t.Errorf("updated %v", *updated)
	}
}

// A manager that cannot list its versions looks the same as an empty host, and
// neither is a reason to say Node is up to date.
func TestUpdateNodeVersions_NothingListedIsAnError(t *testing.T) {
	stubNodeUpdates(t, nil, "")

	if err := updateNodeVersions(context.Background(), ""); err == nil {
		t.Fatal("reported up to date without seeing a single version")
	}
}
