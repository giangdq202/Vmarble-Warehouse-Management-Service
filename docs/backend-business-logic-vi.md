# WAREHOUSE MANAGEMENT SYSTEM
## Tài liệu Logic Nghiệp vụ Backend
### Module Xưởng Gỗ - Sản xuất Carcass

Phiên bản 1.0 - 17/03/2026

---

# 1. TỔNG QUAN NGHIỆP VỤ

## 1.1 Bối cảnh công ty

Công ty hoạt động trong lĩnh vực xuất nhập khẩu, chuyên sản xuất nội thất mặt đá (bàn, tủ, giường...). Sản phẩm được cấu tạo gồm hai lớp: lớp khung gỗ (carcass / hàng trắng) và lớp mặt đá dán lên trên.

Hiện có hai xưởng chính:

- Xưởng Gỗ (Carcass) - đang setup, là phạm vi ưu tiên của hệ thống này.
- Xưởng Đá (Stone) - chưa đưa vào scope, sẽ phát triển sau.

## 1.2 Vật tư & Thuật ngữ

| Thuật ngữ | Tiếng Việt | Mô tả chi tiết |
|---|---|---|
| Carcass | Hàng trắng | Khung gỗ thành phẩm chưa dán đá - output chính của xưởng gỗ |
| Plywood | Ván ép | Nguyên liệu chính - tấm ván gỗ ép, cắt thành hình dạng theo đơn hàng |
| Keo Titebond | Keo gỗ chuyên dụng | Vật tư phụ - dùng để ghép và dán các bộ phận gỗ |
| Keo 502 | Keo siêu dính | Vật tư phụ - dùng trong một số công đoạn gia công đặc biệt |
| Metal / Bát sắt | Tấm lót kim loại | Vật tư đặc biệt - chỉ xuất hiện ở ~10-15% mã hàng yêu cầu gia cố |
| SKU | Mã hàng | Mã định danh duy nhất cho từng mã thành phẩm |
| PO | Purchase Order / Đơn hàng | Đơn đặt hàng từ khách nước ngoài - điểm khởi đầu của toàn bộ luồng |
| Costing | Tính giá thành | Quy trình tính chi phí sản xuất cho từng SKU |
| Remnant | Vật tư dư / tồn lại | Phần nguyên liệu còn lại sau khi cắt, được tái sử dụng cho mã hàng khác |
| Hao hụt | Waste / Scrap | Phần nguyên liệu bị thải bỏ, không tái sử dụng được |

## 1.3 Phân loại vật tư

Mỗi lệnh sản xuất bao gồm ba nhóm vật tư:

- **Vật tư chính**: Plywood (ván ép) - chiếm phần lớn chi phí nguyên liệu.
- **Vật tư phụ**: Keo Titebond, Keo 502 và khoảng 3-4 loại tương tự. Số lượng thực tế tiêu hao được báo cáo bởi công nhân sản xuất và nhập bởi quản lý kho.
- **Vật tư đặc biệt (Metal)**: Tấm lót sắt. Chỉ được đưa vào BOM nếu mã hàng yêu cầu; không áp dụng đại trà.

> Note: Khi thiết kế bảng BOM (Bill of Materials), cần có flag `is_metal_required` hoặc danh sách component linh hoạt để xử lý điều kiện này.

---

# 2. LUỒNG NGHIỆP VỤ CHÍNH (End-to-End Flow)

## 2.1 Sơ đồ tổng quan

Toàn bộ chu trình sản xuất đi qua 6 giai đoạn nối tiếp:

| # | Giai đoạn | Actor | Output |
|---|---|---|---|
| 1 | Tiếp nhận đơn hàng (PO) | Sales / Kế toán | Bản ghi PO trong hệ thống |
| 2 | Lập kế hoạch sản xuất | Bộ phận Kế hoạch | Production Plan (danh sách SKU + số lượng) |
| 3 | Cắt nguyên liệu (CNC) | Vận hành máy CNC + Quản lý kho | Carcass thô + bản ghi tồn kho remnant |
| 4 | Gia công thành phẩm | Công nhân sản xuất | Carcass thành phẩm đã đánh bóng, mài, dán |
| 5 | Tính giá thành (Costing) | Kế toán | Unit cost của từng SKU |
| 6 | Quản lý dòng tiền | Kế toán | P&L theo đơn hàng / kỳ kế toán |

## 2.2 Chi tiết từng giai đoạn

