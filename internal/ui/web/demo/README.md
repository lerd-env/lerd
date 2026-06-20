# Backend-free UI demo

Builds the real lerd web UI (`../src`) as a standalone bundle that runs with
**no daemon**, for embedding in the docs landing page (`docs/index.md` iframes
`/demo/`).

How it works:

- `stubs.ts` patches `window.fetch` and `window.WebSocket` before the app
  loads, so every store reads from the JSON in `fixtures/` instead of the
  daemon. It is imported first in `main.ts`.
- `fixtures/` are real `/api/*` responses captured from a running daemon, then
  sanitized (site names, domains, paths and app names swapped for demo values).
  Regenerate the same way: snapshot the endpoints, scrub identifying fields.
- The theme is forced to dark to match the landing page.

Build it:

```
npm run build:demo   # → docs/public/demo/
```

Rebuild whenever the UI components change so the demo stays in sync. The output
is static; the docs site just serves it from `public/demo/`.
