---
name: lerd-open-pr
description: Draft and open a pull request the lerd way — issue-first, correct issue linking, human prose, none of the banned sections. Use when asked to open, prepare, or draft a PR for lerd. Never creates anything on GitHub without explicit per-action approval.
---

# Open a lerd PR

Every write to GitHub (create/edit/close/reopen/merge/comment on an issue or PR)
needs explicit approval **each time**. Draft the text, show it, and wait. Do not
run `gh pr create` or any state-changing `gh` command until the human says go.

## Before the PR

1. **An issue should already exist** for the work, framed as future work. If it
   doesn't, draft one and ask before creating it.
2. Run `/lerd-preflight` — the PR is not ready until the local gate is green.
3. Confirm the branch is off `main`, not `main` itself, and staged by explicit
   path (never `git add -A`). `git status` first.

## PR body — write it as a human would

Prose paragraphs, single-line (no column wrapping), explaining what changed and
why. Then the issue link:

- Feature PR → `Closes #N` (auto-closes on merge).
- Bug-report issue → `Refs #N` (stays open until the stable release ships).
- Security issue → `Closes #N` (closes once the fix merges to main).

## Never include

- A Test plan section.
- A Verified / Tested / Manual testing section or trailer.
- A checklist of any kind (`- [ ]` / `- [x]`, "Release checklist"…).
- A "Notes for reviewers" section — we own the project, there is no external reviewer.
- `file:line` citations, em dashes, `Co-Authored-By`, or "Generated with…" footers.
- Prose about tests, TDD, or coverage; and don't mention incidental cleanup.

## PR and issue comments

Casual plain prose. No markdown, no bullets, no hyphens — commas instead. Don't
open with boilerplate like "Pulled it down and put it on my install." Vary it.

## After pushing

Return immediately. Don't sit polling CI; failures get flagged by the human.
