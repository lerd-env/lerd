# Extending the Plugin System

A [plugin](../features/plugins.md) can only use what lerd offers it: the surfaces in `plugin.yaml`, the events it hooks and the jobs it can take charge of. When an integration needs something lerd does not offer yet, add it to lerd rather than working around it. A plugin that edits lerd's configuration files or reaches into its containers directly is not approved for the catalog.

Open an issue first describing the plugin and the gap, so the shape of the addition is agreed before it is built. Everything added here is for every plugin: lerd's Go code never names a particular plugin, the way it never names a framework.

## Adding an event

An event tells plugins that something happened, after it succeeded. Plugins cannot stop or change it.

1. Add the constant and its entry in the `events` map in `internal/plugins/events.go`. Name it `<subject>.<past tense>`, like `site.linked`.
2. Call `plugins.Emit` once, from the shared operation the CLI, dashboard, TUI and MCP all go through, never from each surface. If the surfaces do not share one yet, that is the first change to make.
3. Keep the payload flat strings. Reuse `plugins.SitePayload` for a site and add only what a hook could not look up itself.
4. Add the event and its payload to the events table in `docs/features/plugins.md`, and a test that the operation fires it once.

## Adding a job a plugin can take charge of

A job is something lerd does that a plugin can do instead, declared under `provides:`. Add one only when a real plugin needs it, since every job is a promise lerd keeps for every release after it.

1. Add the constant and its entry in the `jobs` map in `internal/plugins/provides.go`.
2. Find the one function every path to that job goes through and hand off there with `plugins.ProviderOf`, the way `SetWorktreeDBIsolated` does for `worktree_database`. A job reached from several places without a shared function is not ready to hand off.
3. Define the request and the answer as a contract. The request goes in as JSON on stdin with the action as the first argument. The answer is decoded strictly, refusing any field lerd does not know, so a plugin learns at once that lerd ignored something it relied on.
4. Record which plugin did the job wherever lerd needs to undo it later, so the undo goes back through the same plugin even after another plugin is installed.
5. When the plugin fails, report what it wrote to stderr and leave things as they were. Never fall back to lerd's own way silently: the plugin is there because lerd's own way was wrong for that user.
6. Document the job in `docs/features/plugins.md` under *Taking over a job from lerd*, with a table of its request fields and the answer it expects.

## Compatibility

Plugins reach users through the catalog with no lerd release in between, so a new event or job has to be safe for older binaries too. An older lerd refuses a plugin that hooks an event or provides a job it does not know, naming it, which is the right outcome. A plugin that needs the addition declares `lerd: ">=X.Y"` with the release that shipped it, so the catalog never offers it to a lerd that would refuse it.

Change `APIVersion` only when an existing contract breaks: a field removed, a payload retyped, an event that stops firing. Adding never breaks one.
