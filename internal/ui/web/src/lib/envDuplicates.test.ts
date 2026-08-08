import { describe, it, expect } from "vitest";
import { findDuplicates, keepOnly } from "./envDuplicates";

describe("env duplicates", () => {
  const file = [
    "# DATABASE_URL=mysql://commented",
    "DATABASE_URL=postgresql://postgres@lerd-postgres:5432/app",
    "APP_ENV=dev",
    "",
    "# lerd debug test",
    'DATABASE_URL="sqlite:///var/app.db"',
  ].join("\n");

  it("reports a key set twice, ignoring a commented value", () => {
    const dupes = findDuplicates(file);
    expect(dupes).toHaveLength(1);
    expect(dupes[0].key).toBe("DATABASE_URL");
    expect(dupes[0].occurrences.map((o) => o.value)).toEqual([
      "postgresql://postgres@lerd-postgres:5432/app",
      "sqlite:///var/app.db",
    ]);
  });

  it("says nothing about a file that sets each key once", () => {
    expect(findDuplicates("APP_ENV=dev\nDATABASE_URL=x\n")).toEqual([]);
  });

  it("keeps the chosen occurrence and drops the other, leaving the rest alone", () => {
    const dupes = findDuplicates(file);
    const keepLast = dupes[0].occurrences[1].line;
    const out = keepOnly(file, "DATABASE_URL", keepLast);

    expect(out).toContain('DATABASE_URL="sqlite:///var/app.db"');
    expect(out).not.toContain("postgresql://");
    // everything else survives, comments included
    expect(out).toContain("APP_ENV=dev");
    expect(out).toContain("# DATABASE_URL=mysql://commented");
    expect(out).toContain("# lerd debug test");
    expect(findDuplicates(out)).toEqual([]);
  });

  it("can keep the first instead", () => {
    const dupes = findDuplicates(file);
    const out = keepOnly(file, "DATABASE_URL", dupes[0].occurrences[0].line);
    expect(out).toContain("postgresql://");
    expect(out).not.toContain("sqlite:///var/app.db");
  });
});
