package cli

import (
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/geodro/lerd/internal/config"
	"github.com/geodro/lerd/internal/feedback"
)

// requiredWorktreeDBChoice returns the database choice a framework definition
// forces for a new worktree, or "" when the user should be prompted as usual.
// An app that keeps deployment state in the database cannot share the parent's:
// applying the worktree's own config there would break the parent site.
func requiredWorktreeDBChoice(fw *config.Framework) string {
	if fw == nil || fw.Worktree == nil || !strings.EqualFold(fw.Worktree.DBIsolation, "required") {
		return ""
	}
	if strings.EqualFold(fw.Worktree.DBSource, "main") {
		return "clone-main"
	}
	return "empty"
}

// worktreeSetupArgs turns a command the definition declares into the console
// invocation that runs it, so nothing in Go needs to know what the command means.
// Returns nil when the framework declares no console to run it with.
func worktreeSetupArgs(fw *config.Framework, command string) []string {
	if fw == nil || fw.Console == "" || strings.TrimSpace(command) == "" {
		return nil
	}
	return append([]string{fw.Console}, strings.Fields(command)...)
}

// runWorktreeSetupCommands runs the console commands a framework declares for a
// new worktree, once its env file and database are both in place. Magento seeds
// its own base URL into env.php, which changes the config hash it stores in the
// database, and it refuses to serve until that config is imported.
func runWorktreeSetupCommands(fw *config.Framework, worktreePath string, log io.Writer) {
	if fw == nil || fw.Worktree == nil {
		return
	}
	for _, command := range fw.Worktree.Commands {
		args := worktreeSetupArgs(fw, command)
		if args == nil {
			continue
		}
		step := feedback.StartOn(log, command)
		code, err := RunPHPCapture(worktreePath, args)
		if err == nil && code != 0 {
			err = fmt.Errorf("exited %d", code)
		}
		if err != nil {
			step.Fail(err)
			continue
		}
		step.OK("")
	}
}

// unattendedWorktreeDBChoice is the database choice for a setup with nobody to
// ask: the definition's required isolation wins, otherwise the caller's request,
// otherwise the parent's database like the prompt's documented default.
func unattendedWorktreeDBChoice(fw *config.Framework, requested string) string {
	if forced := requiredWorktreeDBChoice(fw); forced != "" {
		return forced
	}
	if requested == "" {
		return "share"
	}
	return requested
}

// worktreeMigrateCommand returns the shell command the definition names as the
// one that applies its schema, or "" when it declares none.
func worktreeMigrateCommand(fw *config.Framework) string {
	if fw == nil || fw.Doctor == nil || fw.Doctor.MigrateCommand == "" {
		return ""
	}
	for _, c := range fw.Commands {
		if c.Name == fw.Doctor.MigrateCommand {
			return c.Command
		}
	}
	return ""
}

// migrationsDBChoiceFor picks a worktree's database from its migrations when the
// definition says where they live, or returns "" to leave the choice alone.
func migrationsDBChoiceFor(fw *config.Framework, parentPath, worktreePath string) (choice, reason string) {
	if fw == nil || fw.Worktree == nil || fw.Worktree.Migrations == "" {
		return "", ""
	}
	return migrationsDBChoice(filepath.Join(parentPath, fw.Worktree.Migrations), filepath.Join(worktreePath, fw.Worktree.Migrations))
}

// migrationsDBChoice compares the migration files of the parent checkout, whose
// database share reuses and clone-main copies, with the new worktree's. A branch
// behind the parent cannot run on the parent's newer schema, so it starts empty.
func migrationsDBChoice(parentDir, worktreeDir string) (choice, reason string) {
	parent, err := migrationFiles(parentDir)
	if err != nil {
		return "", ""
	}
	branch, err := migrationFiles(worktreeDir)
	if err != nil {
		return "", ""
	}
	branchOnly, parentOnly := 0, 0
	for f := range branch {
		if !parent[f] {
			branchOnly++
		}
	}
	for f := range parent {
		if !branch[f] {
			parentOnly++
		}
	}
	switch {
	case parentOnly > 0:
		return "empty", fmt.Sprintf("the parent checkout has migrations this branch lacks (%d), so its database is ahead of this code", parentOnly)
	case branchOnly > 0:
		return "clone-main", fmt.Sprintf("this branch adds migrations the parent checkout lacks (%d), so it gets a copy of the parent's database to run them on", branchOnly)
	default:
		return "share", "the migrations match the parent checkout's"
	}
}

// migrationFiles lists the files under dir by path relative to it.
func migrationFiles(dir string) (map[string]bool, error) {
	files := map[string]bool{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			rel, _ := filepath.Rel(dir, path)
			files[rel] = true
		}
		return nil
	})
	return files, err
}

// planUnattendedWorktreeDB decides the database for a setup with nobody to ask:
// with no explicit or required choice it compares migrations, and it migrates
// any database the branch's code has not been run against yet.
func planUnattendedWorktreeDB(fw *config.Framework, requested, parentPath, worktreePath string) (choice, reason string, migrate bool) {
	if requested == "" && requiredWorktreeDBChoice(fw) == "" {
		if c, why := migrationsDBChoiceFor(fw, parentPath, worktreePath); c != "" {
			return c, why, c != "share"
		}
	}
	choice = unattendedWorktreeDBChoice(fw, requested)
	return choice, "", dbChoiceYieldsEmptySchema(choice)
}
