# VMARBLE — Sprint Plan: Remnant Flow MVP

> **Epic**: Quản trị Dòng chảy Vật tư & Tấm lẻ (The Remnant Flow)
> **Duration**: 12 tuần (24/03/2026 — 16/06/2026)
> **Methodology**: Kanban, review cadence 2 tuần
> **WIP Limit**: Mỗi người tối đa 2 task In Progress
> **Last updated**: 2026-03-25

---

## 1. Team

| ID | Tên | Vai trò chính | Vai trò phụ |
|----|-----|---------------|-------------|
| **Dev A** | DatVT | Backend Lead — Inventory, Cutting, Transactions | DevOps, CI/CD |
| **Dev B** | GiangDQ | Backend — Production, Costing, Algorithm | Integration tests |
| **Dev C** | QuyND | Frontend Lead — Next.js Kiosk, Dashboard, Mobile UX | QR/Barcode, UX |

---

## 2. Tech Stack

### Backend (existing)

| Component | Technology |
|-----------|-----------|
| Language | Go 1.24 |
| Database | PostgreSQL 17 |
| Router | gin-gonic/gin v1.10 |
| DB driver | jackc/pgx/v5 (pgxpool) |
| Migrations | pressly/goose/v3 |
| Config | caarlos0/env/v11 |
| Logging | log/slog (stdlib) |

### Frontend (new)

| Component | Technology |
|-----------|-----------|
| Framework | Next.js 15 (App Router) |
| Language | TypeScript 5 |
| Styling | Tailwind CSS 4 |
| State | TanStack Query (server state) + Zustand (client state) |
| QR Scan | html5-qrcode |
| Charts | Recharts |
| Mobile | Responsive PWA (mobile browser tại xưởng) |

### Infrastructure

| Component | Technology |
|-----------|-----------|
| Container | Docker Compose |
| CI | GitHub Actions |
| Staging | Docker Compose trên VPS |

---

## 3. Milestone Overview

```
MONTH 1: FOUNDATION + CORE CUTTING FLOW + FRONTEND SETUP
├── Sprint 1 (W01-W02): Transaction Safety + Test Foundation + FE Scaffold
├── Sprint 2 (W03-W04): Complete Cutting → Remnant Flow + Kiosk Screens

MONTH 2: REMNANT INTELLIGENCE + KIOSK FULL FLOW
├── Sprint 3 (W05-W06): Best Fit + FIFO Algorithm + Bin Location
├── Sprint 4 (W07-W08): Barcode/QR Printing + Kiosk Complete + Mobile Polish

MONTH 3: COSTING + DASHBOARD + POLISH
├── Sprint 5 (W09-W10): Actual Costing + Overflow Alert + Dashboard
├── Sprint 6 (W11-W12): E2E Testing + UAT + Production Deploy
```

---

## 4. Sprint Details

### Sprint 1 — Foundation & Transaction Safety (W01-W02)

**Goal**: Code base an toàn cho production, test coverage cho critical modules, frontend scaffold sẵn sàng.

