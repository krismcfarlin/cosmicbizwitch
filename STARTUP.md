# Platform Starter — New Project Setup

This is a Go + PocketBase + SQLite starter kit with:
- PocketBase admin UI at `/_/`
- In-memory log viewer at `/logs`
- Session-based authentication
- Generic MCP tools for PocketBase CRUD (`pb_list`, `pb_get`, `pb_create`, `pb_update`, `pb_delete`, `pb_schema`)
- Job queue processor
- Vector document store (go-libsql)

## Starting a New Project

### 1. Clone and rename
```bash
git clone https://github.com/krismcfarlin/platform-starter my-new-project
cd my-new-project
go mod edit -module my-new-project
go mod tidy
```

### 2. Configure environment
```bash
cp .env.example .env
# Edit .env: set PORT and DB_PATH
```

### 3. Define your data model
Edit `internal/app/storage/collections.go`:
- Follow the pattern in `example_collection.go`
- Add your collection creation functions
- Register them in `createCollections()`
- Delete `example_collection.go` when done

### 4. Add job types
Edit `internal/app/processor/queue.go`:
- Add cases to the job type dispatch switch
- Implement handler functions

### 5. Add HTTP pages
- Add handler files to `internal/app/server/`
- Register routes in `internal/app/server/server.go`

### 6. Build and run
```bash
go build ./internal/...
./platform-starter serve
# Admin UI: http://localhost:8083/_/
# Logs: http://localhost:8083/logs
```

### 7. Deploy
CGO is required for go-libsql (vector ops). Build on the server:
```bash
rsync -av --exclude='.git' --exclude='data/' --exclude='*.db' . user@server:/opt/myapp/src/
ssh user@server 'cd /opt/myapp/src && CGO_ENABLED=1 go build -o /tmp/myapp-new ./internal/app/cmd/ && systemctl restart myapp'
```

## Architecture

Two databases:
- `data.db` — PocketBase (business collections, admin UI, auth)
- `coaching.db` — go-libsql (vector search with F32_BLOB embeddings)

PocketBase collections are defined in code at startup and browsable via admin UI at `/_/`.

## MCP Tools

The MCP server at `/mcp/` exposes generic CRUD tools:
- `pb_schema` — list all collections and their fields
- `pb_list` — query records with filter/sort/pagination
- `pb_get` — fetch a single record by ID
- `pb_create` — create a new record
- `pb_update` — update fields on a record
- `pb_delete` — delete a record
