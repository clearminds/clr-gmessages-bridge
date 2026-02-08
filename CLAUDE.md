# OpenMessage

Open-source Google Messages client for macOS with built-in MCP server.

## Architecture

```
├── cmd/              Go CLI commands (pair, serve)
├── internal/         Go backend (app, client, db, web, tools)
├── macos/            Swift macOS app wrapper
│   ├── OpenMessage/ Swift package (BackendManager, PairingView, etc.)
│   └── build.sh      Builds universal binary + .app + .dmg
├── site/             Static website (deployed to openmessage.ai)
└── vercel.json       Vercel config (root — NOT site/vercel.json)
```

## Vercel deployment (openmessage.ai)

**CRITICAL: Always deploy from the repo root**, not from `~` or any other directory. The `.vercel/project.json` links to the correct project/scope.

**Config lives at root `vercel.json`**, not `site/vercel.json`. The root config sets `outputDirectory: "site"` and `cleanUrls: true`. A `.vercelignore` excludes Go/Swift build artifacts.

**Scope: `max-ghenis-projects`** (personal account, NOT PolicyEngine).

Deploy:
```bash
cd /Users/maxghenis/openmessages && vercel --prod
```

**Always verify after deploy:**
```bash
curl -s -o /dev/null -w "%{http_code}" https://openmessage.ai
```

**Domains:** `openmessage.ai` (primary) and `openmessages.ai` (alias), both on Cloudflare DNS → 76.76.21.21.

## Building the macOS app

```bash
./macos/build.sh
```

This builds: Go universal binary (arm64+amd64) → Swift app → .app bundle → .dmg

To install locally:
```bash
cp -R macos/build/OpenMessage.app /Applications/ && xattr -cr /Applications/OpenMessage.app
```

To update the GitHub release:
```bash
gh release upload v0.1.0 macos/build/OpenMessage.dmg --repo MaxGhenis/openmessage --clobber
```

## Testing

```bash
go test ./cmd/ -v      # Unit + integration tests
go test ./... -v       # All tests
```

## Key files

- `internal/app/app.go` — data dir resolution (`OPENMESSAGES_DATA_DIR` env var, defaults to `~/.local/share/openmessage`)
- `internal/client/events.go` — handles Google Messages protocol events
- `macos/OpenMessage/Sources/BackendManager.swift` — launches Go backend, manages app state
- `macos/OpenMessage/Sources/PairingView.swift` — QR code pairing UI
