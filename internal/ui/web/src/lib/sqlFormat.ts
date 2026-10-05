// formatSql lays a query out the way a person would write it: each clause on
// its own line, the columns of a select one per line, and AND, OR and joins
// indented under the clause they belong to, a subquery a level deeper. It
// reads strings, quoted names and comments as they are, so nothing in them
// moves.

const CLAUSES = ['select', 'from', 'where', 'group by', 'order by', 'having', 'limit', 'offset', 'values', 'set', 'returning', 'union all', 'union', 'insert into', 'update', 'delete from', 'on duplicate key update'];
const JOINS = ['left outer join', 'right outer join', 'full outer join', 'left join', 'right join', 'inner join', 'cross join', 'full join', 'join'];

const TOKEN = /('(?:[^'\\]|\\.|'')*'?|"(?:[^"\\]|\\.)*"?|`[^`]*`?|--[^\n]*|\/\*[\s\S]*?(?:\*\/|$))|(\s+)|([(),])|([^\s'"`(),]+)/g;

export function formatSql(sql: string): string {
  // Each token with whether the query had space before it, which the layout
  // keeps wherever it does not break the line itself.
  const tokens: string[] = [];
  const spaced: boolean[] = [];
  let gap = false;
  for (const m of sql.matchAll(TOKEN)) {
    if (m[2] !== undefined) {
      gap = true;
      continue;
    }
    tokens.push(m[0]);
    spaced.push(gap);
    gap = false;
  }
  const lower = tokens.map((t) => t.toLowerCase());
  // phrase finds a multi-word clause starting at i, as one of the given ones.
  const phrase = (i: number, list: string[]) => list.find((p) => p.split(' ').every((w, k) => lower[i + k] === w));
  const lines: string[] = [];
  let line = '';
  let depth = 0;
  const inSelect: boolean[] = [false];
  const parens: boolean[] = [];
  const pad = (extra = 0) => '  '.repeat(depth + extra);
  const newline = (extra = 0) => {
    if (line.trim()) lines.push(line.replace(/\s+$/, ''));
    line = pad(extra);
  };
  let at = 0;
  const add = (t: string) => {
    line += (line.trim() === '' || !spaced[at] ? '' : ' ') + t;
  };
  // manyColumns reports whether the select list starting at i has a comma of
  // its own before its from.
  const manyColumns = (i: number) => {
    for (let level = 0; i < tokens.length; i++) {
      if (tokens[i] === '(') level++;
      else if (tokens[i] === ')') {
        if (--level < 0) return false;
      } else if (level === 0 && tokens[i] === ',') return true;
      else if (level === 0 && lower[i] === 'from') return false;
    }
    return false;
  };
  for (let i = 0; i < tokens.length; i++) {
    const t = tokens[i];
    at = i;
    const clause = phrase(i, CLAUSES);
    const join = clause ? undefined : phrase(i, JOINS);
    if (clause || join) {
      const words = (clause ?? join)!.split(' ').length;
      newline(join ? 1 : 0);
      add(tokens.slice(i, i + words).join(' '));
      inSelect[depth] = clause === 'select';
      // A select of several columns lists them one per line; one stays put.
      if (clause === 'select' && manyColumns(i + 1)) newline(1);
      i += words - 1;
      continue;
    }
    if ((lower[i] === 'and' || lower[i] === 'or') && !inSelect[depth]) {
      newline(1);
      add(t);
      continue;
    }
    // Parentheses nest; only those opening a subquery move the layout.
    if (t === '(') {
      const sub = !!phrase(i + 1, ['select']);
      parens.push(sub);
      add('(');
      if (sub) {
        depth++;
        inSelect[depth] = false;
      }
      continue;
    }
    if (t === ')' && parens.pop()) {
      depth--;
      newline();
      add(')');
      continue;
    }
    if (t === ',' && inSelect[depth]) {
      add(',');
      newline(1);
      continue;
    }
    add(t);
  }
  newline();
  return lines.join('\n');
}
