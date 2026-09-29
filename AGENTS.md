# Backend API Specification - Smart Inventory & Asset Tracking System

Tài liệu này đặc tả toàn bộ nghiệp vụ, cơ sở dữ liệu (Database Schema), kiến trúc hệ thống và danh sách API Endpoints chi tiết để triển khai hệ thống Backend cho ứng dụng Smart Inventory & Asset Tracking.

---

## 1. Tổng quan hệ thống (System Overview)

Hệ thống quản lý kho & tài sản cá nhân / doanh nghiệp nhỏ bao gồm các phân hệ chính:
1. **Quản lý phân cấp không gian lưu trữ (Multi-level Storage Hierarchy / Boxes):** Cấu trúc cây đa cấp (Room → Cabinet → Shelf → Box → Drawer).
2. **Quản lý danh mục tài sản (Item Catalog & Lifecycle):** Theo dõi số lượng, đơn vị, giá trị, hình ảnh, hóa đơn, hạn sử dụng (perishables), bảo hành (warranties).
3. **Cảnh báo ngưỡng tồn kho riêng từng món (Item-specific Low Stock Threshold):** Mỗi mặt hàng được thiết lập ngưỡng cảnh báo thiếu hụt riêng (`min_quantity`).
4. **Theo dõi mượn / trả (Lending & Borrowing Tracker):** Quản lý trạng thái mượn đồ, người mượn, hạn trả, cảnh báo quá hạn.
5. **Trung tâm cảnh báo (Alerts & Health Dashboard):** Tổng hợp quá hạn mượn, hết hạn sử dụng, sắp hết hạn, sắp hết bảo hành, chạm ngưỡng tồn kho thấp.
6. **Mã vạch & QR Code (Barcode / QR Scanner):** Định danh vị trí và tra cứu nhanh đồ đạc qua mã vạch / QR.
7. **Báo cáo & Kiểm kê (Audit & Reports):** Thống kê giá trị tài sản, phân bổ trạng thái, xuất báo cáo kiểm kê.

---

## 2. Quy chuẩn chung (API Conventions)

- **Giao thức:** RESTful API qua HTTPS.
- **Định dạng dữ liệu:** `Content-Type: application/json`.
- **Mã hoá thời gian:** Chuỗi ISO 8601 UTC (ví dụ: `2026-09-29T13:30:00.000Z`).
- **Khóa chính (ID):** UUID v4.
- **Xác thực:** Bearer JWT Token gửi trong Header `Authorization: Bearer <token>`.
- **Định dạng phản hồi chuẩn (Response Envelope):**

```json
// Thành công
{
  "success": true,
  "data": { ... },
  "message": "Operation completed successfully",
  "meta": {
    "page": 1,
    "limit": 20,
    "total": 150
  }
}

// Thất bại
{
  "success": false,
  "error": {
    "code": "ITEM_NOT_FOUND",
    "message": "The requested item does not exist or has been deleted",
    "details": []
  }
}
```

---

## 3. Thiết kế Cơ sở dữ liệu (Database Schema)

### 3.1. Bảng `users` (Tài khoản người dùng)

| Trường | Kiểu dữ liệu | Ràng buộc | Diễn giải |
| :--- | :--- | :--- | :--- |
| `id` | UUID | PK, default uuid_generate_v4() | Khóa chính |
| `name` | VARCHAR(120) | NOT NULL | Tên người dùng |
| `email` | VARCHAR(255) | UNIQUE, NOT NULL | Email đăng nhập |
| `password_hash` | VARCHAR(255) | NOT NULL | Mật khẩu hash (bcrypt/argon2) |
| `role` | VARCHAR(50) | DEFAULT 'Owner' | Vai trò (Owner, Manager, Staff) |
| `avatar_url` | TEXT | NULL | Đường dẫn ảnh đại diện |
| `created_at` | TIMESTAMPTZ | DEFAULT NOW() | Thời điểm tạo |
| `updated_at` | TIMESTAMPTZ | DEFAULT NOW() | Thời điểm cập nhật |

### 3.2. Bảng `user_settings` (Cấu hình tùy chọn của người dùng)

