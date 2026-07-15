// inlineBindings resolves a captured query's positional `?` placeholders into a
// runnable statement, escaping each binding as a SQL literal. Placeholders that
// sit inside quoted strings or backtick identifiers are left alone, and any
// question mark past the last binding stays as-is. With no bindings the SQL is
// returned untouched.

function quoteString(s: string): string {
  return "'" + s.replace(/'/g, "''") + "'";
}

function toSqlLiteral(v: unknown): string {
  if (v === null || v === undefined) return 'NULL';
  if (typeof v === 'boolean') return v ? '1' : '0';
  if (typeof v === 'number') return Number.isFinite(v) ? String(v) : quoteString(String(v));
  if (typeof v === 'bigint') return v.toString();
  if (typeof v === 'string') return quoteString(v);
  return quoteString(JSON.stringify(v));
}

export function inlineBindings(sql: string, bindings?: unknown[]): string {
  if (!bindings || bindings.length === 0) return sql;

  let out = '';
  let bindIdx = 0;
  // quote holds the character that opened the current string/identifier, or
  // null when we are in ordinary SQL where a `?` is a real placeholder.
  let quote: string | null = null;

  for (let i = 0; i < sql.length; i++) {
    const ch = sql[i];

    if (quote) {
      out += ch;
      if (ch === '\\' && i + 1 < sql.length) {
        out += sql[i + 1];
        i++;
      } else if (ch === quote) {
        if (sql[i + 1] === quote) {
          out += sql[i + 1];
          i++;
        } else {
          quote = null;
        }
      }
      continue;
    }

    if (ch === "'" || ch === '"' || ch === '`') {
      quote = ch;
      out += ch;
      continue;
    }

    if (ch === '?' && bindIdx < bindings.length) {
      out += toSqlLiteral(bindings[bindIdx++]);
      continue;
    }

    out += ch;
  }

  return out;
}
