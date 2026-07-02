package envfile

import (
	"reflect"
	"testing"
)

func TestMergeMissing(t *testing.T) {
	tests := []struct {
		name       string
		example    string
		env        string
		include    map[string]bool
		wantMerged string
		wantAdded  []string
	}{
		{
			name:       "insert keeps key inside its group",
			example:    "DB_HOST=localhost\nDB_PORT=5432\nDB_DATABASE=app\n",
			env:        "DB_HOST=lerd-postgres\nDB_DATABASE=app\n",
			wantMerged: "DB_HOST=lerd-postgres\nDB_PORT=5432\nDB_DATABASE=app\n",
			wantAdded:  []string{"DB_PORT"},
		},
		{
			name:       "missing before any shared key lands at top",
			example:    "APP_NAME=Lerd\nAPP_ENV=local\nDB_HOST=localhost\n",
			env:        "DB_HOST=lerd-postgres\n",
			wantMerged: "APP_NAME=Lerd\nAPP_ENV=local\nDB_HOST=lerd-postgres\n",
			wantAdded:  []string{"APP_NAME", "APP_ENV"},
		},
		{
			name:       "missing after last shared key lands at bottom",
			example:    "APP_ENV=local\nMAIL_MAILER=smtp\n",
			env:        "APP_ENV=production\n",
			wantMerged: "APP_ENV=production\nMAIL_MAILER=smtp\n",
			wantAdded:  []string{"MAIL_MAILER"},
		},
		{
			name:       "consecutive missing keys keep example order",
			example:    "A=1\nB=2\nC=3\nD=4\n",
			env:        "A=1\nD=4\n",
			wantMerged: "A=1\nB=2\nC=3\nD=4\n",
			wantAdded:  []string{"B", "C"},
		},
		{
			name:       "attached comment is carried but section header is not",
			example:    "# Database\nDB_HOST=localhost\n# outbound mail token\nMAIL_TOKEN=secret\n",
			env:        "DB_HOST=lerd-postgres\n",
			wantMerged: "DB_HOST=lerd-postgres\n# outbound mail token\nMAIL_TOKEN=secret\n",
			wantAdded:  []string{"MAIL_TOKEN"},
		},
		{
			name:       "include filter adds only requested keys",
			example:    "REQUIRED_KEY=x\nOPTIONAL_KEY=y\nANCHOR=z\n",
			env:        "ANCHOR=z\n",
			include:    map[string]bool{"REQUIRED_KEY": true},
			wantMerged: "REQUIRED_KEY=x\nANCHOR=z\n",
			wantAdded:  []string{"REQUIRED_KEY"},
		},
		{
			name:       "empty env gets all example keys in order",
			example:    "A=1\nB=2\n",
			env:        "",
			wantMerged: "A=1\nB=2\n",
			wantAdded:  []string{"A", "B"},
		},
		{
			name:       "no missing keys leaves env untouched",
			example:    "A=1\nB=2\n",
			env:        "A=9\nB=8\n",
			wantMerged: "A=9\nB=8\n",
			wantAdded:  nil,
		},
		{
			name:       "extra env keys not in example are preserved",
			example:    "A=1\nB=2\n",
			env:        "A=1\nCUSTOM=keepme\n",
			wantMerged: "A=1\nB=2\nCUSTOM=keepme\n",
			wantAdded:  []string{"B"},
		},
		{
			name:       "value with equals sign is copied verbatim",
			example:    "APP_KEY=base64:AAAA==\nANCHOR=z\n",
			env:        "ANCHOR=z\n",
			wantMerged: "APP_KEY=base64:AAAA==\nANCHOR=z\n",
			wantAdded:  []string{"APP_KEY"},
		},
		{
			name:       "env without trailing newline stays without one",
			example:    "A=1\nB=2",
			env:        "A=1",
			wantMerged: "A=1\nB=2",
			wantAdded:  []string{"B"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeMissing(tt.example, tt.env, tt.include)
			if got.Merged != tt.wantMerged {
				t.Errorf("Merged mismatch\n got: %q\nwant: %q", got.Merged, tt.wantMerged)
			}
			if !reflect.DeepEqual(got.Added, tt.wantAdded) {
				t.Errorf("Added mismatch\n got: %v\nwant: %v", got.Added, tt.wantAdded)
			}
		})
	}
}

// TestMergeMissing_addedLines verifies the reported line numbers point at the
// inserted lines in the merged output, including a carried comment.
func TestMergeMissing_addedLines(t *testing.T) {
	example := "DB_HOST=localhost\n# port note\nDB_PORT=5432\nDB_DATABASE=app\n"
	env := "DB_HOST=lerd-postgres\nDB_DATABASE=app\n"
	got := MergeMissing(example, env, nil)

	// Merged: DB_HOST(1), # port note(2), DB_PORT(3), DB_DATABASE(4).
	want := []int{2, 3}
	if !reflect.DeepEqual(got.AddedLines, want) {
		t.Fatalf("AddedLines = %v, want %v (merged=%q)", got.AddedLines, want, got.Merged)
	}
	lines := []string{"DB_HOST=lerd-postgres", "# port note", "DB_PORT=5432", "DB_DATABASE=app"}
	for _, ln := range got.AddedLines {
		if ln < 1 || ln > len(lines) {
			t.Fatalf("line %d out of range", ln)
		}
	}
}
