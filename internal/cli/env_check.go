package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/geodro/lerd/internal/envfile"
	"github.com/geodro/lerd/internal/feedback"
	"github.com/pmezard/go-difflib/difflib"
	"github.com/spf13/cobra"
)

// NewEnvCheckCmd returns the env:check command.
func NewEnvCheckCmd() *cobra.Command {
	var fix bool
	cmd := &cobra.Command{
		Use:          "env:check",
		Short:        "Compare all .env files against .env.example and flag missing keys",
		SilenceUsage: true,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runEnvCheck(fix)
		},
	}
	cmd.Flags().BoolVar(&fix, "fix", false, "Insert missing keys into each .env, placed beside the neighbours they have in .env.example")
	return cmd
}

// diffEnvKeys compares envFile against exampleFile and returns
// keys missing from envFile and keys extra in envFile.
func diffEnvKeys(exampleFile, envFile string) (missing, extra []string) {
	exampleKeys, err := envfile.ReadKeys(exampleFile)
	if err != nil {
		return nil, nil
	}
	envKeys, err := envfile.ReadKeys(envFile)
	if err != nil {
		return nil, nil
	}

	envSet := make(map[string]bool, len(envKeys))
	for _, k := range envKeys {
		envSet[k] = true
	}
	exampleSet := make(map[string]bool, len(exampleKeys))
	for _, k := range exampleKeys {
		exampleSet[k] = true
	}

	for _, k := range exampleKeys {
		if !envSet[k] {
			missing = append(missing, k)
		}
	}
	for _, k := range envKeys {
		if !exampleSet[k] {
			extra = append(extra, k)
		}
	}
	return missing, extra
}

// findEnvFiles returns all .env* files in dir except .env.example, sorted.
func findEnvFiles(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, ".env") || e.IsDir() {
			continue
		}
		// Skip files lerd manages internally: .env.example is the baseline we
		// compare against, .env.lerd_override is a partial personal overlay, and
		// .env.before_lerd is a pre-lerd backup — none are full env files.
		if name == ".env.example" || name == envOverrideFile || name == ".env.before_lerd" {
			continue
		}
		files = append(files, filepath.Join(dir, name))
	}
	sort.Strings(files)
	return files
}

func runEnvCheck(fix bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}

	examplePath := filepath.Join(cwd, ".env.example")
	if _, err := os.Stat(examplePath); os.IsNotExist(err) {
		return fmt.Errorf("no .env.example found in %s", cwd)
	}

	envFiles := findEnvFiles(cwd)
	if len(envFiles) == 0 {
		return fmt.Errorf("no .env files found in %s — run lerd env to create one", cwd)
	}

	if fix {
		return fixEnvFiles(examplePath, envFiles)
	}

	exampleKeys, err := envfile.ReadKeys(examplePath)
	if err != nil {
		return fmt.Errorf("reading .env.example: %w", err)
	}

	// Build per-file key sets.
	type fileInfo struct {
		name   string
		keySet map[string]bool
		keys   []string
	}
	var files []fileInfo
	for _, f := range envFiles {
		keys, err := envfile.ReadKeys(f)
		if err != nil {
			continue
		}
		set := make(map[string]bool, len(keys))
		for _, k := range keys {
			set[k] = true
		}
		files = append(files, fileInfo{
			name:   filepath.Base(f),
			keySet: set,
			keys:   keys,
		})
	}

	// Collect all keys that have at least one mismatch.
	allKeys := make(map[string]bool)
	exampleSet := make(map[string]bool, len(exampleKeys))
	for _, k := range exampleKeys {
		exampleSet[k] = true
	}

	// Check for keys missing from any env file or extra in any env file.
	for _, k := range exampleKeys {
		for _, f := range files {
			if !f.keySet[k] {
				allKeys[k] = true
				break
			}
		}
	}
	for _, f := range files {
		for _, k := range f.keys {
			if !exampleSet[k] {
				allKeys[k] = true
			}
		}
	}

	if len(allKeys) == 0 {
		if len(files) == 1 {
			fmt.Printf("  %s is in sync with .env.example\n", files[0].name)
		} else {
			fmt.Println("  all .env files are in sync with .env.example")
		}
		return nil
	}

	// Sort keys for stable output.
	var sortedKeys []string
	for k := range allKeys {
		sortedKeys = append(sortedKeys, k)
	}
	sort.Strings(sortedKeys)

	// Build the matrix: one column per env source, a ✓/✗ cell per key.
	headers := make([]string, 0, len(files)+2)
	headers = append(headers, "KEY", ".env.example")
	for _, f := range files {
		headers = append(headers, f.name)
	}
	rows := make([][]string, 0, len(sortedKeys))
	for _, k := range sortedKeys {
		row := make([]string, 0, len(files)+2)
		row = append(row, k, envMark(exampleSet[k]))
		for _, f := range files {
			row = append(row, envMark(f.keySet[k]))
		}
		rows = append(rows, row)
	}
	feedback.Table(headers, rows)

	fmt.Println()
	fmt.Printf("  %d key(s) out of sync\n", len(sortedKeys))

	return nil
}

