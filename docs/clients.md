# Clients: Claude Code

Point a coding tool at ccw and it needs nothing else. ccw does the
conversion between shapes, so there is no local proxy to run next to the tool.
Any tool that writes the tool's own config can set this up; the steps below use
[CC Switch](https://github.com/farion1231/cc-switch), which does.

Make an API key first: **Endpoint → API keys** in the dashboard. Name a model
`<provider>/<model>` (see [Routing](routing.md)); `GET /v1/models` lists them.

## Claude Code

In CC Switch add a **Custom** provider for Claude, with the upstream format
**Anthropic Messages (native)**. It writes these into `~/.claude/settings.json`:

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "https://<your ccw host>",
    "ANTHROPIC_AUTH_TOKEN": "<the ccw API key>",
    "ANTHROPIC_MODEL": "antigravity/claude-sonnet-4-6",
    "ANTHROPIC_DEFAULT_OPUS_MODEL": "antigravity/claude-opus-4-6-thinking",
    "ANTHROPIC_DEFAULT_SONNET_MODEL": "antigravity/claude-sonnet-4-6",
    "ANTHROPIC_DEFAULT_HAIKU_MODEL": "antigravity/gemini-3.7-flash"
  }
}
```

- The base URL has **no `/v1`**: Claude Code adds `/v1/messages` itself.
- Set all three tiers. Claude Code sends a bare `claude-*` id for its background
  work when a tier is not set, and ccw answers `404 model_not_found` for it.
- A `[1m]` suffix on a model id is accepted and removed: the provider gets the
  model's own name. It does not change the window Claude Code assumes; see below.
- **Context window.** For an id that does not start with `claude-`, Claude Code
  assumes 200k and cannot read the window from `/v1/models`. Pin it for a model
  with a larger one, with the window it really has:

  ```json
  "CLAUDE_CODE_MAX_CONTEXT_TOKENS": "1000000",
  "CLAUDE_CODE_AUTO_COMPACT_WINDOW": "900000"
  ```

  Without them Claude Code compacts a 1M model at 200k.

## What was tried

Run against a live ccw, with the config above in a scratch
`CLAUDE_CONFIG_DIR`, and the models `antigravity/claude-opus-4-6-thinking`,
`claude-sonnet-4-6` and `gemini-3.7-flash`:

- Claude Code: a three-turn task with two tool calls and thinking on.
- The pinned 1M window in Claude Code (`modelUsage.contextWindow` read 1000000).

Not tried here: the CC Switch window itself (its form labels) was not driven,
so those steps come from its source and changelog.

## Web search and web fetch

**WebFetch needs nothing from ccw.** Claude Code downloads the page itself, then
asks its small model to answer from it: an ordinary call with no tools, sent to the
Haiku tier. That is why all three tiers must be set (`ANTHROPIC_DEFAULT_HAIKU_MODEL`
above): left unset, it goes out under a `claude-*` name that ccw does not serve.

**WebSearch is different.** It does not search either: it sends a request that
declares Anthropic's hosted tool (`web_search_20250305`) and lets Anthropic's
servers run the search. No other provider has that tool, so a model behind ccw
used to answer from memory and name sources it never read, and Claude Code
showed them as results.

ccw runs the search itself, the way claude-code-router does:

1. It finds the hosted tool in the request, and reads the query from the last user
   message ("Perform a web search for the query: …").
2. It calls a search service, and adds the results to the system prompt of the
   request, without the hosted tool, for any model to answer.
3. It puts the blocks Anthropic would have written at the head of the answer: a
   `server_tool_use` for the call, a `web_search_tool_result` with the results,
   and `usage.server_tool_use.web_search_requests`. Claude Code reads its
   sources from there.

**Nothing to set up if you have an Antigravity account.** With no search service
configured, ccw searches through Google: it asks a Gemini model behind the
account with Google Search grounding (`gemini-2.5-flash`, or `CCW_SEARCH_MODEL`),
and lists the sources the answer was grounded in, with the statements each was
cited for. Google's redirect links are followed, so the client lists the real
pages. It costs one call of a fast model per search, from the account's quota.

To use a search service instead, set `CCW_SEARCH_PROVIDER` to `brave`, `tavily`,
`serper` or `exa` and `CCW_SEARCH_KEY` to its key (`antigravity` names the
default explicitly). With none available, or when the service refuses, the tool is
answered `unavailable` and the model is told to say the search failed, not to
invent results. A request that goes to an Anthropic account is left alone: it runs
the search itself.

Not covered: free search that needs no account was tried and does not hold up:
DuckDuckGo answers a script with a verification page, Jina wants a key, and of 24
public SearXNG instances one returned JSON.

Tried with `antigravity/claude-sonnet-4-6` and `opencode/big-pickle`: the latest Go
release was asked for and both answered with the release and pages that were
really searched (`go.dev/doc/go1.27`). A stand-in Brave service was used for the
key-based path.

## Other tools Anthropic runs or defines

A request to Anthropic can declare tools that Claude was trained on. For a model
that is not Claude's, ccw supplies what Anthropic keeps on its side. It applies
to the Messages API (`/v1/messages`) when no Anthropic account serves the request.
Claude Code does not send these, so they matter to other clients and SDKs.

**Client tools declared by type** (`bash_…`, `text_editor_…`, `memory_…`). The
request names the tool and gives no schema. ccw adds the standard one, with the
command list of that version (`undo_edit` belongs to the January 2025 editor
only), and the client still runs the tool.

**`web_fetch_…`.** The model asks for a URL and ccw fetches it. The rules are
Anthropic's: the URL must already appear in a user message, a client tool result,
or an earlier fetch or search result; `max_uses`, `allowed_domains`,
`blocked_domains` and `max_content_tokens` hold; the answer carries the
`server_tool_use` and `web_fetch_tool_result` blocks and
`usage.server_tool_use.web_fetch_requests`. A URL a web search found counts too, when both tools are declared. Because the
model picks the URL, a
name that resolves to this machine or a private network is refused
(`url_not_allowed`), at the connection and at each redirect. Pages are read as
text; a PDF answers `unsupported_content_type`, and a page that needs JavaScript
is read as served.

**`tool_search_tool_regex_…` and `tool_search_tool_bm25_…`.** Tools marked
`defer_loading` stay out of the request until the model searches for them. The
regex variant takes RE2 syntax, which differs from Python's only in look-around
and back-references; both cap the pattern (200) or query (500) as Anthropic does.
Tools a search found are offered on the next call, and again on later requests
because ccw reads the earlier `tool_search_tool_result` blocks in the history.

**`mcp_servers` with an `mcp_toolset`.** ccw connects to each server over
Streamable HTTP (a JSON body or an event stream), lists its tools, offers them
to the model, and calls them. `authorization_token` goes as a bearer token;
`enabled` and `defer_loading` apply per set and per tool. Only tools are
supported, as in Anthropic's connector. The old HTTP+SSE transport is not. A
tool whose name clashes with a client tool is offered as `<server>__<tool>`. A
server that cannot be reached answers the request with a 400. The server's URL
comes from the client and is not checked: unlike Anthropic's connector, ccw
accepts `http` and a private address, because a server on this network is the
usual case. Any holder of an ccw key can make this machine connect to one.

For these three, ccw calls the model itself, runs the calls that are its own,
and calls the model again, up to 10 times; then it answers `pause_turn`. If the
model asks for a client tool in the same turn, the answer comes back at once with
the blocks ccw produced and `stop_reason: tool_use`. The model is called
without a stream, so a client that asked for a stream gets the same events in
order, but all at once when the work is done. Usage adds up over the calls.

Still not covered, and dropped with one line in the log per tool: `advisor`,
`code_execution`, `computer_…`, the Files API, Batches, and the Responses hosted
tools (`web_search`, `file_search`, `image_generation`, `code_interpreter`).

Tried with the official `anthropic` Python SDK against `opencode/big-pickle`,
`antigravity/claude-sonnet-4-6` and `antigravity/gemini-3-flash`: an
MCP tool call (a small MCP server on this machine), with and without a stream; a
tool search, then a second request that sent the first answer back; and a fetch
of `example.com`; and, on the Gemini model, a web search then a fetch of a page it
found. The SDK parsed every block. A fetch of `127.0.0.1` and of
`169.254.169.254` was refused.

## OpenCode Zen through Claude Code

The `opencode` provider needs no key; see [Providers](providers.md#notes-per-provider).
Tried with `opencode/big-pickle` and the muse model on a live ccw:

- Claude Code: two turns with a tool call and `--resume`, one upstream session for
  the whole conversation (`GET /api/zen/sessions`).

## What ccw repairs on the way

A tool can send a request that a provider refuses. ccw repairs these so the
tool does not have to:

| Case | What ccw does |
| --- | --- |
| A tool call with no result, or a result with no call, in the history | Adds a placeholder result, or turns the stray result into text. Claude Code trims and compacts history, which leaves both. |
| A thinking block with an empty signature sent to an Anthropic upstream | Drops it. Anthropic verifies signatures. |
| Thinking on, in a tool loop whose last assistant turn has no thinking block | Runs that one request without thinking, for an Anthropic upstream. |
| A tool schema with `$schema`, a `uri` format, or no type | Cleans it for a strict chat provider. |
| A reply that puts its reasoning in the content, between `<think>` tags (DeepSeek, Qwen, GLM) | Sends it as a `thinking` block, or drops it when the client did not ask for thinking. |
| `?beta=true`, which Claude Code appends to `/v1/messages` | Keeps it off a request that is translated to another provider's endpoint. |
| A `count_tokens` call for a provider that cannot count | Estimates, with a fixed cost per image instead of counting the base64. |
| A request body compressed with gzip or deflate | Reads it. Other encodings get `415`. A body over 50 MB, or that inflates past it, gets `413`. |

Claude behind Antigravity reasons only when it is asked with a token budget, so
ccw sends the effort the tool asked for as `thinkingBudget` with
`includeThoughts`.
