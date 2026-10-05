// A small highlighter for the SQL, PHP and GraphQL the Debug window shows: keywords,
// strings, numbers, comments, variables and placeholders, as escaped HTML with
// hl-* classes app.css colours. It reads one line or statement at a time, so a
// string or comment that spans lines is coloured per line, which is enough for
// a query or a few lines of code around a frame.

export type HighlightLang = 'sql' | 'php' | 'graphql';

const SQL_KEYWORDS = new Set(
  'select from where and or not in is null as on join left right inner outer cross full using group by order having limit offset insert into values update set delete returning distinct union all exists between like ilike case when then else end asc desc create table alter drop index primary key foreign references default with recursive over partition window true false if replace ignore duplicate lock for share nowait skip locked count sum min max avg coalesce cast interval'.split(' ')
);

const PHP_KEYWORDS = new Set(
  'abstract and array as break callable case catch class clone const continue declare default do echo else elseif empty enddeclare endfor endforeach endif endswitch endwhile enum extends final finally fn for foreach function global goto if implements include include_once instanceof insteadof interface isset list match namespace new or print private protected public readonly require require_once return static switch throw trait try unset use var while xor yield from true false null self parent mixed void never int float string bool iterable object'.split(' ')
);

const GRAPHQL_KEYWORDS = new Set('query mutation subscription fragment on true false null schema type input enum interface union scalar extend implements directive repeatable'.split(' '));

const TOKENS: Record<HighlightLang, RegExp> = {
  graphql: /(#[^\n]*)|("(?:[^"\\]|\\.)*"?)|(\$[A-Za-z_]\w*)|(-?\b\d+(?:\.\d+)?\b)|(@[A-Za-z_]\w*)|([A-Za-z_]\w*)(?=\s*\()|([A-Za-z_]\w*)/g,
  sql: /(--[^\n]*|\/\*[\s\S]*?(?:\*\/|$))|('(?:[^'\\]|\\.|'')*'?)|("(?:[^"\\]|\\.)*"?|`[^`]*`?)|(\b\d+(?:\.\d+)?\b)|(\?|:[A-Za-z_]\w*)|([A-Za-z_]\w*)(?=\s*\()|([A-Za-z_]\w*)/g,
  php: /(\/\/[^\n]*|#(?!\[)[^\n]*|\/\*[\s\S]*?(?:\*\/|$))|('(?:[^'\\]|\\.)*'?|"(?:[^"\\]|\\.)*"?)|(\$[A-Za-z_]\w*)|(\b\d+(?:\.\d+)?\b)|(#\[)|([A-Za-z_]\w*)(?=\s*\()|([A-Za-z_\\][\w\\]*)/g
};

const esc = (s: string) => s.replace(/[&<>"]/g, (c) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;' })[c]!);
const span = (cls: string, s: string) => `<span class="hl-${cls}">${esc(s)}</span>`;

export function highlight(code: string, lang: HighlightLang): string {
  const re = new RegExp(TOKENS[lang]);
  let out = '';
  let at = 0;
  for (let m = re.exec(code); m; m = re.exec(code)) {
    out += esc(code.slice(at, m.index));
    at = m.index + m[0].length;
    const [all, comment, str, third, fourth, fifth, call, word] = m;
    if (comment) out += span('com', all);
    else if (str) out += span('str', all);
    else if (lang === 'graphql') {
      if (third) out += span('var', all);
      else if (fourth) out += span('num', all);
      else if (fifth) out += span('kw', all);
      else if (call) out += span('fn', all);
      else if (GRAPHQL_KEYWORDS.has(word)) out += span('kw', all);
      else out += /^[A-Z]/.test(word) ? span('cls', all) : esc(all);
    } else if (lang === 'sql') {
      if (third) out += span('id', all);
      else if (fourth) out += span('num', all);
      else if (fifth) out += span('var', all);
      else if (call) out += span(SQL_KEYWORDS.has(call.toLowerCase()) ? 'kw' : 'fn', all);
      else out += SQL_KEYWORDS.has(word.toLowerCase()) ? span('kw', all) : esc(all);
    } else {
      if (third) out += span('var', all);
      else if (fourth) out += span('num', all);
      else if (fifth) out += span('com', all);
      else if (call) out += span(PHP_KEYWORDS.has(call.toLowerCase()) ? 'kw' : 'fn', all);
      else if (PHP_KEYWORDS.has(word.toLowerCase())) out += span('kw', all);
      else out += /^\\?[A-Z]/.test(word) ? span('cls', all) : esc(all);
    }
  }
  return out + esc(code.slice(at));
}
