# Logs

lerd gives AI assistants a single, filtered view over every log it can reach, so debugging a broken site doesn't mean opening files by hand. It is exposed as the `logs` MCP tool with two actions: `sources` and `fetch`.

## Sources

Call `logs` with `action: "sources"` to enumerate what you can query for the current site plus the shared infrastructure. Source names:

| Name | Backend | What it is |
|---|---|---|
| `app:<file>` | file | Framework application logs (e.g. `app:laravel.log` from `storage/logs/*.log`), parsed with real timestamps and levels |
| `fpm` | container | The site's PHP-FPM container stdout |
| `worker:<name>` | journal | A declared worker unit: `worker:queue`, `worker:horizon`, `worker:schedule`, custom workers |
| `nginx` | container | nginx access/error output |
| `dns` | container | dnsmasq |
| `watcher`, `ui` | journal | The lerd file watcher and UI server |
| `<service>` | container | A default service (`mysql`, `redis`, `mailpit`, …) |
| `php<ver>` | container | An installed PHP-FPM container, for site-less sessions |

Sources are listed whether or not they are currently running; a source that isn't up simply returns no lines at fetch time.

## Fetch

Call `logs` with `action: "fetch"`, a `source` name, and any of these filters:

- `grep`: keep only lines matching this pattern. It is compiled as a regular expression and falls back to a literal substring match when the pattern isn't valid regex.
- `since` / `until`, a time window. Accepts relative durations (`15m`, `1h`, `2h30m`), absolute timestamps (`2026-06-11T10:00:00Z` or `2026-06-11 10:00:00`), or a `cursor` returned by a previous fetch.
- `level`: application logs only: filter by `error`, `warning`, `info`, `debug`, etc.
- `lines`: maximum number of lines to return (default 50).

Entries come back in chronological order, oldest first. Container-stdout and journal sources push the time window down to `podman logs --since/--until` and `journalctl --since/--until -g` respectively; application-log files are filtered in process.

## Streaming via cursor

MCP is a request/response protocol, so there is no live follow. Instead every `fetch` returns an opaque `cursor` marking the newest line returned. To watch a log evolve, call `fetch` again with `since` (or `cursor`) set to that value and you receive only the lines that arrived since. The cursor format differs per backend (a journal cursor, an RFC3339 timestamp, or a log entry's own timestamp), so treat it as opaque and just echo it back.

## Notes and limitations

- Raw logs without timestamps (most non-monolog files) ignore `since`/`until`/`level` and simply return the last N lines; their cursor is empty.
- A container or unit that isn't running returns whatever partial output exists rather than an error.
- On macOS, worker/watcher/ui sources read from `~/Library/Logs/lerd/<unit>.log` (or `podman logs` for container-mode workers) since there is no systemd journal; on Linux they read the user journal.
- Large files are read from a 512 KB tail; when that cap is hit the result is flagged as truncated.