### Giai đoạn 1 - Tiếp nhận đơn hàng (PO)

Đơn hàng đến từ khách hàng nước ngoài. Kế toán nhập PO vào hệ thống với các thông tin:

- Mã PO
- Danh sách SKU và số lượng từng SKU
- Giá bán (selling price) theo SKU
- Ngày giao hàng dự kiến

> Note: Giá bán được nhập ở giai đoạn này và sẽ đối chiếu với costing ở giai đoạn 5 để ra lợi nhuận.

### Giai đoạn 2 - Lập kế hoạch sản xuất

Bộ phận kế hoạch nhận PO và tạo Production Plan. Một PO có thể tạo ra nhiều Production Plan theo đợt sản xuất.

Production Plan bao gồm:

- Danh sách SKU cần sản xuất trong đợt này.
- Số lượng từng SKU.
- Hình dạng cần cắt: bàn tròn (round), bàn chữ nhật (rect-dining) hay hình dạng khác.
- Thứ tự ưu tiên hoặc deadline nội bộ.

Kế hoạch sau khi được duyệt sẽ được đẩy xuống xưởng. Xưởng nhận kế hoạch qua hệ thống, không qua giấy tờ thủ công.

### Giai đoạn 3 - Cắt nguyên liệu (CNC)

Đây là giai đoạn phức tạp nhất về mặt quản lý kho. Máy CNC nhận lệnh từ kế hoạch và cắt plywood theo hình dạng yêu cầu.

Quy trình cụ thể:

- Quản lý kho xuất tấm plywood từ kho (theo mã tấm, kích thước gốc).
- Lập trình viên CNC nhập chương trình cắt vào máy. Máy chạy và cắt ra carcass thô theo đúng kích thước.
- Sau khi cắt, phần plywood còn lại (remnant) được ghi nhận. Nếu remnant đủ lớn để dùng cho SKU khác thì chuyển thành mã remnant riêng trong kho. Nếu không sử dụng được thì ghi nhận là hao hụt (waste).

Nghiệp vụ remnant - ví dụ thực tế từ khách hàng:

- Mua tấm plywood kích thước 1m x 2m (mã A).
- Cắt cho bàn hình chữ nhật, chỉ dùng phần 0.8m x 2m.
- Còn lại 0.2m x 2m - đủ để cắt mã B (size nhỏ hơn). Kho ghi nhận: remnant từ mã A, chuyển sang dùng cho mã B.
- Chi phí tấm plywood ban đầu sẽ được phân bổ giữa mã A và mã B (xem Giai đoạn 5).

> Note: Đây là nghiệp vụ core của hệ thống. Quản lý kho PHẢI nhập liệu remnant đúng quy trình thì costing mới chính xác.

### Giai đoạn 4 - Gia công thành phẩm

Carcass thô sau khi cắt được chuyển sang bộ phận gia công. Công nhân thực hiện các thao tác:

- Đổ keo, ghép các tấm.
- Khoan lỗ theo bản vẽ.
- Đánh bóng, mài bề mặt.
- Các công đoạn bổ sung khác tùy mã hàng.

Trong quá trình gia công, công nhân hoặc tổ trưởng BÁO cáo:

- Loại và số lượng vật tư phụ đã dùng (vd: "dùng 2 chai keo 502").
- Thời gian gia công (phục vụ tính chi phí nhân công).

Quản lý kho nhận báo cáo này và NHẬP liệu vào hệ thống. Kế toán KHÔNG tự nhập vật tư tiêu hao mà đọc từ dữ liệu kho.

### Giai đoạn 5 - Tính giá thành (Costing)

Kế toán thực hiện costing cho từng SKU sau khi có đầy đủ dữ liệu từ kho và sản xuất.

Công thức tổng quát:

```text
Unit Cost (SKU) = Chi phí nguyên liệu chính (phân bổ) + Chi phí vật tư phụ + Chi phí nhân công
```

Trong đó chi phí nguyên liệu chính được phân bổ như sau:

- Nếu 1 tấm plywood cho 1 SKU: 100% chi phí tấm đó gán cho SKU.
- Nếu 1 tấm plywood cho nhiều SKU (có remnant được tái dùng): chi phí tấm được chia theo tỷ lệ diện tích hoặc theo logic do kế toán định nghĩa.

Ví dụ số thực tế từ khách hàng:

