---
name: rbac-hardener
description: >
  Use to audit and enforce role-based access control (RBAC) across all API endpoints.
  Scans handler routes, maps required roles from the business spec, generates
  RequireRole middleware guards, and reports orphan (unprotected) endpoints.
  Trigger words: "rbac", "phân quyền", "role", "access control", "security audit",
  "guard endpoints", "protect routes".
---

# RBAC Hardener — Role-Based Access Control for VMARBLE WMS

## Reference: Roles & Permissions (from spec section 5)

| Role | System name | Can do |
|---|---|---|
| Admin / Sếp | `admin` | Read-only on ALL modules, dashboard |
| Kế toán | `accountant` | CRUD PO, costing, finalize, export reports |
| Bộ phận KH | `planner` | CRUD Production Plan, view inventory |
| Quản lý Kho | `warehouse` | CRUD inventory transactions, remnants, consumption records |
| Quản lý CNC | `cnc_manager` | Assign WO to CNC operators, view progress |
| Vận hành CNC | `cnc` | View assigned WOs, update cutting status, scan barcode (Kiosk) |
| Tổ trưởng SX | `foreman` | Report consumption, update WO status |

---

## Phase 1 — Route Scan

Scan ALL handler files to build a complete route inventory:

```bash
grep -rn 'rg\.\(GET\|POST\|PUT\|PATCH\|DELETE\)' internal/module/*/handler.go
```

For each route, record:
- **Method** (GET/POST/PUT/DELETE)
- **Path** (e.g. `/api/v1/barcodes/:id`)
- **Module** (e.g. barcode)
- **Handler function name**
- **Current guard** (RequireRole? None?)

Output a markdown table of ALL routes.

---

## Phase 2 — Role Mapping

Map each route to allowed roles based on the business spec:

### catalog module
| Route | Allowed roles | Reason |
|---|---|---|
| `GET /materials`, `GET /skus` | ALL authenticated | Reference data, everyone reads |
| `POST /materials`, `POST /skus` | `admin`, `warehouse` | Only warehouse/admin creates catalog |
| `DELETE /materials/:id`, `DELETE /skus/:id` | `admin` | Deactivation is admin-only |
| `POST /skus/:id/bom`, `GET /skus/:id/bom` | `admin`, `warehouse`, `planner` | BOM management |

### order module
| Route | Allowed roles | Reason |
|---|---|---|
| `POST /pos` | `accountant`, `admin` | Only accounting creates POs |
| `GET /pos`, `GET /pos/:id` | ALL authenticated | Everyone can view POs |
| `DELETE /pos/:id` | `accountant`, `admin` | Only accounting deactivates |
| `GET /pos/:id/line-items` | ALL authenticated | Reference data |

### planning module
| Route | Allowed roles | Reason |
|---|---|---|
| `POST /plans` | `planner`, `admin` | Only planners create plans |
| `GET /plans`, `GET /plans/:id` | ALL authenticated | Everyone views |
| `POST /plans/:id/approve` | `planner`, `admin` | Only planners approve |
| `POST /plans/:id/cancel` | `planner`, `admin` | Only planners cancel |

### inventory module
| Route | Allowed roles | Reason |
|---|---|---|
| `POST /inventory/receive` | `warehouse`, `admin` | Only warehouse receives stock |
| `GET /inventory/lots` | ALL authenticated | Everyone views |
| `POST /inventory/record-cut` | `warehouse`, `cnc`, `cnc_manager` | Cutting operation |
| `GET /inventory/sheets`, `GET /inventory/sheets/:id` | ALL authenticated | Everyone views |
| `POST /inventory/sheets/:id/pre-assign` | `cnc_manager`, `warehouse` | Sheet pre-assignment |
| `GET /inventory/remnants` | ALL authenticated | Everyone views |
| `GET /inventory/remnants/:id` | ALL authenticated | Everyone views |
| `GET /inventory/remnants/find` | `warehouse`, `cnc`, `cnc_manager` | Remnant search for cutting |
| `POST /inventory/remnants/suggest` | `cnc`, `cnc_manager`, `warehouse` | Remnant suggestion for kiosk |
| `POST /inventory/remnants/:id/allocate` | `cnc`, `cnc_manager`, `warehouse` | Allocate remnant |
| `POST /inventory/remnants/:id/waste` | `warehouse`, `admin` | Mark waste is warehouse decision |
| `POST /inventory/remnants/:id/stock` | `warehouse` | Physical bin assignment |
| `GET /inventory/remnants/:id/lineage` | ALL authenticated | Traceability |
| `GET /inventory/storage-locations` | ALL authenticated | Reference data |

