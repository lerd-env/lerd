import { describe, it, expect } from 'vitest';
import { highlight } from './highlight';

describe('highlight', () => {
  it('colours a query and escapes everything it leaves alone', () => {
    expect(highlight("select * from `users` where id = ? and name = 'a<b' -- c", 'sql')).toBe(
      '<span class="hl-kw">select</span> * <span class="hl-kw">from</span> <span class="hl-id">`users`</span> <span class="hl-kw">where</span> id = <span class="hl-var">?</span> <span class="hl-kw">and</span> name = <span class="hl-str">\'a&lt;b\'</span> <span class="hl-com">-- c</span>'
    );
  });

  it('tells SQL functions from keywords', () => {
    expect(highlight('count(*), lower(name)', 'sql')).toBe('<span class="hl-kw">count</span>(*), <span class="hl-fn">lower</span>(name)');
  });

  it('colours PHP variables, calls, classes and keywords', () => {
    expect(highlight('return $next($request)->header(\'X-Demo\', 1); // done', 'php')).toBe(
      '<span class="hl-kw">return</span> <span class="hl-var">$next</span>(<span class="hl-var">$request</span>)-&gt;<span class="hl-fn">header</span>(<span class="hl-str">\'X-Demo\'</span>, <span class="hl-num">1</span>); <span class="hl-com">// done</span>'
    );
    expect(highlight('new Cache\\Repository', 'php')).toBe('<span class="hl-kw">new</span> <span class="hl-cls">Cache\\Repository</span>');
  });

  it('leaves a string that runs to the end of the line coloured', () => {
    expect(highlight("$x = 'open", 'php')).toBe('<span class="hl-var">$x</span> = <span class="hl-str">\'open</span>');
  });

  it('colours a GraphQL operation', () => {
    expect(highlight('mutation Login($e: String!) { login(email: $e) @auth { token } } # x', 'graphql')).toBe(
      '<span class="hl-kw">mutation</span> <span class="hl-fn">Login</span>(<span class="hl-var">$e</span>: <span class="hl-cls">String</span>!) { <span class="hl-fn">login</span>(email: <span class="hl-var">$e</span>) <span class="hl-kw">@auth</span> { token } } <span class="hl-com"># x</span>'
    );
  });
});