| Trường | Kiểu dữ liệu | Ràng buộc | Diễn giải |
| :--- | :--- | :--- | :--- |
| `id` | UUID | PK, default uuid_generate_v4() | Khóa chính |
| `user_id` | UUID | FK -> `users.id`, UNIQUE | Liên kết tài khoản |
| `currency_symbol` | VARCHAR(10) | DEFAULT '$' | Ký hiệu tiền tệ ($, €, ₫, ¥...) |
| `default_unit` | VARCHAR(20) | DEFAULT 'pcs' | Đơn vị mặc định khi tạo item |
| `expiry_warning_days` | INT | DEFAULT 30 | Cảnh báo trước ngày hết hạn (ngày) |
| `notify_expiring` | BOOLEAN | DEFAULT TRUE | Bật/tắt cảnh báo sắp hết hạn |
| `notify_overdue` | BOOLEAN | DEFAULT TRUE | Bật/tắt cảnh báo mượn quá hạn |
| `notify_low_stock` | BOOLEAN | DEFAULT TRUE | Bật/tắt cảnh báo sắp hết hàng |
| `vibrate_on_scan` | BOOLEAN | DEFAULT TRUE | Rung khi quét barcode thành công |
| `auto_open_on_scan` | BOOLEAN | DEFAULT TRUE | Tự động mở chi tiết khi quét trúng |

### 3.3. Bảng `categories` (Danh mục vật phẩm)

| Trường | Kiểu dữ liệu | Ràng buộc | Diễn giải |
| :--- | :--- | :--- | :--- |
| `id` | UUID | PK, default uuid_generate_v4() | Khóa chính |
| `user_id` | UUID | FK -> `users.id` | Sở hữu bởi user |
| `name` | VARCHAR(100) | NOT NULL | Tên danh mục |
| `icon_name` | VARCHAR(50) | DEFAULT 'devices' | Tên icon đại diện |
| `is_default` | BOOLEAN | DEFAULT FALSE | Danh mục mẫu hệ thống |
| `created_at` | TIMESTAMPTZ | DEFAULT NOW() | |

*Danh mục mẫu mặc định:*
- `Electronics`
- `Clothing`
- `Tools & Hardware`
- `Kitchen & Dining`
- `Medicine & Health`
- `Documents & Books`
- `Gaming & Media`
- `Office & Stationery`
- `Personal & Misc`

### 3.4. Bảng `boxes` (Không gian lưu trữ / Thùng chứa)

Cấu trúc Self-Referencing cây phân cấp.

| Trường | Kiểu dữ liệu | Ràng buộc | Diễn giải |
| :--- | :--- | :--- | :--- |
| `id` | UUID | PK, default uuid_generate_v4() | Khóa chính |
| `user_id` | UUID | FK -> `users.id`, NOT NULL | Sở hữu bởi user |
| `parent_id` | UUID | FK -> `boxes.id`, NULL | ID thùng/vị trí cha (NULL = Root) |
| `name` | VARCHAR(150) | NOT NULL | Tên thùng/vị trí |
| `type` | VARCHAR(30) | NOT NULL | `room`, `cabinet`, `shelf`, `box`, `drawer`, `other` |
| `description` | TEXT | DEFAULT '' | Mô tả vị trí |
| `label` | VARCHAR(100) | UNIQUE, NOT NULL | Mã định danh QR / nhãn in |
| `icon` | VARCHAR(50) | NULL | Tên icon tuỳ chọn |
| `created_at` | TIMESTAMPTZ | DEFAULT NOW() | |
| `updated_at` | TIMESTAMPTZ | DEFAULT NOW() | |

### 3.5. Bảng `items` (Tài sản & Vật phẩm)