| Khoản mục | Giá trị |
|---|---|
| Giá tấm plywood 1m x 2m | 80.000 đ |
| Chi phí nhân công | 20.000 đ |
| Vật tư phụ (keo, v.v.) | 1.000 đ |
| Tổng chi phí tạm (1 tấm -> 2 SKU) | 101.000 đ |
| Phân bổ cho SKU A (diện tích lớn hơn) | ~60.000 đ |
| Phân bổ cho SKU B (phần remnant) | ~41.000 đ |

> Note: Logic phân bổ chi phí remnant là điểm cần confirm với kế toán trước khi implement. Có thể theo diện tích, theo trọng số, hoặc rule custom.

### Giai đoạn 6 - Quản lý dòng tiền

Đây là giai đoạn cuối cùng của kế toán, tổng hợp toàn bộ PO:

- Giá bán (từ PO) - Giá thành (từ Costing) = Lợi nhuận gộp theo SKU.
- Tổng hợp theo PO, theo kỳ kế toán (tháng/quý).
- Theo dõi dòng tiền vào (thu từ xuất khẩu) và dòng tiền ra (mua nguyên liệu, nhân công, v.v.).

---

# 3. NGHIỆP VỤ KHO CHI TIẾT

## 3.1 Quản lý nhập kho nguyên liệu

Khi mua plywood về, kho ghi nhận:

- Mã tấm (board ID) - sinh tự động hoặc nhập theo mã nhà cung cấp.
- Kích thước gốc: chiều dài x chiều rộng (cm hoặc mm).
- Đơn giá (cost per sheet).
- Số lượng tấm.
- Ngày nhập, mã nhà cung cấp.

## 3.2 Quản lý xuất kho & cắt CNC

Khi xuất tấm plywood để đưa vào máy CNC, hệ thống ghi nhận lệnh xuất kho với các thông tin:

- Mã tấm plywood xuất.
- Mã Production Plan (biết tấm này cắt cho đợt nào).
- SKU đích (cắt cho mã hàng nào).
- Kích thước cắt (phần sử dụng cho SKU này).
- Phần còn lại sau cắt: kích thước remnant, trạng thái (reusable / waste).

> Note: Mỗi lần cắt phải tạo ra ít nhất 1 bản ghi usage cho SKU chính và 0-N bản ghi remnant. Hệ thống phải enforce nhập liệu đầy đủ trước khi cho phép hoàn thành lệnh cắt.

## 3.3 Quản lý Remnant

Remnant là nghiệp vụ đặc thù quan trọng nhất. Một tấm remnant sau khi được ghi nhận có các trạng thái:

| Trạng thái | Ý nghĩa | Hành động tiếp theo |
|---|---|---|
| AVAILABLE | Còn trong kho, chưa dùng | Có thể được chọn để cắt cho SKU khác |
| ALLOCATED | Đã được phân cho một lệnh sản xuất | Chờ thực tế cắt |
| CONSUMED | Đã cắt hết | Không còn tồn kho |
| WASTE | Không tái sử dụng được | Ghi hao hụt, tính vào chi phí của SKU gốc |

Quy tắc nghiệp vụ remnant:

- Khi kế hoạch sản xuất cần tìm nguyên liệu cho một SKU, hệ thống ưu tiên tìm remnant phù hợp kích thước trước khi xuất tấm mới (FIFO hoặc Best Fit - cần confirm với khách hàng).
- Remnant thuộc về tấm mẹ, phải lưu lineage: `remnant.parent_board_id` để costing truy vết được nguồn gốc.
- Một remnant có thể lại sinh ra remnant con (nested cutting) - hệ thống cần hỗ trợ đệ quy.

> Note: Best Fit: chọn tấm remnant nhỏ nhất đủ dùng để giảm lãng phí. FIFO: dùng tấm nhập trước. Chiến lược này ảnh hưởng đến costing nên cần xác nhận với kế toán.

## 3.4 Quản lý vật tư phụ

Vật tư phụ (keo, v.v.) được ghi nhận theo từng lệnh sản xuất (Work Order), không theo từng tấm plywood.

- Tổ trưởng sản xuất báo cáo: "mã SKU X, dùng 2 chai keo 502 và 0.5L keo Titebond".
- Quản lý kho nhập bản ghi tiêu hao (consumption record) vào hệ thống.
- Kế toán đọc dữ liệu này để tính vật tư phụ trong costing.

