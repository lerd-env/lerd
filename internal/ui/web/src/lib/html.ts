// Paraglide compiles a message to a plain template literal, so a value
// interpolated into one reaches {@html} unescaped. Any dynamic value spliced
// into a message rendered that way has to come through here first.
export function escapeHtml(value: unknown): string {
  if (value === null || value === undefined) return '';
  return String(value)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;')
    .replace(/'/g, '&#39;');
}