| Trường | Kiểu dữ liệu | Ràng buộc | Diễn giải |
| :--- | :--- | :--- | :--- |
| `id` | UUID | PK, default uuid_generate_v4() | Khóa chính |
| `user_id` | UUID | FK -> `users.id`, NOT NULL | Sở hữu bởi user |
| `box_id` | UUID | FK -> `boxes.id`, NOT NULL | Vị trí cất giữ hiện tại |
| `category_id` | VARCHAR(100) | NOT NULL | Danh mục phân loại |
| `name` | VARCHAR(255) | NOT NULL | Tên vật phẩm |
| `description` | TEXT | DEFAULT '' | Ghi chú / thông số chi tiết |
| `quantity` | NUMERIC(12, 2) | DEFAULT 1.0, NOT NULL | Số lượng hiện có |
| `unit` | VARCHAR(30) | DEFAULT 'pcs', NOT NULL | Đơn vị tính (`pcs`, `kg`, `set`...) |
| **`min_quantity`** | **NUMERIC(12, 2)** | **NULL** | **Ngưỡng tồn kho tối thiểu (Từng món riêng)** |
| `status` | VARCHAR(30) | DEFAULT 'stored' | `stored`, `inUse`, `lent`, `broken`, `disposed` |
| `purchase_price` | NUMERIC(15, 2) | NULL | Giá mua / giá trị tài sản |
| `purchase_date` | DATE | NULL | Ngày mua hàng |
| `serial_number` | VARCHAR(150) | NULL | Số sê-ri thiết bị |
| `barcode` | VARCHAR(150) | NULL | Mã vạch UPC/EAN/Code128 |
| `warranty_expiry_date`| DATE | NULL | Ngày hết hạn bảo hành |
| `expiry_date` | DATE | NULL | Hạn sử dụng (thuốc, thực phẩm, pin) |
| `photos` | JSONB | DEFAULT '[]'::jsonb | Danh sách URL ảnh sản phẩm |
| `receipt_photos` | JSONB | DEFAULT '[]'::jsonb | Danh sách URL ảnh hóa đơn/bảo hành |
| `custom_attributes` | JSONB | DEFAULT '{}'::jsonb | Cặp key-value linh hoạt (màu, cấu hình...) |
| `created_at` | TIMESTAMPTZ | DEFAULT NOW() | |
| `updated_at` | TIMESTAMPTZ | DEFAULT NOW() | |

> **Nguyên tắc Low Stock:**  
> Một Item được xác định là Low Stock khi:  
> `min_quantity IS NOT NULL AND quantity <= min_quantity`

### 3.6. Bảng `lending_records` (Lịch sử mượn & trả đồ)

| Trường | Kiểu dữ liệu | Ràng buộc | Diễn giải |
| :--- | :--- | :--- | :--- |
| `id` | UUID | PK, default uuid_generate_v4() | Khóa chính |
| `user_id` | UUID | FK -> `users.id`, NOT NULL | Người quản lý |
| `item_id` | UUID | FK -> `items.id`, NOT NULL | Vật phẩm được mượn |
| `borrower_name` | VARCHAR(150) | NOT NULL | Tên người mượn |
| `borrower_contact` | VARCHAR(150) | DEFAULT '' | SĐT / Email người mượn |
| `lent_date` | TIMESTAMPTZ | DEFAULT NOW() | Ngày xuất mượn |
| `expected_return_date`| TIMESTAMPTZ | NULL | Ngày hẹn trả |
| `actual_return_date` | TIMESTAMPTZ | NULL | Ngày thực tế đã trả (NULL = Đang mượn) |
| `notes` | TEXT | DEFAULT '' | Ghi chú thêm |
| `created_at` | TIMESTAMPTZ | DEFAULT NOW() | |

---

## 4. Danh sách API Endpoints chi tiết

### 4.1. Authentication (Xác thực)

#### `POST /api/v1/auth/register`
Đăng ký tài khoản người dùng mới.
- **Request Body:**
  ```json
  {
    "name": "Sara Wilson",
    "email": "sara.wilson@example.com",
    "password": "SecurePassword123!"
  }
  ```
- **Response 201:** Trả về access token và thông tin profile.

#### `POST /api/v1/auth/login`
Đăng nhập hệ thống.
- **Request Body:**
  ```json
  {
    "email": "sara.wilson@example.com",
    "password": "SecurePassword123!"
  }
  ```
- **Response 200:**
  ```json
  {
    "success": true,
    "data": {
      "token": "eyJhbGciOiJIUzI1Ni...",
      "user": {
        "id": "c8699bf3-8ea7-4228-a3cb-465451e06fa5",
        "name": "Sara Wilson",
        "email": "sara.wilson@example.com",
        "role": "Owner"
      }
    }
  }
  ```

#### `GET /api/v1/auth/me`
Lấy thông tin người dùng hiện tại từ Token.

---

### 4.2. Profile & Settings (Thông tin cá nhân & Cấu hình)