| # | Task | Owner | Est | Priority | DoD |
|---|------|-------|-----|----------|-----|
| 1.1 | **DB transaction support** cho `inventory/pgstore.go` — wrap RecordCut trong `pgx.Tx` (update board_sheet + insert cutting_record + insert remnant = 1 atomic transaction) | DatVT | 3d | P0 | RecordCut atomic, rollback on failure |
| 1.2 | **Row-level locking** — `SELECT ... FOR UPDATE` cho board_sheet và remnant khi allocate, prevent double-allocation | DatVT | 2d | P0 | Concurrent test passes |
| 1.3 | **Unit tests cho `inventory/service.go`** — BR-K01 (stock check), BR-K03 (area conservation), BR-K04 (remnant lineage), BR-K05 (status lifecycle). Target >=80% | GiangDQ | 4d | P0 | `go test -cover` >= 80% cho inventory service |
| 1.4 | **Unit tests cho `production/service.go`** — BR-P01 (state machine), BR-P02 (plan approval), BR-P04 (metal requirement) | GiangDQ | 3d | P0 | `go test -cover` >= 80% cho production service |
| 1.5 | **Integration test setup** — Docker Compose test environment + test DB + seed fixtures | DatVT | 2d | P0 | `make test-integration` chạy xanh |
| 1.6 | **CI pipeline** — GitHub Actions: lint + test + build on PR | GiangDQ | 1d | P1 | PR blocked nếu test fail |
| 1.7 | **Next.js project scaffold** — init App Router, Tailwind, TanStack Query, project structure (`/app`, `/components`, `/lib/api`), connect tới Go API | QuyND | 3d | P0 | `npm run dev` chạy, fetch healthz thành công |
| 1.8 | **API client layer** — TypeScript API client (typed fetch wrapper hoặc openapi-typescript), error handling, auth token placeholder | QuyND | 2d | P0 | Type-safe API calls |
| 1.9 | **UI component library setup** — base components: Button (big touch), Card, Table, Modal, Toast. Mobile-first responsive | QuyND | 3d | P0 | Storybook hoặc demo page với tất cả components |
| 1.10 | **PWA manifest + responsive layout** — service worker, mobile viewport, bottom nav cho kiosk mode | QuyND | 2d | P1 | Installable trên Chrome mobile |

**Deliverable**: Backend có transaction + locking + 80% test coverage cho 2 module critical. Frontend scaffold với component library mobile-first.

---

### Sprint 2 — Complete Cutting → Remnant Flow (W03-W04)

**Goal**: Luồng hoàn chỉnh cắt → sinh tấm lẻ → kế thừa thuộc tính → nhập kho. Kiosk screens đầu tiên.

| # | Task | Owner | Est | Priority | DoD |
|---|------|-------|-----|----------|-----|
| 2.1 | **Mở rộng Remnant schema** — migration thêm: `supplier_code`, `lot_batch`, `grain_pattern`, `quality_grade`, `bounding_box_length_mm`, `bounding_box_width_mm`, `bin_location_id` (FK) | DatVT | 2d | P0 | Migration up/down OK |
| 2.2 | **Tạo bảng `storage_locations`** — migration: `id`, `zone`, `rack`, `shelf`, `label`, `barcode`, `is_active`. FK từ remnant | DatVT | 1d | P0 | Migration up/down OK |
| 2.3 | **Inheritance Logic (4.1)** — RecordCut auto-copy: supplier, lot, grain, quality từ source board_sheet hoặc parent_remnant sang remnant mới | DatVT | 3d | P0 | Test: remnant có đầy đủ thuộc tính kế thừa |
| 2.4 | **Bounding Box (4.2)** — thêm `usable_dimension` vào Remnant, validate `usable <= actual`, index cho search | GiangDQ | 3d | P0 | Search by min dimensions hoạt động |
| 2.5 | **Nested remnant cutting** — cắt remnant → sinh remnant mới. Lineage chain: remnant.parent_remnant_id. Area conservation áp dụng trên remnant source | GiangDQ | 3d | P0 | Test: cắt remnant 3 cấp, lineage trace OK |
| 2.6 | **API endpoints mới** — `GET /remnants?min_length=X&min_width=Y&material_type=Z&status=AVAILABLE`, `GET /remnants/:id/lineage`, `GET /storage-locations` | DatVT | 2d | P1 | Swagger updated, response typed |
| 2.7 | **Tests** cho tất cả tính năng mới (unit + integration) | GiangDQ | 2d | P0 | Coverage maintained >= 80% |
| 2.8 | **Kiosk: Màn hình "Lệnh cắt hôm nay"** — list work orders IN_CUTTING, hiển thị: SKU, kích thước, vật liệu, số lượng. Touch-friendly cards, pull-to-refresh | QuyND | 4d | P0 | Mobile Chrome render đúng, data từ API |
| 2.9 | **Kiosk: Màn hình "Báo cáo kết quả cắt"** — form nhập: kích thước used, kích thước remnant, toggle waste. Gọi RecordCut API. Success → hiển thị QR mới | QuyND | 4d | P0 | Submit thành công, API trả remnant_id |
| 2.10 | **Kiosk: Mobile navigation** — bottom tab bar: Lệnh cắt / Báo cáo / Kho tấm lẻ / Tài khoản | QuyND | 2d | P1 | Navigate giữa các tab mượt |