> Note: Hệ thống cần form nhập liệu đơn giản cho quản lý kho: chọn SKU -> chọn loại vật tư -> nhập số lượng -> xác nhận. Không nên bắt kho nhập chi phí tiền.

## 3.5 Quản lý vật tư đặc biệt - Metal

Bát sắt (metal bracket / support plate) là vật tư chỉ dùng cho một số mã hàng đặc thù (~10-15%).

- BOM của SKU phải có trường: `requires_metal = true/false`.
- Nếu `requires_metal = true`, Work Order tự động thêm metal vào danh sách vật tư cần xuất kho.
- Kho metal được quản lý riêng (tồn kho, nhập xuất) nhưng cùng logic với plywood.

---

# 4. BARCODE & TRACKING

## 4.1 Mục tiêu

Hệ thống barcode cho phép quét mã để lên hệ thống ngay lập tức, tương tự siêu thị. Mục tiêu là giảm nhập liệu thủ công và tăng tốc độ truy xuất trạng thái.

## 4.2 Thông tin trong barcode

Mỗi barcode đính kèm một carcass thành phẩm chứa các trường tối thiểu sau:

| Trường | Kiểu dữ liệu | Mô tả |
|---|---|---|
| sku_code | String | Mã SKU - định danh mã hàng |
| sku_name | String | Tên mã hàng (mô tả sản phẩm) |
| dimensions | String | Kích thước (vd: 120x80cm hoặc R=60cm) |
| production_plan_id | UUID | Mã đợt sản xuất để truy vết |
| po_id | UUID | Mã đơn hàng gốc |
| produced_date | Date | Ngày sản xuất |

> Note: Barcode content có thể encode dạng JSON mini hoặc chỉ encode `barcode_id` rồi lookup từ DB. Nên dùng QR Code thay vì barcode 1D để chứa đủ thông tin.

## 4.3 Điểm quét barcode trong quy trình

Enum `ScanCheckpoint` có 3 giá trị: `CNC_COMPLETE`, `FINISHED_GOODS`, `SHIPPED`.

- **Điểm 1 — `CNC_COMPLETE`**: Khi hoàn thành cắt CNC — scan để xác nhận carcass thô đã ra lò.
- **Điểm 2 — `FINISHED_GOODS`**: Khi hoàn thành gia công — scan để chuyển trạng thái thành phẩm.
- **Điểm 3 — `SHIPPED`**: Khi xuất kho đi đóng gói / giao hàng — scan để trừ tồn và gắn với PO.

---

# 5. ACTORS & PHÂN QUYỀN NGHIỆP VỤ

| Actor | Role (system) | Vai trò | Quyền hạn chính trong hệ thống |
|---|---|---|---|
| Kế toán | `accountant` | Nhập PO, costing, dòng tiền | CRUD PO, xem toàn bộ costing, export báo cáo tài chính |
| Bộ phận Kế hoạch | `planner` | Lập Production Plan | Tạo / duyệt Production Plan từ PO, xem tồn kho |
| Quản lý Kho | `warehouse` | Nhập xuất nguyên liệu, vật tư | CRUD giao dịch kho, nhập remnant, nhập tiêu hao vật tư phụ |
| Quản lý CNC (CNC Manager) | `cnc_manager` | Điều phối sản xuất CNC | Giao Work Order cho CNC, xem tiến độ toàn xưởng, ưu tiên lệnh cắt |
| Vận hành CNC | `cnc` | Chạy máy, báo kết quả cắt | Xem danh sách Work Order được giao, cập nhật trạng thái cắt, scan barcode (Kiosk) |
| Tổ trưởng SX | `foreman` | Quản lý gia công | Báo cáo tiêu hao vật tư phụ, cập nhật trạng thái Work Order (Dashboard) |
| Admin / Sếp | `admin` | Super-admin toàn hệ thống | Toàn quyền trên tất cả module, có quyền override mọi thao tác nghiệp vụ khi cần, xem dashboard tổng hợp |


> Note: `admin` là super-admin. Trong trường hợp cần xử lý khẩn cấp hoặc override vận hành, admin được phép thực hiện mọi thao tác ghi dữ liệu trên tất cả module. Các role chuyên trách vẫn là luồng vận hành mặc định hằng ngày.

## 5.1 Quy tắc Giao việc (Assignment)

