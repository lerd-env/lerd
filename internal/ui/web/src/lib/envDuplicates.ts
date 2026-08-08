// A dotenv file setting a key twice means different things to different
// runtimes: Symfony's dotenv keeps the last assignment, Laravel's phpdotenv
// keeps the first, and lerd reads the first. Nothing can be right for all of
// them, so the editor shows the conflict and the user says which value they
// meant.

export interface EnvDuplicate {
  key: string;
  /** Every live occurrence, in file order. */
  occurrences: { line: number; value: string }[];
}

/** Keys the buffer sets more than once, ignoring commented-out lines. */
export function findDuplicates(text: string): EnvDuplicate[] {
  const seen = new Map<string, { line: number; value: string }[]>();
  text.split("\n").forEach((raw, i) => {
    const line = raw.trim();
    if (!line || line.startsWith("#")) return;
    const eq = line.indexOf("=");
    if (eq <= 0) return;
    const key = line.slice(0, eq).trim();
    if (!key) return;
    const value = line
      .slice(eq + 1)
      .trim()
      .replace(/^["']|["']$/g, "");
    const at = seen.get(key) ?? [];
    at.push({ line: i, value });
    seen.set(key, at);
  });
  return [...seen.entries()]
    .filter(([, occ]) => occ.length > 1)
    .map(([key, occurrences]) => ({ key, occurrences }))
    .sort((a, b) => a.key.localeCompare(b.key));
}

/**
 * Drops every occurrence of key except the one on keepLine, leaving the rest of
 * the file untouched so the result is an ordinary unsaved edit the user reviews
 * and saves like any other.
 */
export function keepOnly(text: string, key: string, keepLine: number): string {
  const lines = text.split("\n");
  const out = lines.filter((raw, i) => {
    if (i === keepLine) return true;
    const line = raw.trim();
    if (!line || line.startsWith("#")) return true;
    const eq = line.indexOf("=");
    if (eq <= 0) return true;
    return line.slice(0, eq).trim() !== key;
  });
  return out.join("\n");
}
