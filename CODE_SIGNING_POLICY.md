# Code signing policy

Free code signing provided by [SignPath.io](https://about.signpath.io), certificate by [SignPath Foundation](https://signpath.org).

## Status

Lerd is in the SignPath Foundation program. Until the production certificate is issued, only release dry runs are signed, with a test certificate, and released Windows artifacts are not signed.

## Scope

| Artifact | Built in | Published in |
| --- | --- | --- |
| `lerd.exe`, the Windows command and login agent (x64 and Arm64) | [lerd-env/lerd](https://github.com/lerd-env/lerd), `cmd/lerd-windows` | The Linux archives in [lerd releases](https://github.com/lerd-env/lerd/releases), which WSL installs use |

The Windows installer, `Install-Lerd.exe`, is built in [lerd-env/lerd-desktop](https://github.com/lerd-env/lerd-desktop) and is not signed yet. It will be added as a separate project.

The tray icon on Windows belongs to Lerd Desktop, so there is no `lerd-tray.exe`. The Lerd Desktop application itself is outside the current scope and will be added as a separate project.

Linux and macOS artifacts are outside the SignPath scope. Since 1.39 the `checksums.txt` of each Lerd release, which lists every archive, is signed with cosign (keyless), and every archive has a build provenance attestation.

Third-party software, such as WSL, Ubuntu and Podman, is not Lerd code and is not signed with the Lerd certificate. The installer does not bundle these components. It downloads them from their own publishers or package repositories during setup.

## Roles

- Project owner, author and approver: George Dumitrescu ([@geodro](https://github.com/geodro))
- Reviewer for Windows artifacts: [@millancore](https://github.com/millancore)

Changes from other contributors are made through pull requests and reviewed before merging. Everyone in these roles must use multi-factor authentication on GitHub and SignPath.

## Release signing process

- Signed releases are built from tagged commits on `main`, on GitHub-hosted runners, by the GitHub Actions workflows in the public repositories listed above.
- Building, submitting for signing and uploading release files are automated. No locally built file is submitted for signing or added to a release.
- Every production signing request is approved by the project owner.

## Privacy

Lerd does not collect telemetry. It has no usage reporting and no install identifier.

Lerd makes these network requests without a separate action by the user. None of them carries usage data; as with any request, the receiving server sees the IP address and the request headers.

- It checks GitHub for a newer Lerd release, normally once every 24 hours, and reads its changelog.
- It refreshes framework and service definitions from GitHub every 6 hours while the watcher is running.
- The dashboard checks container registries for newer images and nodejs.org for newer Node.js releases, and keeps the answers in a local cache.
- It refreshes the plugin catalog every hour while the dashboard service is running, from `plugins.lerd.sh`, which the Lerd project runs.
- The dashboard loads a font stylesheet from Google Fonts.
- If the user enables browser push notifications, notifications are delivered through the browser vendor's push service.

Installing Lerd and adding sites, services, plugins or tools download files from their publishers.

An interactive `lerd uninstall` asks an optional question. If it is answered, the chosen reason, any comment, the Lerd version and the operating system are sent to `feedback.lerd.sh`. Pressing Enter at the question sends nothing. The IP address is used for rate limiting and is not stored. The code is in [`workers/uninstall-feedback`](workers/uninstall-feedback).

Plugins and the applications a user runs in Lerd have their own network behavior.

## Security contact

See [SECURITY.md](SECURITY.md).
