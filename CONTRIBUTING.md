# Contributing to Lerd

Thank you for your interest in contributing to **Lerd**! We welcome contributions from everyone, whether you are fixing a bug, improving documentation, or adding new features.

This document provides a set of guidelines and best practices to help you get started with your contributions.

---

## Table of Contents

- [Code of Conduct](#code-of-conduct)
- [How Can I Contribute?](#how-can-i-contribute)
	- [Reporting Bugs](#reporting-bugs)
	- [Suggesting Enhancements](#suggesting-enhancements)
	- [Pull Requests](#pull-requests)
- [Development Setup](#development-setup)
- [Branch Naming Conventions](#branch-naming-conventions)
- [Commit Message Guidelines](#commit-message-guidelines)
- [Testing](#testing)

---

## Code of Conduct

Please help us keep this project open, welcoming, and inclusive. Be respectful and constructive in all discussions, issues, and pull requests.

---

## How Can I Contribute?

### Reporting Bugs
If you find a bug or an unexpected error, please open a new issue on GitHub with:
1. A clear and descriptive title.
2. Steps to reproduce the bug.
3. Expected vs. actual behavior.
4. Your operating system and Go environment details (`go version`).

### Suggesting Enhancements
Feature requests are always welcome! When proposing a new feature:
- Explain **why** this feature would be useful.
- Describe how it should work or interact with existing packages.

---

## Development Setup

Follow these steps to set up your local development environment:

1. **Fork the Repository:**
	 Click the **Fork** button on top right of the GitHub repository page.

2. **Clone your Fork:**
	 ```bash
	 git clone https://github.com/YOUR-USERNAME/lerd.git
	 cd lerd
	 ```

3. **Add Remote Upstream:**
	 Keep your fork synchronized with the main repository:
	 ```bash
	 git remote add upstream https://github.com/lerd-env/lerd.git
	 ```

4. **Download Go Dependencies:**
	 ```bash
	 go mod download
	 ```

---

## Branch Naming Conventions

Always create a new branch from `main` before starting any work. Do not commit directly to the `main` branch.

Use the following prefixes when naming your branch:
* `feat/` – For new features or additions (e.g., `feat/add-logger-middleware`)
* `fix/` – For bug fixes or patches (e.g., `fix/context-timeout-leak`)
* `docs/` – For documentation improvements (e.g., `docs/add-contributing-guide`)
* `refactor/` – For code restructuring without changing functionality (e.g., `refactor/store-layer`)
* `test/` – For adding or updating unit tests (e.g., `test/config-parser`)

**Example Command:**
```bash
git checkout -b docs/add-contributing-guide
```

---

## Commit Message Guidelines

We follow conventional commit format to keep history clean and readable:

- `feat:` for new features
- `fix:` for bug fixes
- `docs:` for documentation updates
- `test:` for unit tests
- `refactor:` for code cleanups

**Examples:**
```text
docs: add CONTRIBUTING.md guide for new contributors
fix(config): correct environment variable parsing logic
test(store): add missing unit test for key-value store
```

---

## Testing

Before submitting a Pull Request, ensure that all unit tests pass without any failure:

Run all tests across the project:
```bash
go test ./...
```

Run tests for a specific package:
```bash
go test ./internal/config/...
```

---

## Submitting a Pull Request (PR)

1. Keep your branch updated with `upstream/main`:
	 ```bash
	 git checkout main
	 git pull upstream main
	 git checkout your-branch-name
	 git rebase main
	 ```
2. Push your branch to your GitHub fork:
	 ```bash
	 git push origin your-branch-name
	 ```
3. Open GitHub and click **"Compare & pull request"**.
4. Fill out the PR template/description clearly explaining what changes were made.
5. Wait for maintainers to review your PR!

---


Thanks for taking the time to join our community and start contributing!