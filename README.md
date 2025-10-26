# [gh-copilot-proxy](https://github.com/ilcm96/gh-copilot-proxy)

English | [한국어](README_ko.md)

## Overview

`gh-copilot-proxy` is a proxy server that lets you use the GitHub Copilot API with personal access tokens.

- **Automatic token loading**: Prefers the `COPILOT_OAUTH_TOKEN` environment variable; if absent, it searches for `"oauth_token"` in the existing config file under `$HOME/.config/github-copilot` (or `AppData/Local` on Windows).
- **Token lifecycle management**: `internal/auth` keeps the Copilot token in memory and refreshes it automatically.
- **Streaming support**: Handles both SSE-based and non-streaming responses.
- **OpenAI/Anthropic compatibility**: Accepts client requests written in either the OpenAI or Anthropic formats.

## Directory Structure

```
.
├── cmd/server/main.go          # Server bootstrap and execution
└── internal
    ├── auth                    # Copilot token management and auto-refresh
    ├── proxy                   # Routing, auth middleware, upstream forwarding
    ├── adapter                 # Anthropic/OpenAI conversion and SSE handling
    └── httpx                   # HTTP utilities (CORS, header copying, etc.)
```

## Build and Run

### Local

```bash
go build -o gh-copilot-proxy cmd/server/main.go
API_KEY=YOUR_API_KEY PORT=4000 ./gh-copilot-proxy
```

### Docker

```bash
API_KEY=$(jq -r 'first(.[] | select(has("oauth_token")) | .oauth_token)' \
    ~/.config/github-copilot/apps.json)

docker build -t gh-copilot-proxy .

docker run -d --name gh-copilot-proxy \
    -e COPILOT_OAUTH_TOKEN="${API_KEY}" \
    -e API_KEY=YOUR_API_KEY \
    -e PORT=4000 \
    -p 4000:4000 \
    gh-copilot-proxy
```

### Environment Variables

| Name                  | Default           | Description                                                                                                                |
| --------------------- | ----------------- | -------------------------------------------------------------------------------------------------------------------------- |
| `COPILOT_OAUTH_TOKEN` | None (required\*) | GitHub Copilot OAuth token. If empty, the proxy searches existing GitHub CLI/VS Code settings (`apps.json`, `hosts.json`). |
| `API_KEY`             | Auto-generated    | Bearer token for proxy access control. When empty, a cryptographically secure value is generated at startup.               |
| `PORT`                | `4000`            | Port to bind. Example: `5000`.                                                                                             |

- In containerized environments, providing `COPILOT_OAUTH_TOKEN` is recommended due to filesystem permission constraints.
- To obtain the GitHub Copilot OAuth token, execute the following command:
  ```bash
  jq -r 'first(.[] | select(has("oauth_token")) | .oauth_token)' \
      ~/.config/github-copilot/apps.json
  ```

## Supported Endpoints

- **OpenAI**
  - `/v1/chat/completions`
  - `/chat/completions`
  - `/v1/embeddings`
  - `/embeddings`
- **Anthropic**
  - `/v1/messages`
  - `/messages`

All endpoints expect the `Authorization: Bearer <API_KEY>` header.