### production module
| Route | Allowed roles | Reason |
|---|---|---|
| `POST /work-orders` | `planner`, `cnc_manager`, `admin` | Create WO from plan |
| `GET /work-orders` | ALL authenticated | Everyone views |
| `GET /work-orders/:id` | ALL authenticated | Everyone views |
| `GET /work-orders/mine` | `cnc` | Already guarded |
| `POST /work-orders/:id/status` | `cnc`, `cnc_manager`, `warehouse`, `foreman` | Status transitions |
| `POST /work-orders/:id/assign` | `cnc_manager` | Already guarded |
| `POST /work-orders/:id/suggest-assignment` | `cnc_manager` | Already guarded |
| `POST /work-orders/:id/consumptions` | `warehouse`, `foreman` | Record material usage |
| `GET /work-orders/:id/consumptions` | ALL authenticated | Everyone views |

### costing module
| Route | Allowed roles | Reason |
|---|---|---|
| `POST /costing/:id/compute` | `accountant`, `admin` | Only accounting computes cost |
| `POST /costing/:id/finalize` | `accountant`, `admin` | Only accounting finalizes |
| `GET /costing/:id` | `accountant`, `admin`, `planner` | Cost data is sensitive |
| `GET /costing` | `accountant`, `admin` | Cost list is sensitive |

### barcode module
| Route | Allowed roles | Reason |
|---|---|---|
| `POST /barcodes` | `warehouse`, `cnc`, `cnc_manager` | Generate barcode after cut |
| `GET /barcodes` | ALL authenticated | Everyone views |
| `GET /barcodes/:id` | ALL authenticated | Everyone views |
| `GET /barcodes/:id/qr` | ALL authenticated | QR image for scanning |
| `POST /barcodes/:id/scans` | `cnc`, `warehouse`, `foreman` | Scan events at checkpoints |
| `GET /barcodes/:id/scans` | ALL authenticated | Everyone views |

---

## Phase 3 — Implementation

### Pattern: add RequireRole to handler.go Register()

```go
import "github.com/vmarble/warehouse-management-service/internal/platform/auth"

func (h *Handler) Register(rg *gin.RouterGroup) {
    // Public reads (any authenticated user)
    rg.GET("/resources", h.list)
    rg.GET("/resources/:id", h.get)

    // Guarded writes
    rg.POST("/resources",
        auth.RequireRole(auth.RoleWarehouse, auth.RoleAdmin),
        h.create,
    )
    rg.DELETE("/resources/:id",
        auth.RequireRole(auth.RoleAdmin),
        h.deactivate,
    )
}
```

### Rules

1. **Read endpoints** (GET) that return non-sensitive data: allow ALL authenticated users
2. **Write endpoints** (POST/PUT/DELETE): guard with specific roles from the mapping above
3. **Sensitive reads** (costing, reports): guard with `accountant` + `admin`
4. **Never use wildcard**: always list explicit roles
5. **Admin always included**: `admin` can access everything (read-only or full, per spec)

---

## Phase 4 — Verification

After applying guards, verify with this audit script:

```bash
# List all routes and their guards
grep -A1 'rg\.\(GET\|POST\|PUT\|PATCH\|DELETE\)' internal/module/*/handler.go \
  | grep -E '(GET|POST|PUT|PATCH|DELETE|RequireRole)'
```

### Checklist

- [ ] Every `POST/PUT/DELETE` route has a `RequireRole` guard
- [ ] Costing `GET` routes are guarded (sensitive financial data)
- [ ] `GET /healthz` is NOT behind auth middleware
- [ ] `POST /api/auth/login` is NOT behind auth middleware
- [ ] No endpoint is accidentally open to `cnc` that should be `accountant`-only
- [ ] Run the full test suite: `make test` (RequireRole is middleware, not service logic — tests should still pass)

### Manual smoke test

For each role, verify access:

```bash
# Login as each role and test
TOKEN=$(curl -s POST /api/auth/login -d '{"username":"admin","password":"..."}' | jq -r .token)

# Should succeed (admin reads everything)
curl -H "Authorization: Bearer $TOKEN" GET /api/v1/costing

# Login as CNC
CNC_TOKEN=$(curl -s POST /api/auth/login -d '{"username":"cnc1","password":"..."}' | jq -r .token)

# Should return 403 (CNC cannot access costing)
curl -H "Authorization: Bearer $CNC_TOKEN" GET /api/v1/costing
```

---

## Phase 5 — Orphan Report

After all guards are applied, generate the final orphan report:

| Status | Meaning |
|---|---|
| GUARDED | Has `RequireRole` middleware |
| AUTH-ONLY | Behind JWT middleware but no role check (acceptable for public reads) |
| OPEN | No auth at all (only `/healthz` and `/api/auth/login` should be here) |
| ORPHAN | Should be guarded but is not — **fix immediately** |

Any route marked ORPHAN is a security gap and must be resolved before go-live.
