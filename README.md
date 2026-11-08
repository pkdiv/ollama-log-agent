# ollama-log-agent

A minimal log-analysis agent in Go. A local LLM (`qwen3:8b` via Ollama) answers questions about a log file by calling a regex `search_logs` tool in a tool-calling loop. Standard library only, no agent frameworks.

Built to understand the agent loop from first principles: what the API returns, how tool calls round-trip, and where small models break.

## How it works

```
user question ──► chat(messages + tools) ──► response
                        ▲                        │
                        │             tool_calls empty?
                        │               │yes            │no
                        │               ▼               ▼
                        │          print content   run each tool
                        │            (done)        append results
                        └────────────────────────────────┘
```

Each iteration:

1. POST `messages` and the tool schema to Ollama's `/api/chat` (non-streaming).
2. Append the assistant message to `messages` unchanged. The API is stateless, so history is replayed on every call.
3. If `message.tool_calls` is empty, `message.content` is the final answer. Print it and exit.
4. Otherwise execute each call and append one `role: "tool"` message per call (with `tool_name` set), then loop.

The loop is capped at 10 rounds.

## Tool

`search_logs(pattern, limit)`

| Arg | Type | Required | Description |
|---|---|---|---|
| `pattern` | string | yes | Go `regexp` matched against each line |
| `limit` | integer | no | Max lines returned (clamped to 1–50, default 50) |

Returns matching lines as `<lineno>: <line>`, or `no matches`. Errors (bad regex, missing file) are returned as tool output, not Go errors, so the model can read the failure and retry.

## Requirements

- Go 1.21+
- [Ollama](https://ollama.com) running on `localhost:11434`
- The model: `ollama pull qwen3:8b`

## Run

```sh
cp sample.log app.log   # or point it at your own log
go run .
```