**Deliverable**: Backend remnant flow hoàn chỉnh với inheritance + bounding box + nested cutting. Kiosk 2 màn hình đầu tiên chạy trên mobile.

---

### Sprint 3 — Remnant Intelligence (W05-W06)

**Goal**: "Bộ não" của hệ thống — thuật toán gợi ý tấm lẻ tối ưu + quản lý vị trí kệ.

| # | Task | Owner | Est | Priority | DoD |
|---|------|-------|-----|----------|-----|
| 3.1 | **Best Fit + FIFO Algorithm** — `SuggestRemnants(required Dimension, materialType) → []RemnantSuggestion` với scoring: `fit_score = required_area / bounding_box_area` (higher = less waste) + `age_score = days_in_stock / max_days` (older = higher). Combined: `score = w1*fit_score + w2*age_score`. Weights configurable via env | DatVT | 5d | P0 | Top suggestion <= 10% waste trong test scenarios |
| 3.2 | **Allocation workflow** — `POST /inventory/suggest-allocation` body: `{work_order_id, required_dimensions[]}` → trả danh sách suggestions. `POST /inventory/allocate` → lock remnant. Auto-release sau 24h | DatVT | 3d | P0 | Allocate + timeout release test |
| 3.3 | **Allocation locking** — remnant status `AVAILABLE → ALLOCATED` với `allocated_to_wo_id` + `allocated_at`. Background goroutine hoặc cron: release nếu `allocated_at + 24h < now` | GiangDQ | 3d | P0 | Timeout release test passes |
| 3.4 | **Storage Location CRUD** — `POST/GET/PUT/DELETE /storage-locations`. Barcode cho mỗi location. Validate unique zone+rack+shelf | GiangDQ | 2d | P1 | CRUD hoạt động, barcode sinh đúng |
| 3.5 | **Remnant assign location** — `PUT /remnants/:id/location` body: `{location_id}`. Scan QR remnant + scan QR location | GiangDQ | 2d | P1 | Location gán đúng, searchable |
| 3.6 | **Algorithm tests** — edge cases: no fit found, exact fit, multiple equal fits (FIFO tiebreak), nested remnant fit, different material types | DatVT | 2d | P0 | 20+ test scenarios pass |
| 3.7 | **Kiosk: "Gợi ý tấm lẻ"** — khi chọn work order để cắt, hiện popup: "Hệ thống gợi ý X tấm lẻ phù hợp". Mỗi suggestion hiện: kích thước, vị trí kệ, tuổi, fit %. Nút "Chọn" hoặc "Bỏ qua, dùng tấm nguyên" | QuyND | 4d | P0 | Mobile UX test: < 3 tap để chọn remnant |
| 3.8 | **Kiosk: "Nhập kho tấm lẻ"** — scan remnant QR → hiện info. Scan location QR → gán vị trí. Confirm → done. | QuyND | 3d | P0 | Scan flow < 15 giây |
| 3.9 | **Kiosk: "Kho tấm lẻ"** — list remnants AVAILABLE, filter by material/size, hiện vị trí. Tap → detail (lineage, dimensions, age) | QuyND | 3d | P1 | Scroll mượt trên mobile với 100+ items |

**Deliverable**: Thuật toán gợi ý tối ưu hoạt động. Kiosk có flow hoàn chỉnh: gợi ý → chọn → cắt → nhập kho tấm lẻ. Mobile UX production-ready.