#### `GET /api/v1/settings`
Lấy toàn bộ cấu hình hiển thị và tùy chọn cảnh báo của người dùng.
- **Response 200:**
  ```json
  {
    "success": true,
    "data": {
      "currencySymbol": "$",
      "defaultUnit": "pcs",
      "expiryWarningDays": 30,
      "notifyExpiring": true,
      "notifyOverdue": true,
      "notifyLowStock": true,
      "vibrateOnScan": true,
      "autoOpenOnScan": true
    }
  }
  ```

#### `PATCH /api/v1/settings`
Cập nhật tùy chọn người dùng.
- **Request Body:**
  ```json
  {
    "currencySymbol": "₫",
    "defaultUnit": "cái",
    "expiryWarningDays": 15,
    "notifyLowStock": true
  }
  ```

#### `PATCH /api/v1/user/profile`
Cập nhật thông tin tài khoản (Tên, vai trò, avatar).
- **Request Body:**
  ```json
  {
    "name": "Sara Wilson",
    "role": "Inventory Lead",
    "avatarUrl": "https://cdn.example.com/avatars/sara.png"
  }
  ```

---

### 4.3. Categories (Danh mục vật phẩm)

#### `GET /api/v1/categories`
Lấy danh sách tất cả danh mục (bao gồm danh mục mặc định và do người dùng tạo).
- **Response 200:**
  ```json
  {
    "success": true,
    "data": [
      { "id": "cat_1", "name": "Electronics", "isDefault": true },
      { "id": "cat_2", "name": "Tools & Hardware", "isDefault": true },
      { "id": "cat_3", "name": "Camping Gear", "isDefault": false }
    ]
  }
  ```

#### `POST /api/v1/categories`
Tạo danh mục phân loại mới.
- **Request Body:**
  ```json
  {
    "name": "Camping Gear"
  }
  ```

#### `DELETE /api/v1/categories/:name`
Xóa danh mục tự tạo (không cho phép xóa danh mục hệ thống).

---

### 4.4. Storage Spaces / Boxes (Khu vực & Thùng lưu trữ)

#### `GET /api/v1/boxes`
Lấy danh sách tất cả các thùng/vị trí lưu trữ. Hỗ trợ query theo cây phân cấp.
- **Query Params:**
  - `parentId`: UUID hoặc `root` để lấy các tầng cao nhất (Rooms / Floors).
  - `tree`: `true` để lấy cấu trúc phân nhánh lồng nhau.
- **Response 200:**
  ```json
  {
    "success": true,
    "data": [
      {
        "id": "b1b017f8-1111-4234-8c81-86a073f47262",
        "name": "1st Floor",
        "type": "room",
        "description": "Main ground level",
        "label": "loc_floor_1",
        "parentId": null,
        "itemCount": 14,
        "subBoxCount": 3,
        "createdAt": "2026-09-29T10:00:00.000Z"
      }
    ]
  }
  ```

#### `POST /api/v1/boxes`
Tạo một vị trí hoặc thùng chứa mới.
- **Request Body:**
  ```json
  {
    "name": "Living Room Shelf A",
    "type": "shelf",
    "description": "Top wooden shelf near television",
    "parentId": "b1b017f8-1111-4234-8c81-86a073f47262",
    "label": "shelf_living_01"
  }
  ```

#### `GET /api/v1/boxes/:id`
Lấy chi tiết thùng chứa kèm Breadcrumb Trail và danh sách vật phẩm bên trong.
- **Response 200:**
  ```json
  {
    "success": true,
    "data": {
      "box": {
        "id": "box_elect_01",
        "name": "Electronics Storage Box",
        "type": "box",
        "description": "Cables, adapters and accessories",
        "label": "box_elect_01",
        "parentId": "shelf_living_01"
      },
      "breadcrumbs": [
        { "id": "loc_floor_1", "name": "1st Floor" },
        { "id": "loc_living", "name": "Living Room" },
        { "id": "shelf_living_01", "name": "Living Room Shelf A" },
        { "id": "box_elect_01", "name": "Electronics Storage Box" }
      ],
      "subBoxes": [],
      "items": [
        {
          "id": "item_123",
          "name": "65W GaN Dual USB-C Charger",
          "quantity": 2.0,
          "unit": "pcs",
          "minQuantity": 3.0,
          "isLowStock": true,
          "status": "stored"
        }
      ]
    }
  }
  ```

