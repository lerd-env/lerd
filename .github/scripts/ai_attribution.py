"""Fail when commit messages or a PR description credit an AI tool."""
import re
import sys

# A co-author is judged by its email, since people can share a tool's name.
AI_EMAILS = (r'@(anthropic\.com|openai\.com|cursor\.com|cognition\.ai|aider\.chat|codeium\.com|windsurf\.com)>'
             r'|copilot@users\.noreply\.github\.com|(devin-ai-integration|gemini-code-assist)\[bot\]')
TOOLS = r'(claude|copilot|chatgpt|codex|cursor|gemini|devin|aider|windsurf|codeium)'
PATTERNS = [
    re.compile(r'^\s*co-authored-by:.*(' + AI_EMAILS + ')', re.I),
    re.compile(r'^\W*generated (with|by)\s+\W*' + TOOLS, re.I),
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