---

### Sprint 4 — Barcode/QR + Kiosk Complete + Mobile Polish (W07-W08)

**Goal**: QR printing tích hợp, flow xưởng end-to-end, mobile UX sẵn sàng cho công nhân.

| # | Task | Owner | Est | Priority | DoD |
|---|------|-------|-----|----------|-----|
| 4.1 | **QR code generation API** — `GET /barcode/:id/qr` trả QR image (PNG). Content: JSON `{type, id, sku, dimensions, lot, location}`. Dùng Go lib `skip2/go-qrcode` | DatVT | 2d | P0 | QR scan bằng phone → decode đúng JSON |
| 4.2 | **Label template engine** — `GET /barcode/:id/label` trả PDF label. Template: QR code + text info (SKU, kích thước, lot, vị trí). Support 2 sizes: 50x30mm (remnant), 100x70mm (WIP) | DatVT | 3d | P0 | PDF render đúng, in ra giấy đọc được |
| 4.3 | **Auto-generate barcode on RecordCut** — khi RecordCut thành công, auto-create barcode records cho: WIP items + remnant. Trả barcode IDs trong response | DatVT | 2d | P0 | RecordCut response chứa barcode_ids |
| 4.4 | **Batch print API** — `POST /barcode/batch-print` body: `{barcode_ids[]}` → trả multi-page PDF. Cho phép in nhiều tem một lúc | GiangDQ | 2d | P0 | 10 labels trong 1 PDF |
| 4.5 | **Scan event tracking** — `POST /barcode/scan` body: `{barcode_id, checkpoint, scanned_by}`. 3 checkpoints: CNC_COMPLETE, FINISHING_COMPLETE, WAREHOUSE_SHIP | GiangDQ | 2d | P0 | Scan events stored, queryable |
| 4.6 | **Unit tests cho barcode module** | GiangDQ | 2d | P0 | >= 80% coverage |
| 4.7 | **Kiosk: "In tem" flow** — sau báo cáo cắt → hiện nút "In tem". Preview label → confirm → trigger print (browser print dialog hoặc network printer API) | QuyND | 3d | P0 | Print dialog opens với đúng label |
| 4.8 | **Kiosk: QR camera scan** — dùng `html5-qrcode`. Camera permission handling, flash toggle, scan history. Works offline (queue scans) | QuyND | 3d | P0 | Scan 10 mã liên tiếp, 0 lỗi |
| 4.9 | **Kiosk: "Quét điểm kiểm tra"** — 3 checkpoint screens. Scan QR → hiện product info → confirm checkpoint → update status | QuyND | 3d | P1 | 3 checkpoints track đúng |
| 4.10 | **Mobile UX audit & polish** — test trên 3 devices thực (Android phone, Android tablet, iPad). Fix responsive issues, touch targets >= 48px, font >= 16px | QuyND | 2d | P0 | Không có lỗi UX trên 3 devices |

**Deliverable**: QR/barcode system hoàn chỉnh. Kiosk flow end-to-end: lệnh cắt → gợi ý remnant → cắt → in tem → nhập kho → scan checkpoint. Mobile-ready.

---

### Sprint 5 — Actual Costing + Overflow Alert + Dashboard (W09-W10)

**Goal**: Tính giá thành tự động, cảnh báo tràn kho, dashboard cho quản lý.