#### `PUT /api/v1/boxes/:id`
Chỉnh sửa tên, phân loại, mô tả của thùng chứa.

#### `PATCH /api/v1/boxes/:id/move`
Di chuyển thùng chứa sang một vị trí cha khác (hỗ trợ kéo thả / tổ chức lại).
- **Request Body:**
  ```json
  {
    "newParentId": "loc_office_floor_2"
  }
  ```

#### `DELETE /api/v1/boxes/:id`
Xóa thùng chứa.
- **Query Params:**
  - `cascade`: `true` nếu xóa toàn bộ đồ bên trong, `false` để chặn nếu thùng còn đồ.

---

### 4.5. Items / Asset Catalog (Vật phẩm & Tài sản)

#### `GET /api/v1/items`
Tìm kiếm và lọc danh sách tài sản.
- **Query Params:**
  - `q`: Từ khóa tìm kiếm (tên, mô tả, barcode, serial number).
  - `boxId`: Lọc theo vị trí lưu trữ.
  - `category`: Lọc theo danh mục (Electronics, Tools...).
  - `status`: Lọc theo trạng thái (`stored`, `inUse`, `lent`, `broken`, `disposed`).
  - `isLowStock`: `true` để lọc ra danh sách sắp hết hàng (`quantity <= min_quantity`).
  - `isExpired`: `true` để lấy các món hết hạn.
  - `isExpiring`: `true` để lấy các món sắp hết hạn trong N ngày.
  - `page`: Số trang (mặc định 1).
  - `limit`: Số item trên trang (mặc định 20).
- **Response 200:**
  ```json
  {
    "success": true,
    "data": [
      {
        "id": "e72ff0a4-39f5-4927-920f-0746e5ca9399",
        "name": "65W GaN Dual USB-C Charger",
        "description": "Compact fast charger for laptop and mobile devices",
        "boxId": "box_elect_01",
        "boxName": "Electronics Storage Box",
        "categoryId": "Electronics",
        "quantity": 2.0,
        "unit": "pcs",
        "minQuantity": 3.0,
        "isLowStock": true,
        "status": "stored",
        "purchasePrice": 42.00,
        "purchaseDate": "2026-07-31",
        "warrantyExpiryDate": "2027-07-31",
        "expiryDate": null,
        "serialNumber": null,
        "barcode": "6934177712345",
        "photos": ["https://images.unsplash.com/..."],
        "receiptPhotos": [],
        "customAttributes": { "Max Output": "65W", "Ports": "2x USB-C" },
        "createdAt": "2026-07-31T08:00:00.000Z"
      }
    ],
    "meta": { "total": 45, "page": 1, "limit": 20 }
  }
  ```

#### `POST /api/v1/items`
Tạo một vật phẩm mới trong kho.
- **Request Body:**
  ```json
  {
    "name": "Neosporin First Aid Antibiotic",
    "description": "Triple antibiotic protection ointment 1oz",
    "boxId": "box_med_01",
    "categoryId": "Medicine",
    "quantity": 2.0,
    "unit": "tubes",
    "minQuantity": 4.0,
    "purchasePrice": 9.99,
    "purchaseDate": "2025-10-15",
    "expiryDate": "2026-10-11",
    "barcode": "300810733877",
    "status": "stored",
    "photos": ["https://images.unsplash.com/..."],
    "customAttributes": {
      "Dosage": "Topical",
      "Volume": "1 oz"
    }
  }
  ```

#### `GET /api/v1/items/:id`
Lấy chi tiết toàn bộ thông số của vật phẩm, breadcrumb vị trí, thông tin bảo hành, bản ghi mượn đồ hiện tại (nếu có) và lịch sử mượn trả.

#### `PUT /api/v1/items/:id`
Cập nhật thông tin vật phẩm (hỗ trợ chỉnh sửa số lượng, ngưỡng `minQuantity`, giá, ảnh...).

#### `PATCH /api/v1/items/:id/move`
Chuyển vật phẩm sang thùng/vị trí lưu trữ mới.
- **Request Body:**
  ```json
  {
    "targetBoxId": "box_med_cabinet_2"
  }
  ```

#### `PATCH /api/v1/items/:id/quantity`
Cập nhật nhanh số lượng (tăng/giảm kho khi sử dụng hoặc nhập thêm).
- **Request Body:**
  ```json
  {
    "quantity": 5.0
  }
  ```

