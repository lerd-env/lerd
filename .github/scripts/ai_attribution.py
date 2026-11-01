"""Fail when commit messages or a PR description credit an AI tool."""
import re
import sys

TOOLS = r'(claude|anthropic|copilot|openai|chatgpt|codex|cursor|gemini|devin|aider|windsurf|codeium)'
# Only trailers and footers count, so prose that names a tool still passes.
PATTERNS = [
    re.compile(r'^\s*co-authored-by:.*' + TOOLS, re.I),
    re.compile(r'generated (with|by)\b.*' + TOOLS, re.I),
]


def offending(text):
    return [line.strip() for line in text.splitlines() if any(p.search(line) for p in PATTERNS)]


if __name__ == '__main__':
    found = offending(sys.stdin.read())
    for line in found:
        print(f'::error::AI attribution: {line}')
    if found:
        print('Remove AI co-author trailers and "Generated with" footers from the commits and the PR description.')
    sys.exit(1 if found else 0)
