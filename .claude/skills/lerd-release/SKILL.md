---
name: lerd-release
description: Cut a lerd release, beta, patch or stable — lanes first, the changelog and README the lerd way, a release PR, the tag only after it merges, then the checks that need the published release. Use when asked to release, cut a beta, ship a version, or tag lerd. Every git and GitHub step needs the human's word.
---

# Cut a lerd release

A release is a PR that changes the changelog and README, then a tag on its
merge commit. The tag runs the release workflow, which builds, signs, attests,
updates the Homebrew tap and announces on Discord; nothing else publishes it.
Merging is not releasing: a binary change reaches users only at a tag.

Every commit, push, PR, merge, tag and issue edit needs the human's yes.

## 1. Lanes before the PR

- Lane the exact build the tag will ship: main as it will be tagged. Anything
  merged after the lanes, other than tests and docs, means another round.
- Run the guests three at a time: six at once on one public IP hits Docker
  Hub's anonymous pull limit (`toomanyrequests`).
- Give each guest 20G free before it starts; a full disk fails phases 6 to 15
  in ways that look like lerd bugs.
- Revert a snapshot while the VM is off, reapply its memory and CPUs, then boot:
  snapshots carry the old domain config and partition size. Grow the disk with
  `virsh blockresize` only once it runs (it does nothing on a stopped VM), then
  grow the partition and filesystem inside.
- Phase 12 (upgrade) only reaches published releases, so it runs after the tag.
- A FAIL is a bug until a rerun on clean state says otherwise; a rerun on top of
  a full lane's leftovers fails for its own reasons.

## 2. The changelog (`docs/changelog.md`, `CHANGELOG.md` links to it)

- **Beta:** the first beta adds `## [X.Y.0-beta.1] - <date>` above the last
  release; each later beta renames that heading in place and folds its changes
  into the same section.
- **Stable:** rename the last beta heading to `## [X.Y.0] - <date>`, add what
  merged since that beta, and drop any wording that called a feature beta.
- **Patch (X.Y.Z):** a new section above. A fix released right away carries its
  version bump as a second commit in the fix PR, no separate release PR.
- Never add `[Unreleased]` and never overwrite a released section.
- Sections: a short prose intro, then `### Added`, `### Changed`, `### Fixed`.
  Entries are `- **Title** (#PR). Prose…`, newest first within a section, and
  name an outside contributor with `contributed by @login`.
- Leave out fixes for bugs introduced and fixed between the last release and
  this one; fold them into the feature's entry instead. Check with
  `git show <last-tag>:<path>`.
- Leave out lane, test, CI and pinned-tool bumps; dependency bumps for an
  advisory join the "Dependencies with security advisories" entry.
- No em dashes.

List what merged since the last tag: `git log --oneline <last-tag>..origin/main`,
then read each PR's body, not just its title.

## 3. README and the rest of the PR

- README: update any line the release changes (a feature list, pinned tools).
- MCP: check `internal/cli/aidocs/` and `internal/plugins/guide.md` cover what
  changed for assistants; update them in this PR if not.
- **Stable only:**
  - The dev digest: `docs/public/digest/vX.Y.0.html` from
    `docs/digest-template.html`, its card with `node docs/scripts/digest-card.mjs
    vX.Y.0` (needs rsvg-convert and the fonts), and the landing banner in
    `docs/.vitepress/theme/components/LandingPage.vue` pointing at it.
  - The version fallbacks: `internal/version/version.go` (`Version`) and
    `internal/ui/web/demo/fixtures/version.json`, which the demo dashboard shows.
    Every stable release since 1.34 bumped them except 1.38.0, which left 1.37.0.

## 3b. The demo has data for what shipped

lerd.sh embeds a backend-free build of the dashboard (`internal/ui/web/demo`),
fed by `fixtures/` and `stubs.ts`. A feature with no fixture renders empty or
breaks there (Share on LAN crashed in 1.39 before `share-tools.json`).

- For each Added and Changed entry with a dashboard surface, give the demo data
  that shows it: a new endpoint gets a fixture and a line in `stubs.ts`, a new
  field goes into the existing fixture. Capture real answers from a running
  lerd and scrub names, paths and domains (a share base domain included);
  hand-formatted fixtures get their new block inserted as text, not rewritten.
- Build it and click through it:

  ```
  cd internal/ui/web && npm run build:demo        # → docs/public/demo/
  cd docs/public && python3 -m http.server 4180   # in another terminal
  PLAYWRIGHT=<path to playwright/index.js> \
    node .claude/skills/lerd-release/demo-crawl.mjs http://127.0.0.1:4180/demo/
  ```

  It clicks each rail section, list entry and tab, and lists the GETs that hit
  no fixture (the demo logs `[demo] no fixture for GET …`) and any page error.
  Both must be zero, then look at the new features in screenshots.
- Stable releases also bump `fixtures/version.json` (see above).

## 4. The release PR

- Branch `release/vX.Y.Z` off fresh `origin/main`; title `release: vX.Y.Z`.
- Body in prose: what the release brings, then what this PR changes.
- Stage files by path; no attribution trailers.
- Check Greptile reaches 5/5 and CI passes; the human merges.

## 5. The tag

- Only after the PR merges. `git fetch` alone, read the merge commit, then
  `git tag -a vX.Y.Z -m vX.Y.Z <merge-commit>` and push the tag.
- A fix that merges after the release PR and must ship goes into the changelog
  first, in a follow-up PR, and the tag goes on that newer commit instead
  (1.34.0, 1.34.2, 1.34.3 and 1.35.0 were tagged past their release PR).
- Confirm the Release workflow ran and `gh release view vX.Y.Z` lists the
  archives, `checksums.txt` and its `.sigstore.json`.

## 6. After the tag

- **Upgrade phase** on one guest: `LERD_CHANNEL=beta FROM_VERSION=<last stable>
  vm.sh run <guest> 12` for a beta, without `LERD_CHANNEL` for a stable.
- **Docs:** a stable tag (no `-`) deploys lerd.sh by itself; a beta does not.
  Check a page the release changed is live.
- **Issues:** a bug someone else reported was linked with `Refs #N` and stays
  open until a stable release ships it; close it then, with a short comment
  naming the version. Issues we filed closed with their PR.
- Record the lane runs and the release in the lane run log.