#### `DELETE /api/v1/items/:id`
Xóa vật phẩm khỏi hệ thống.

---

### 4.6. Lending & Borrowing Tracker (Quản lý Mượn / Trả)

#### `POST /api/v1/lending/lend`
Xuất mượn một vật phẩm.
- **Nghiệp vụ:** 
  1. Kiểm tra vật phẩm có đang ở trạng thái `stored` hoặc `inUse` không.
  2. Tạo bản ghi `lending_records`.
  3. Cập nhật `item.status = 'lent'`.
- **Request Body:**
  ```json
  {
    "itemId": "item_ps5_controller",
    "borrowerName": "Alex Johnson",
    "borrowerContact": "+1 (555) 234-5678",
    "lentDate": "2026-09-20T10:00:00.000Z",
    "expectedReturnDate": "2026-09-27T18:00:00.000Z",
    "notes": "Lent for weekend game tournament"
  }
  ```

#### `POST /api/v1/lending/:id/return`
Đánh dấu đã hoàn trả vật phẩm.
- **Nghiệp vụ:**
  1. Cập nhật `actual_return_date = NOW()`.
  2. Cập nhật `item.status = 'stored'`.
- **Response 200:**
  ```json
  {
    "success": true,
    "message": "Item returned successfully"
  }
  ```

#### `GET /api/v1/lending/active`
Lấy danh sách các tài sản đang cho mượn (`actual_return_date IS NULL`).

#### `GET /api/v1/lending/overdue`
Lấy danh sách các tài sản mượn đã quá hạn trả (`actual_return_date IS NULL AND expected_return_date < NOW()`).

#### `GET /api/v1/lending/history?itemId=...`
Lấy toàn bộ lịch sử các lần mượn trả của một món đồ.

---

### 4.7. Barcode & QR Code Scanning (Quét mã)

#### `GET /api/v1/scan/lookup`
Tra cứu thông minh mã quét từ camera hoặc máy đọc barcode.
- **Query Params:**
  - `code`: Chuỗi barcode hoặc nội dung QR vừa quét (VD: `loc_floor_1`, `6934177712345`, `SN-MBP14-M3-9821`).
- **Nghiệp vụ:**
  1. Đối soát bảng `boxes` qua trường `label`. Nếu khớp -> trả về type `box`.
  2. Đối soát bảng `items` qua trường `barcode`, `serial_number`, hoặc `id`. Nếu khớp -> trả về type `item`.
  3. Nếu không tìm thấy -> trả về `NOT_FOUND` kèm gợi ý tạo mới.
- **Response 200 (Khớp Box):**
  ```json
  {
    "success": true,
    "data": {
      "type": "box",
      "payload": {
        "id": "box_game_01",
        "name": "Gaming Accessories Drawer",
        "type": "drawer",
        "itemCount": 5
      }
    }
  }
  ```
- **Response 200 (Khớp Item):**
  ```json
  {
    "success": true,
    "data": {
      "type": "item",
      "payload": {
        "id": "item_charger_65w",
        "name": "65W GaN Dual USB-C Charger",
        "boxId": "box_elect_01",
        "quantity": 2.0,
        "isLowStock": true
      }
    }
  }
  ```

---

### 4.8. Alerts & Dashboard (Cảnh báo & Sức khỏe kho)

#### `GET /api/v1/alerts/summary`
Lấy số đếm tổng hợp cho các huy hiệu cảnh báo (Badge count) trên màn hình chính.
- **Response 200:**
  ```json
  {
    "success": true,
    "data": {
      "totalAlertCount": 5,
      "expiredCount": 1,
      "expiringSoonCount": 1,
      "warrantyEndingCount": 0,
      "overdueLoansCount": 1,
      "lowStockCount": 2
    }
  }
  ```

