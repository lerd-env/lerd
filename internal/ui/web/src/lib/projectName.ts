// A new project's name becomes its folder, its site name and a DNS label, so it
// is kept to lowercase letters, digits, dots and dashes. The daemon and the CLI
// refuse anything else (siteops.ProjectSlug); this keeps the field from ever
// holding what they would refuse.

export function slugifyProjectName(value: string): string {
  return value
    .toLowerCase()
    .replace(/[^a-z0-9.-]+/g, '-')
    .replace(/-{2,}/g, '-')
    .replace(/^[-.]+/, '');
}

// The trailing dash slugifyProjectName leaves for the next word is not part of
// the name once it is sent.
export function finishProjectName(value: string): string {
  return slugifyProjectName(value).replace(/[-.]+$/, '');
}