| # | Task | Owner | Est | Priority | DoD |
|---|------|-------|-----|----------|-----|
| 5.1 | **Enhance Costing** — tích hợp remnant value: `remnant_value = (remnant_area / source_area) * source_cost`. Multi-cut allocation chính xác | DatVT | 3d | P0 | Costing test: 1 sheet → 3 SKUs, cost sums correctly |
| 5.2 | **Nested remnant costing** — trace cost qua lineage chain. Remnant cắt từ remnant khác: cost = `(area / parent_remnant_area) * parent_remnant_value` | DatVT | 3d | P0 | 3-level nested costing test passes |
| 5.3 | **Costing report API** — `GET /costing/reports?po_id=X` → breakdown: material cost by SKU, waste %, remnant savings (so với dùng tấm nguyên) | DatVT | 2d | P1 | Report matches manual calculation |
| 5.4 | **Remnant Overflow Alert** — `GET /inventory/overflow-status`. Config: `REMNANT_OVERFLOW_THRESHOLD_PCT=15` (env). Logic: `total_remnant_area / total_raw_stock_area > threshold` → `{status: "RED", message: "...", block_new_sheet_issue: true}` | GiangDQ | 3d | P0 | Red alert triggers at > 15% |
| 5.5 | **Block new sheet issue** — khi overflow RED, `POST /inventory/issue-sheet` trả 422 với message: "Vui lòng sử dụng tấm lẻ. Kho tấm lẻ đang quá tải (X%)" | GiangDQ | 2d | P0 | Sheet issue blocked khi overflow |
| 5.6 | **Dashboard APIs** — `GET /dashboard/remnant-summary` (total count, total area, avg age, by material), `GET /dashboard/cutting-efficiency` (waste % by week/month), `GET /dashboard/overflow-history` | GiangDQ | 3d | P0 | All endpoints return correct data |
| 5.7 | **Dashboard UI: Tổng quan** — cards: tổng tấm lẻ, tổng diện tích, tuổi TB, overflow indicator (green/yellow/red). Charts: cutting efficiency trend, remnant by material pie | QuyND | 4d | P0 | Dashboard render trên desktop + tablet |
| 5.8 | **Dashboard UI: Costing** — bảng cost by PO, drill-down by SKU. Hiện: material cost, waste cost, remnant savings, total | QuyND | 3d | P1 | Data matches API |
| 5.9 | **Dashboard UI: Overflow Alert banner** — red banner sticky top khi overflow. Click → xem chi tiết remnant cần tiêu thụ | QuyND | 2d | P0 | Banner hiện đúng status |
| 5.10 | **Tests** cho costing + overflow logic | GiangDQ | 2d | P0 | Coverage >= 80% |

**Deliverable**: Costing tự động (kể cả nested remnant), overflow alert + block, dashboard cho quản lý.

---

### Sprint 6 — E2E Testing + UAT + Production Deploy (W11-W12)

**Goal**: Kiểm thử toàn diện, fix bugs, deploy production, demo cho khách hàng.

| # | Task | Owner | Est | Priority | DoD |
|---|------|-------|-----|----------|-----|
| 6.1 | **E2E test suite** — full flow automated: tạo PO → Plan → WorkOrder → gợi ý remnant → cắt → sinh remnant → nhập kho → re-cut → cost. Cover happy path + 5 error scenarios | GiangDQ | 4d | P0 | E2E chạy xanh trong CI |
| 6.2 | **Performance testing** — load test: 10,000 remnants trong DB, 100 concurrent suggestion requests. Target p95 < 500ms. Optimize indexes nếu cần | GiangDQ | 2d | P1 | p95 < 500ms |
| 6.3 | **Auth integration** — JWT login. Kiosk mode: simplified PIN login (4 số). Dashboard: full login. Role-based: OPERATOR, WAREHOUSE, PLANNING, ACCOUNTING, ADMIN | DatVT | 3d | P1 | Login flow hoạt động, roles enforced |
| 6.4 | **Remnant Inventory UI (desktop)** — full table: search, sort by age/size/material, multi-filter. Click row → detail panel (lineage tree visualization, location, all attributes) | QuyND | 4d | P0 | Desktop Chrome + Safari render đúng |
| 6.5 | **Lineage tree visualization** — visual tree: tấm nguyên → remnant 1 → remnant 1.1. Hiện dimensions + cost at each node | QuyND | 3d | P1 | Tree render tới 4 levels |
| 6.6 | **Bug fixing & UX polish** — từ internal testing + stakeholder preview | All | 3d | P0 | 0 P0 bugs, < 3 P1 bugs |
| 6.7 | **Production deploy setup** — Docker Compose production config, nginx reverse proxy, SSL, backup cron, monitoring (uptime check) | DatVT | 2d | P0 | Production accessible via HTTPS |
| 6.8 | **User documentation** — hướng dẫn sử dụng Kiosk (có hình), FAQ, troubleshooting. Viết bằng tiếng Việt | QuyND | 2d | P1 | PDF/web guide hoàn chỉnh |
| 6.9 | **UAT with customer** — demo live tại xưởng, công nhân thử Kiosk, quản lý thử Dashboard. Thu thập feedback, phân loại Phase 2 | All | 2d | P0 | Customer sign-off hoặc feedback list |
| 6.10 | **Retrospective & Phase 2 planning** — tổng kết 3 tháng, velocity analysis, Phase 2 backlog grooming | All | 1d | P1 | Phase 2 backlog prioritized |