- **Giao thủ công**: CNC Manager chọn Work Order và gán trực tiếp cho 1 tài khoản Vận hành CNC.
- **Giao tự động (Gợi ý)**: Hệ thống gợi ý gán cho CNC đang có ít Work Order ở trạng thái `IN_CUTTING` nhất.
- **Thông báo**: Khi có lệnh gán mới, hệ thống push notification tới thiết bị của CNC được gán.

---

# 6. CÁC ENTITY CHÍNH & QUAN HỆ

## 6.1 Danh sách Entity

| Entity | Mô tả | Quan hệ chính |
|---|---|---|
| PurchaseOrder (PO) | Đơn hàng từ khách nước ngoài | 1 PO -> N ProductionPlan, N POLineItem |
| POLineItem | Chi tiết từng SKU trong PO | N:1 PO, N:1 SKU |
| SKU | Mã hàng thành phẩm | 1 SKU -> 1 BOM, N WorkOrder |
| BOM (Bill of Materials) | Danh sách vật tư cần để làm 1 đơn vị SKU | 1 BOM -> N BOMComponent |
| BOMComponent | Từng vật tư trong BOM | N:1 BOM, N:1 Material |
| Material | Danh mục vật tư (plywood, keo, metal...) | 1 Material -> N InventoryLot |
| ProductionPlan | Đợt sản xuất | N:1 PO, 1 Plan -> N WorkOrder |
| WorkOrder | Lệnh sản xuất cho 1 SKU / đợt | N:1 ProductionPlan, N:1 SKU |
| InventoryLot | Lô nhập kho (batch nguyên liệu) | N:1 Material, 1 Lot -> N BoardSheet |
| BoardSheet | Từng tấm plywood cụ thể | N:1 InventoryLot, 1 Sheet -> N CuttingRecord |
| CuttingRecord | Bản ghi mỗi lần cắt trên tấm | N:1 BoardSheet, N:1 WorkOrder |
| Remnant | Phần tấm còn lại sau cắt | N:1 BoardSheet (parent), N:1 CuttingRecord |
| ConsumptionRecord | Bản ghi tiêu hao vật tư phụ | N:1 WorkOrder, N:1 Material |
| CostingRecord | Chi phí giá thành từng SKU theo đợt | N:1 WorkOrder, đọc từ Cutting + Consumption |
| Barcode | Mã barcode gắn với carcass thành phẩm | N:1 WorkOrder (1 WO có thể có nhiều barcode) |

## 6.2 Quan hệ đặc biệt cần lưu ý

- **BoardSheet -> Remnant**: quan hệ đệ quy. Remnant có thể tiếp tục sinh Remnant con. Cần lưu `parent_board_id` hoặc `parent_remnant_id`.
- **CostingRecord**: KHÔNG lưu logic tính toán mà đọc từ CuttingRecord và ConsumptionRecord. Kế toán xem aggregation, không nhập số liệu sản xuất.
- **WorkOrder** có trạng thái (status): `PLANNED -> IN_CUTTING -> IN_PROCESSING -> COMPLETED -> COSTED`.

> Note: Trạng thái WorkOrder là xương sống để hệ thống biết dữ liệu nào đã sẵn sàng cho bước tiếp theo. Kế toán chỉ được costing khi `WorkOrder.status = COMPLETED`.

---

# 7. QUY TẮC NGHIỆP VỤ CẦN IMPLEMENT (Business Rules)

## 7.1 Kho & Tồn liệu

- **BR-K01**: Không cho phép xuất vật tư khi số lượng tồn kho < số lượng yêu cầu. Trả lỗi 422 với message cụ thể.
- **BR-K02**: Mỗi lần cắt (CuttingRecord) bắt buộc nhập đầy đủ: kích thước dùng + kích thước remnant (hoặc flag = waste). Không cho phép để trống.
- **BR-K03**: Tổng diện tích (used + remnant + waste) phải <= diện tích tấm gốc. Hệ thống validate trước khi lưu.
- **BR-K04**: Remnant có trạng thái AVAILABLE mặc định khi tạo. Chỉ chuyển CONSUMED khi có CuttingRecord sử dụng nó. Chỉ chuyển WASTE khi người dùng xác nhận thủ công.
- **BR-K05**: Khi lập Production Plan, gợi ý remnant phù hợp (kích thước >= kích thước SKU cần). Không ép buộc dùng remnant nhưng ghi log nếu bỏ qua.

## 7.2 Sản xuất & Work Order

