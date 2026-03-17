# Vmarble Warehouse Management Service

Backend cho hệ thống quản lý kho và sản xuất carcass tại xưởng gỗ.

## Mục tiêu dự án

- Quản lý luồng từ đơn hàng (PO) đến sản xuất, nhập xuất kho và tính giá thành.
- Theo dõi remnant (vật tư dư) để tối ưu sử dụng ván ép và giảm hao hụt.
- Chuẩn hóa dữ liệu để kế toán có thể tính costing chính xác theo từng SKU.

## Phạm vi hiện tại (MVP)

- PO và kế hoạch sản xuất.
- Kho vật tư: plywood, vật tư phụ, metal.
- Work Order và trạng thái sản xuất.
- Costing cơ bản theo SKU.
- Barcode/QR tracking.

## Tài liệu nghiệp vụ

- Xem tài liệu chi tiết tại [docs/backend-business-logic-vi.md](docs/backend-business-logic-vi.md).

## Branch Rules

Quy tắc nhánh áp dụng chung:

### Nhánh `main` (production)

- Cấm push trực tiếp.
- Bắt buộc tạo Pull Request (PR) để merge.
- Bắt buộc có ít nhất 1 approve trước khi merge.
- Chặn force push.

### Nhánh `dev` (integration)

- Cấm push trực tiếp.
- Bắt buộc tạo PR để merge.
- Không yêu cầu approve (có thể tự review và merge để giữ tốc độ).
- Chặn force push.

## Quy trình làm việc nhanh

1. Tạo nhánh tính năng từ `dev`.
2. Hoàn thành code và push nhánh tính năng.
3. Tạo PR vào `dev` và merge khi đạt yêu cầu kiểm tra.
4. Khi đủ tính năng release, tạo PR từ `dev` sang `main`.
5. Có approve hợp lệ rồi mới merge vào `main`.
