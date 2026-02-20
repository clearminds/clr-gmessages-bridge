# clr-gmessages-bridge

Fork of MaxGhenis/openmessage with Supabase sync. Google Messages bridge for Clearminds MCP infrastructure.

## Architecture

```
├── cmd/                    CLI commands (pair, serve, send)
├── internal/
│   ├── app/                App struct, backfill logic
│   ├── client/             libgm wrapper, event handling, SupabaseSync interface
│   ├── db/                 SQLite storage
│   ├── supabase/           Supabase PostgREST RPC writer + Storage API + migrations
│   ├── tools/              Built-in MCP tools (9 tools)
│   └── web/                HTTP API + static web UI
├── main.go                 CLI dispatcher
├── Dockerfile              Multi-stage Docker build
└── .github/workflows/      CI (test) + Docker build
```

## Key files

- `internal/supabase/supabase.go` — PostgREST RPC writer, Storage API uploads, auto-migration
- `internal/supabase/migrations/001_initial_schema.sql` — Tables, indexes, RPC functions
- `internal/client/events.go` — Event handler with SupabaseSync interface for Supabase writes
- `internal/app/app.go` — App struct with optional Supabase field, wiring
- `internal/app/backfill.go` — Backfill with Supabase sync
- `internal/web/api.go` — REST API with `/api/download` for media→Supabase
- `cmd/serve.go` — Server startup, media uploader closure

## Supabase sync pattern

- `SupabaseSync` interface in `internal/client/` (avoids import coupling)
- `*supabase.Writer` satisfies the interface via duck typing
- All Supabase writes are fire-and-forget goroutines (never block SQLite path)
- Optional: if `SUPABASE_URL`/`SUPABASE_KEY` not set, sync is disabled

## Testing

```bash
go test ./...   # 7 test suites
go vet ./...    # Lint
```

## Building

```bash
go build -o gmessages-bridge .    # Local
docker build -t gmessages-bridge . # Docker
```