**Deliverable**: MVP production-ready. Customer UAT complete. Phase 2 backlog established.

---

## 5. Task Distribution Summary

Tổng quan phân công qua 6 sprints:

### DatVT (Dev A) — Backend Lead

| Sprint | Focus |
|--------|-------|
| S1 | Transaction safety, row locking, integration test setup |
| S2 | Remnant schema expansion, inheritance logic, new APIs |
| S3 | **Best Fit + FIFO algorithm**, allocation workflow |
| S4 | QR generation, label engine, auto-barcode on cut |
| S5 | Costing enhancement, nested remnant costing |
| S6 | Auth, production deploy |

### GiangDQ (Dev B) — Backend + Testing

| Sprint | Focus |
|--------|-------|
| S1 | Unit tests inventory + production, CI pipeline |
| S2 | Bounding box, nested cutting, tests |
| S3 | Allocation locking, storage location CRUD |
| S4 | Batch print, scan events, barcode tests |
| S5 | Overflow alert, dashboard APIs, tests |
| S6 | **E2E test suite**, performance testing |

### QuyND (Dev C) — Frontend Lead

| Sprint | Focus |
|--------|-------|
| S1 | Next.js scaffold, API client, component library, PWA |
| S2 | Kiosk: lệnh cắt + báo cáo cắt, mobile nav |
| S3 | Kiosk: gợi ý tấm lẻ, nhập kho, kho tấm lẻ |
| S4 | In tem, QR camera scan, checkpoint screens, mobile polish |
| S5 | Dashboard UI: tổng quan, costing, overflow banner |
| S6 | Remnant inventory desktop, lineage tree, user docs |

---

## 6. Definition of Done (DoD)

Mỗi task phải thỏa mãn **tất cả** điều kiện:

- [ ] Code follows conventions trong `CLAUDE.md`
- [ ] Unit tests pass (`make test`), coverage >= 80% cho service layer
- [ ] Lint pass (`make lint`)
- [ ] Code reviewed bởi ít nhất 1 team member khác
- [ ] API: Swagger/OpenAPI updated
- [ ] Frontend: responsive trên mobile (375px) + tablet (768px) + desktop (1280px)
- [ ] Deployed to staging environment
- [ ] PR merged vào `dev` branch

---

## 7. Risk Register