#### `GET /api/v1/alerts/all`
Lấy chi tiết danh sách tất cả các nhóm cần hành động (được hiển thị trên màn hình [AlertsScreen](file:///Users/namtruong/WorkSpace/Project/Inventory/lib/screens/alerts_screen.dart)):
1. `expiredItems`: Mặt hàng hết hạn dùng.
2. `expiringSoonItems`: Mặt hàng sắp hết hạn (trong vòng `expiryWarningDays` ngày).
3. `expiringWarranties`: Hết hạn bảo hành sắp tới.
4. `overdueLoans`: Khoản mượn quá hạn hẹn trả.
5. `lowStockItems`: Mặt hàng chạm hoặc dưới ngưỡng `min_quantity` riêng biệt.

---

### 4.9. Reports & Inventory Audit (Báo cáo & Kiểm kê)

#### `GET /api/v1/reports/inventory-health`
Cung cấp dữ liệu cho Bottom Sheet *Inventory Health Breakdown*.
- **Response 200:**
  ```json
  {
    "success": true,
    "data": {
      "totalAssetsCount": 12,
      "totalEstimatedValue": 4350.50,
      "breakdown": {
        "stored": 9,
        "inUse": 2,
        "lent": 1,
        "lowStock": 2,
        "expired": 1,
        "expiringSoon": 1
      },
      "storageLocationsCount": 11
    }
  }
  ```

#### `GET /api/v1/reports/export`
Xuất file báo cáo kiểm kê tài sản (định dạng CSV / JSON / PDF) để sao lưu hoặc kiểm kê định kỳ.

---

### 4.10. Media Upload (Tải lên hình ảnh)

#### `POST /api/v1/media/upload`
Tải lên ảnh chụp thực tế đồ đạc hoặc ảnh hóa đơn mua hàng.
- **Content-Type:** `multipart/form-data`
- **Body:**
  - `file`: File nhị phân (jpg, png, webp, heic).
  - `folder`: `items` hoặc `receipts`.
- **Response 200:**
  ```json
  {
    "success": true,
    "data": {
      "url": "https://storage.example.com/inventory/items/img_987213.webp"
    }
  }
  ```

---

## 5. Quy tắc nghiệp vụ bắt buộc (Business Logic Constraints)

1. **Ngưỡng tồn kho tối thiểu (`min_quantity`):**
   - Đây là trường tùy chọn trên từng món đồ, không bắt buộc.
   - Nếu `min_quantity IS NULL`: Món đồ không áp dụng theo dõi tồn kho thiếu hụt.
   - Nếu `min_quantity IS NOT NULL`: Khi `quantity <= min_quantity`, hệ thống tự động gắn nhãn `isLowStock = true` và đưa vào danh sách cảnh báo thiếu hụt.

2. **Chống lặp vòng cây vị trí (Cycle Prevention):**
   - Khi cập nhật `parent_id` của một `Box`, hệ thống phải kiểm tra đảm bảo `parent_id` mới không được trùng với chính nó và không phải là con cháu cấp dưới của nó.

3. **Toàn vẹn trạng thái mượn đồ (Lending Integrity):**
   - Không cho phép xuất mượn món đồ đang có trạng thái `lent`, `broken`, hoặc `disposed`.
   - Khi trả đồ, trạng thái phải được trả lại là `stored` hoặc `inUse`.

4. **Định danh mã vạch & QR Code:**
   - Trường `label` của bảng `boxes` là duy nhất trên phạm vi toàn hệ thống hoặc người dùng để in tem QR dán lên hộp đựng.
   - Trường `barcode` của `items` hỗ trợ các định dạng mã vạch thương mại chuẩn (UPC-A, EAN-13, Code 128).

---

## 6. Hướng dẫn công nghệ Backend khuyến nghị

- **Ngôn ngữ / Framework gợi ý:**
  - **Node.js (NestJS / Express + TypeScript + Prisma ORM)**: Thích hợp với việc phát triển nhanh, type-safe hoàn chỉnh đồng bộ với Frontend Flutter/Dart.
  - **Go (Gin / Fiber + GORM / sqlx)**: Hiệu năng cao, tốn ít tài nguyên, dễ đóng gói single binary.
  - **Python (FastAPI + SQLAlchemy / Pydantic)**: Xử lý dữ liệu và tích hợp AI/OCR nhận diện hóa đơn rất mạnh.
- **Cơ sở dữ liệu:** PostgreSQL 15+ (Hỗ trợ JSONB, CTE đệ quy tra cứu cây phân cấp vị trí `WITH RECURSIVE`).
- **File Storage:** AWS S3 / Cloudflare R2 / MinIO cho lưu trữ ảnh vật phẩm và hóa đơn.
