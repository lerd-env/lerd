package cli

import "testing"

func TestFrankenPHPBuildJobsSkipsCurrentImages(t *testing.T) {
	orig := needsFrankenPHPRebuild
	t.Cleanup(func() { needsFrankenPHPRebuild = orig })

	var asked []string
	needsFrankenPHPRebuild = func(versions []string) bool {
		asked = append(asked, versions...)
		return versions[0] == "8.4"
	}

	jobs := frankenPHPBuildJobs([]string{"8.3", "8.4"})
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(jobs))
	}
	if jobs[0].Label != "FrankenPHP 8.4" {
		t.Fatalf("unexpected label %q", jobs[0].Label)
	}
	if len(asked) != 2 {
		t.Fatalf("expected each version checked once, got %v", asked)
	}
}

func TestFrankenPHPBuildJobsEmptyWhenAllCurrent(t *testing.T) {
	orig := needsFrankenPHPRebuild
	t.Cleanup(func() { needsFrankenPHPRebuild = orig })
	needsFrankenPHPRebuild = func([]string) bool { return false }

	if jobs := frankenPHPBuildJobs([]string{"8.3", "8.4"}); len(jobs) != 0 {
		t.Fatalf("expected no jobs, got %d", len(jobs))
	}
}