| # | Risk | Prob | Impact | Mitigation | Owner |
|---|------|------|--------|------------|-------|
| R1 | Công nhân không chịu dùng Kiosk/mobile | Cao | Cao | UX max 3 bước, in tem tự động, training hands-on tại xưởng | QuyND |
| R2 | Best Fit algorithm chậm khi remnant nhiều | TB | TB | Composite index trên `(material_type, status, bounding_box_*)`, pagination | DatVT |
| R3 | Customer thay đổi yêu cầu giữa chừng | Cao | TB | Scope lock per sprint, backlog parking lot, review 2 tuần | All |
| R4 | QR scan không ổn định trên mobile browser | TB | Cao | Fallback: manual input mã số, test trên nhiều devices sớm (S2) | QuyND |
| R5 | Tích hợp máy in tem phức tạp | TB | Thấp | Fallback: PDF label → browser print dialog | QuyND |
| R6 | Next.js + Go CORS/proxy issues | Thấp | Thấp | Next.js API routes proxy, hoặc Go CORS middleware sẵn có | QuyND |
| R7 | 3 devs không đủ bandwidth | TB | Cao | Cut scope: Auth, Performance test, Lineage tree vis là P1, có thể defer | All |

---

## 8. MVP Scope

### In Scope (ship trong 3 tháng)

1. Luồng cắt hoàn chỉnh: tấm nguyên → CNC → WIP + tấm lẻ + waste
2. Tấm lẻ thông minh: kế thừa thuộc tính, bounding box, vị trí kệ, lineage
3. Gợi ý tấm lẻ tối ưu: Best Fit + FIFO
4. Allocation & locking: prevent double-use, 24h auto-release
5. Cảnh báo tràn kho: red alert + block xuất tấm nguyên
6. Kiosk mobile: lệnh cắt → gợi ý → cắt → in tem → nhập kho → scan checkpoint
7. QR/barcode: auto-generate, label printing, 3 scan checkpoints
8. Costing tự động: area-based, nested remnant support
9. Dashboard: remnant stats, cutting efficiency, overflow, costing by PO
10. Test coverage >= 80% cho business logic
11. CI/CD: automated lint + test + staging deploy

### Out of Scope (Phase 2+)

- Stone workshop module
- Full P&L & cash flow reporting
- Vendor/purchasing management
- Labor costing (chờ confirm phương pháp với khách)
- Native mobile app (MVP dùng PWA)
- External accounting integration (Misa)
- Workforce & shift scheduling
- Remnant image capture (chụp ảnh tấm lẻ)

---

## 9. Open Decisions (cần confirm với khách hàng)

| # | Decision | Impact | Default Assumption | Deadline |
|---|----------|--------|--------------------|----------|
| 1 | Algorithm weights (fit vs FIFO) | Ảnh hưởng suggestion quality | `w1=0.6, w2=0.4` (fit ưu tiên hơn) | Sprint 3 |
| 2 | Overflow threshold % | Khi nào block xuất tấm nguyên | 15% | Sprint 5 |
| 3 | Allocation timeout duration | Bao lâu auto-release remnant locked | 24 giờ | Sprint 3 |
| 4 | Label printer model | Ảnh hưởng integration approach | ZPL-compatible (Zebra) hoặc PDF fallback | Sprint 4 |
| 5 | Kiosk auth method | PIN, badge, hoặc no auth? | 4-digit PIN per operator | Sprint 6 |
| 6 | Waste allocation policy | Waste tính vào đâu? | Absorbed as overhead (BR-C03) | Sprint 5 |
| 7 | Hosting environment | VPS, cloud, on-premise? | Docker Compose on VPS | Sprint 6 |

---

## 10. KPIs

| KPI | Target | How to Measure | When |
|-----|--------|----------------|------|
| Test coverage (service layer) | >= 80% | `go test -cover ./internal/module/*/service.go` | Every PR |
| API response time (p95) | < 500ms | Load test with 10K remnants | Sprint 6 |
| Remnant suggestion quality | Best suggestion <= 10% waste | 20 manual test scenarios | Sprint 3 |
| Kiosk task completion | < 60 seconds full flow (view → cut → label → store) | Stopwatch test at factory | Sprint 6 |
| Mobile UX score | 0 blocking issues on 3 test devices | Manual testing | Sprint 4 |
| Sprint completion rate | >= 85% P0 tasks done per sprint | Kanban board | Every sprint |
| Customer UAT pass rate | >= 90% acceptance criteria met | UAT session | Sprint 6 |

