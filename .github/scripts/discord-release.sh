#!/usr/bin/env bash
set -euo pipefail

# Announces a release on Discord. The description is the first paragraph of the
# version's changelog section, which is the prose summary written for humans,
# unlike the GitHub release body that opens with install boilerplate.

TAG="${1:?usage: discord-release.sh <tag>}"
VERSION="${TAG#v}"
CHANGELOG="${CHANGELOG_FILE:-docs/changelog.md}"
REPO="${GITHUB_REPOSITORY:-lerd-env/lerd}"

summary=$(awk -v heading="## [$VERSION]" '
  index($0, heading) == 1 { found = 1; next }
  found && (/^## / || /^### /) { exit }
  found && NF { print; exit }
' "$CHANGELOG")

if [ -z "$summary" ]; then
  echo "no changelog section for $VERSION in $CHANGELOG" >&2
  exit 1
fi

if [ "${#summary}" -gt 300 ]; then
  summary="${summary:0:300}…"
fi

case "$VERSION" in
  *-*)
    title="🧪 lerd $TAG · pre-release"
    colour=13801762
    update="lerd update --beta"
    ;;
  *)
    title="🚀 lerd $TAG"
    colour=3055171
    update="lerd update"
    ;;
esac

url="https://github.com/$REPO/releases/tag/$TAG"

payload=$(jq -n \
  --arg title "$title" \
  --arg url "$url" \
  --arg summary "$summary" \
  --arg update "$update" \
  --argjson colour "$colour" \
  '{
    embeds: [{
      title: $title,
      url: $url,
      description: $summary,
      color: $colour,
      fields: [
        { name: "New install", value: "```\ncurl -fsSL https://lerd.sh/install.sh | bash\n```" },
        { name: "Already have lerd", value: ("```\n" + $update + "\n```") },
        { name: "Notes", value: ("[" + $url + "](" + $url + ")") }
      ],
      footer: { text: "lerd.sh" },
      timestamp: (now | todate)
    }]
  }')

echo "$payload"

if [ -n "${DISCORD_WEBHOOK:-}" ]; then
  curl -fsS -X POST -H "Content-Type: application/json" -d "$payload" "$DISCORD_WEBHOOK" >/dev/null
fi