- **BR-P01**: Mỗi WorkOrder chỉ được phép chuyển sang trạng thái tiếp theo theo thứ tự: `PLANNED -> IN_CUTTING -> IN_PROCESSING -> COMPLETED`.
- **BR-P02**: Không cho phép tạo WorkOrder khi ProductionPlan chưa được duyệt (`status != APPROVED`).
- **BR-P03**: ConsumptionRecord (vật tư phụ) chỉ được nhập khi `WorkOrder.status = IN_PROCESSING` hoặc `COMPLETED`.
- **BR-P04**: Với SKU có `requires_metal = true`, Work Order phải có ít nhất 1 ConsumptionRecord với `material_type = METAL` trước khi chuyển COMPLETED.

## 7.3 Costing

- **BR-C01**: Kế toán chỉ được tạo CostingRecord khi `WorkOrder.status = COMPLETED`.
- **BR-C02**: Nếu 1 tấm plywood được dùng cho nhiều SKU (qua remnant), chi phí tấm phải được phân bổ. Hệ thống tự tính theo tỷ lệ diện tích sử dụng.
- **BR-C03**: Công thức phân bổ: `cost_for_sku = (area_used_by_sku / total_area_of_sheet) * sheet_cost`. Phần waste không phân bổ cho SKU nào mà ghi vào tài khoản hao hụt riêng.
- **BR-C04**: Sau khi CostingRecord được confirm, không cho phép sửa. Muốn sửa phải tạo CostingAdjustment riêng với lý do.

---

# 8. CÁC ĐIỂM CẦN XÁC NHẬN VỚI KHÁCH HÀNG

Các điểm sau chưa được làm rõ trong buổi trao đổi, cần confirm trước khi implement:

| # | Vấn đề | Tác động nếu không confirm |
|---|---|---|
| 1 | Chiến lược chọn remnant: FIFO hay Best Fit hay thủ công? | Ảnh hưởng trực tiếp đến thuật toán gợi ý kho và costing |
| 2 | Công thức phân bổ chi phí remnant: theo diện tích hay theo trọng số hay rule khác? | Logic tính giá thành mỗi SKU sẽ khác nhau |
| 3 | Cách tính chi phí nhân công: theo giờ, theo sản phẩm hay khoán tháng? | Cần biết để thiết kế bảng nhập liệu cho tổ trưởng |
| 4 | Barcode in khi nào: sau cắt CNC, sau gia công hay khi nhập kho thành phẩm? | Ảnh hưởng đến flow scan và điểm trigger in barcode |
| 5 | Hệ thống có cần quản lý nhà cung cấp (vendor) và đơn mua hàng (PO mua) không? | Ảnh hưởng đến scope module kho nhập |
| 6 | Quản lý công nhân và ca làm việc có thuộc scope không? | Ảnh hưởng đến cách tính chi phí nhân công |
| 7 | Xưởng đá (Stone) sẽ được tích hợp sau, dữ liệu có dùng chung entity SKU và PO không? | Ảnh hưởng đến thiết kế schema từ đầu |

---

# 9. TIMELINE & PHẠM VI PHIÊN BẢN 1

## 9.1 Deadline

Khách hàng yêu cầu phiên bản đầu tiên có thể sử dụng thực tế trong vòng 3-6 tháng kể từ ngày bắt đầu. Đây là thời điểm họ cần ổn định quy trình kho và sản xuất song song với việc tuyển nhân lực mới.

## 9.2 Scope MVP (Phiên bản 1)

- Module PO & Kế hoạch sản xuất.
- Module Kho: nhập xuất plywood, remnant tracking, vật tư phụ.
- Module Work Order & trạng thái sản xuất.
- Module Costing cơ bản (tính giá thành từng SKU).
- Barcode generation & scan.
- Dashboard tổng hợp cho sếp / kế toán.

## 9.3 Ngoài scope phiên bản 1

- Module Dòng tiền đầy đủ (P&L, cash flow report) - làm sau khi costing ổn định.
- Xưởng Đá (Stone module).
- Tích hợp với phần mềm kế toán bên ngoài (Misa, v.v.).
- Mobile app cho công nhân - phiên bản 1 dùng web responsive.

---

*Tài liệu này được tổng hợp từ buổi trao đổi thực tế với khách hàng và phản ánh hiểu biết hiện tại của nhóm phát triển. Cần review và cập nhật sau mỗi buổi clarification.*