---

## 11. Communication & Ceremonies

| Ceremony | Frequency | Duration | Participants |
|----------|-----------|----------|-------------|
| Daily standup (async) | Daily | Slack update | All |
| Sprint review + demo | Bi-weekly | 60 min | All + stakeholders |
| Code review | Per PR | Async | 2 reviewers required |
| Backlog grooming | Weekly | 30 min | All |
| Customer check-in | Bi-weekly | 30 min | DatVT + customer |
| Retrospective | Bi-weekly | 30 min | All |

---

## 12. Branch Strategy

```
main (production)
  ↑ PR (requires 1 approval)
dev (integration)
  ↑ PR (from feature branches)
feature/s1-transaction-safety     ← DatVT
feature/s1-inventory-tests        ← GiangDQ
feature/s1-nextjs-scaffold        ← QuyND
...
```

PR naming: `[module] brief description` (e.g., `[inventory] add transaction support for RecordCut`)

---

## Appendix A: Frontend Project Structure (Next.js)

```
frontend/
├── app/
│   ├── (kiosk)/              # Kiosk layout (mobile-optimized)
│   │   ├── cutting-orders/   # Lệnh cắt hôm nay
│   │   ├── report-cut/       # Báo cáo kết quả cắt
│   │   ├── remnant-store/    # Nhập kho tấm lẻ
│   │   ├── remnant-list/     # Kho tấm lẻ
│   │   ├── scan/             # QR scan checkpoints
│   │   └── layout.tsx        # Bottom nav, big touch targets
│   ├── (dashboard)/          # Dashboard layout (desktop/tablet)
│   │   ├── overview/         # Tổng quan
│   │   ├── remnants/         # Remnant inventory table
│   │   ├── costing/          # Cost reports
│   │   └── layout.tsx        # Side nav, charts
│   ├── login/
│   └── layout.tsx            # Root layout
├── components/
│   ├── ui/                   # Base: Button, Card, Table, Modal, Toast
│   ├── kiosk/                # BigButton, ScannerView, LabelPreview
│   └── dashboard/            # Charts, StatCard, AlertBanner
├── lib/
│   ├── api/                  # Typed API client
│   ├── hooks/                # useRemnants, useCuttingOrders, useScan
│   └── utils/
├── public/
│   └── manifest.json         # PWA manifest
└── next.config.ts
```

---

## Appendix B: Key API Endpoints (new + enhanced)

| Method | Path | Sprint | Description |
|--------|------|--------|-------------|
| GET | `/api/v1/remnants` | S2 | List remnants with filter (dimensions, material, status, location) |
| GET | `/api/v1/remnants/:id/lineage` | S2 | Get lineage tree of a remnant |
| POST | `/api/v1/inventory/suggest-allocation` | S3 | Get Best Fit + FIFO suggestions |
| POST | `/api/v1/inventory/allocate` | S3 | Lock remnant for work order |
| POST | `/api/v1/inventory/release-allocation` | S3 | Manual release locked remnant |
| CRUD | `/api/v1/storage-locations` | S3 | Manage bin locations |
| PUT | `/api/v1/remnants/:id/location` | S3 | Assign remnant to bin location |
| GET | `/api/v1/barcode/:id/qr` | S4 | Get QR code image |
| GET | `/api/v1/barcode/:id/label` | S4 | Get printable label PDF |
| POST | `/api/v1/barcode/batch-print` | S4 | Batch print labels |
| POST | `/api/v1/barcode/scan` | S4 | Record scan event |
| GET | `/api/v1/inventory/overflow-status` | S5 | Get overflow alert status |
| GET | `/api/v1/dashboard/remnant-summary` | S5 | Remnant statistics |
| GET | `/api/v1/dashboard/cutting-efficiency` | S5 | Waste % by period |
| GET | `/api/v1/costing/reports` | S5 | Cost report by PO with breakdown |
