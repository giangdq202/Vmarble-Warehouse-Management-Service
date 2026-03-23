# Runbook: Debugging Guide

## Log analysis

The service uses `log/slog` structured logging. Key fields:

- `module` — which domain module produced the log
- `handler` — HTTP handler name
- `method` / `path` — request details
- `status` — HTTP response status code
- `error` — error message (if any)
- `duration` — request duration

### Adjusting log level

Set `LOG_LEVEL` in `.env`:
```bash
LOG_LEVEL=debug   # Verbose — includes SQL timing
LOG_LEVEL=info    # Default — requests + business events
LOG_LEVEL=warn    # Minimal — only warnings and errors
LOG_LEVEL=error   # Errors only
```

## Common issues

### 404 on API endpoints

1. Check route registration in the module's `handler.go`
2. Verify the module is wired in `cmd/server/main.go`
3. Check Swagger docs match expected paths: `GET /swagger/index.html`

### 409 Conflict (invalid state transition)

The WorkOrder state machine is strict:
```
PLANNED → IN_CUTTING → IN_PROCESSING → COMPLETED → COSTED
```

Check current status before attempting transition:
```sql
SELECT id, status FROM work_orders WHERE id = '<uuid>';
```

### 422 Area conservation error (BR-K03)

The cutting operation validates: `used_area + remnant_area <= source_area`

Debug by checking:
```sql
-- Source sheet area
SELECT id, width_mm, height_mm, (width_mm * height_mm) as area_mm2
FROM board_sheets WHERE id = '<sheet_id>';

-- Existing cuts from this sheet
SELECT id, used_area_mm2, remnant_area_mm2
FROM cutting_records WHERE source_board_id = '<sheet_id>';
```

### 422 Insufficient stock

Check available inventory:
```sql
SELECT id, material_id, quantity, status
FROM inventory_lots
WHERE material_id = '<material_id>' AND status = 'AVAILABLE';
```

### 412 Precondition failed

Usually means a concurrent modification. Retry the operation or check for stale data.

## Database inspection

### Connect to local DB
```bash
docker exec -it vmarble-postgres psql -U vmarble -d vmarble
```

### Useful queries
```sql
-- Check migration version
SELECT * FROM goose_db_version ORDER BY id DESC LIMIT 5;

-- Count records per table
SELECT schemaname, relname, n_live_tup
FROM pg_stat_user_tables ORDER BY n_live_tup DESC;

-- Check active connections
SELECT count(*) FROM pg_stat_activity WHERE datname = 'vmarble';
```