func envMark(present bool) string {
	if present {
		return feedback.Green("✓")
	}
	return feedback.Red("✗")
}

// mergeEnvFile reads the example and one env file and returns the proposed merge
// that adds the example's missing keys next to their neighbours. Split out from
// fixEnvFiles so the placement is testable without the interactive prompt.
func mergeEnvFile(examplePath, envPath string) (envfile.MergeResult, error) {
	exampleContent, err := os.ReadFile(examplePath)
	if err != nil {
		return envfile.MergeResult{}, fmt.Errorf("reading .env.example: %w", err)
	}
	envContent, err := os.ReadFile(envPath)
	if err != nil {
		return envfile.MergeResult{}, err
	}
	return envfile.MergeMissing(string(exampleContent), string(envContent), nil), nil
}

// fixEnvFiles offers to insert each env file's missing example keys in place. The
// insertion is purely additive: existing values are never changed and keys the
// env has beyond the example (which a merge can't reconcile) are left untouched.
func fixEnvFiles(examplePath string, envFiles []string) error {
	proposed := 0
	for _, f := range envFiles {
		res, err := mergeEnvFile(examplePath, f)
		if err != nil || len(res.Added) == 0 {
			continue
		}
		info, err := os.Stat(f)
		if err != nil {
			continue
		}
		current, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		proposed++
		name := filepath.Base(f)
		printEnvDiff(name, string(current), res.Merged, res.Added)
		if !feedback.Interactive() {
			fmt.Printf("  %s: %d key(s) would be added (run interactively to apply)\n", name, len(res.Added))
			continue
		}
		if !feedback.Confirm(fmt.Sprintf("Insert %d missing key(s) into %s?", len(res.Added), name), true) {
			continue
		}
		if err := os.WriteFile(f, []byte(res.Merged), info.Mode().Perm()); err != nil {
			feedback.Warn("could not write %s: %v", name, err)
			continue
		}
		fmt.Printf("  %s Updated %s\n", feedback.Green("✓"), name)
	}
	if proposed == 0 {
		fmt.Println("  nothing to insert — every .env already has the example's keys")
	}
	return nil
}

// printEnvDiff previews a merge as a unified diff of the env file, so the
// surrounding context lines show where each inserted key lands, not just which
// keys were added.
func printEnvDiff(name, current, merged string, added []string) {
	diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        difflib.SplitLines(current),
		B:        difflib.SplitLines(merged),
		FromFile: name + " (current)",
		ToFile:   name + " (proposed)",
		Context:  2,
	})
	if err != nil {
		return
	}
	fmt.Printf("\n%s %s — %d key(s) to insert:\n\n", metaStyle.Render("~"), name, len(added))
	for _, line := range strings.Split(strings.TrimRight(diff, "\n"), "\n") {
		switch {
		case strings.HasPrefix(line, "+"):
			fmt.Println(addStyle.Render(line))
		case strings.HasPrefix(line, "-"):
			fmt.Println(delStyle.Render(line))
		case strings.HasPrefix(line, "@@"), strings.HasPrefix(line, "---"), strings.HasPrefix(line, "+++"):
			fmt.Println(metaStyle.Render(line))
		default:
			fmt.Println(line)
		}
	}
	fmt.Println()
}
