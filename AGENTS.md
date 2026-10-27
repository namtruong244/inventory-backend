# Backend API Specification - Smart Inventory & Asset Tracking System

Tài liệu này đặc tả toàn bộ nghiệp vụ, cơ sở dữ liệu (Database Schema & PostgreSQL DDL Migrations), kiến trúc hệ thống, danh sách API Endpoints chi tiết, và các mô hình DTOs (Golang Structs) để triển khai và bảo trì hệ thống Backend cho ứng dụng Smart Inventory & Asset Tracking.

---

## 1. Tổng quan hệ thống (System Overview)

Hệ thống quản lý kho & tài sản cá nhân / doanh nghiệp nhỏ bao gồm các phân hệ chính:
1. **Xác thực tài khoản & Mạng xã hội (Authentication & Social Login):** Đăng ký/đăng nhập truyền thống qua Email/Password, hỗ trợ đăng nhập nhanh bằng Google (Gmail).
2. **Quản lý hồ sơ & Cài đặt (Profile & User Settings):** Cập nhật tên, chức vụ/vai trò (role), ảnh đại diện (avatar), cấu hình tiền tệ, đơn vị tính mặc định, và các tùy chọn bật/tắt cảnh báo.
3. **Quản lý phân cấp không gian lưu trữ (Multi-level Storage Hierarchy / Boxes):** Cấu trúc cây đa cấp linh hoạt (Room → Cabinet → Shelf → Box → Drawer). Hỗ trợ điều chuyển vị trí cha (move box) với cơ chế kiểm tra chống lặp vòng (Cycle Prevention).
4. **Quản lý danh mục & Thuộc tính động (Item Catalog, Specifications & Custom Attributes):** Theo dõi số lượng, đơn vị, giá trị, hình ảnh, hóa đơn, hạn sử dụng (perishables), bảo hành (warranties), kèm bảng thông số kỹ thuật động (Color, RAM, Storage, Dimension...) dạng key-value.
5. **Cập nhật kho nhanh (Quick Quantity Update & Transfer):** Tăng/giảm số lượng tồn kho nhanh chóng và điều chuyển tài sản giữa các kho/thùng chứa.
6. **Cảnh báo ngưỡng tồn kho riêng từng món (Item-specific Low Stock Threshold):** Mỗi mặt hàng được thiết lập ngưỡng cảnh báo thiếu hụt riêng (`min_quantity`). Tự động phát hiện khi `quantity + lentQuantity <= min_quantity`.
7. **Theo dõi mượn / trả (Lending & Borrowing Tracker):** Quản lý trạng thái mượn đồ, thông tin người mượn, hạn trả, cảnh báo quá hạn.
8. **Mã vạch & QR Code (Barcode / QR Scanner):** Định danh vị trí lưu trữ (Box label) và tra cứu thông minh đồ đạc qua mã vạch sản phẩm (UPC/EAN/Code128) hoặc serial number.
9. **Trung tâm cảnh báo (Alerts & Health Dashboard):** Tổng hợp quá hạn mượn, hết hạn sử dụng, sắp hết hạn, sắp hết bảo hành, chạm ngưỡng tồn kho thấp.
10. **Báo cáo & Kiểm kê (Audit & Reports):** Thống kê tổng giá trị tài sản, phân bổ trạng thái lưu trữ, xuất báo cáo kiểm kê định kỳ (CSV/JSON/PDF).
11. **Tải lên tệp đa phương tiện (Media Upload):** Hỗ trợ lưu trữ ảnh chụp đồ đạc và hóa đơn mua hàng (Local / Cloudflare R2 / AWS S3 / MinIO).

---

## 2. Quy chuẩn chung (API Conventions)

- **Base URL:** `http://localhost:8080` (hoặc cấu hình tùy ý từ màn hình Cài đặt của App).
- **Giao thức:** RESTful API qua HTTP / HTTPS.
- **Header chuẩn bắt buộc:**
  ```http
  Accept: application/json
  Content-Type: application/json
  Authorization: Bearer <token> (Cho các API yêu cầu xác thực)
  ```
- **Mã hoá thời gian:** Chuỗi ISO 8601 UTC (ví dụ: `2026-10-01T14:30:00.000Z`).
- **Khóa chính (ID):** UUID v4 chuẩn (chuỗi 36 ký tự).
- **Định dạng phản hồi chuẩn (Response Envelope):**

```json
// Phản hồi Thành công (Status 200, 201)
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

// Phản hồi Thất bại (Status 400, 401, 403, 404, 409, 500)
{
  "success": false,
  "error": {
    "code": "INVALID_CREDENTIALS",
    "message": "Invalid email or password",
    "details": []
  }
}
```

- **Mã lỗi theo loại:** dữ liệu không hợp lệ → 400 (`INVALID_INPUT` hoặc `*_FAILED`); không tìm thấy / không thuộc user → 404 `NOT_FOUND`; xung đột (trùng `label`, item đang mượn, cập nhật đồng thời, box còn nội dung khi `cascade=false`) → 409 `CONFLICT`; lỗi không lường trước → 500 `INTERNAL_ERROR` (message chung, chi tiết chỉ ghi log).
- **Ngày của item** (`purchaseDate`, `warrantyExpiryDate`, `expiryDate`): server chuẩn hoá về `YYYY-MM-DD` (nhận cả ISO 8601 đầy đủ). Client nên gửi đúng ngày lịch người dùng chọn.

---

## 3. Thiết kế Cơ sở dữ liệu (Database Schema & DDL Migration)

### 3.1. Chi tiết các bảng dữ liệu

#### 3.1.1. Bảng `users` (Tài khoản người dùng)
Hỗ trợ đăng nhập chuẩn (Email/Password) và Đăng nhập mạng xã hội (Google).

| Trường | Kiểu dữ liệu | Ràng buộc | Diễn giải |
| :--- | :--- | :--- | :--- |
| `id` | UUID | PK, default uuid_generate_v4() | Khóa chính |
| `name` | VARCHAR(150) | NOT NULL | Tên người dùng |
| `email` | VARCHAR(255) | UNIQUE, NOT NULL | Email đăng nhập |
| `password_hash` | VARCHAR(255) | NULL | Mật khẩu hash (Cho phép NULL nếu đăng nhập qua Google) |
| `provider` | VARCHAR(50) | DEFAULT 'local' | Nhà cung cấp auth (`local`, `google`, `guest`) |
| `provider_id` | VARCHAR(255) | NULL | Mã định danh Google Sub |
| `role` | VARCHAR(50) | DEFAULT 'Owner' | Vai trò (`Owner`, `Manager`, `Staff`) |
| `avatar_url` | TEXT | NULL | Đường dẫn ảnh đại diện |
| `created_at` | TIMESTAMPTZ | DEFAULT NOW() | Thời điểm tạo |
| `updated_at` | TIMESTAMPTZ | DEFAULT NOW() | Thời điểm cập nhật |

#### 3.1.2. Bảng `user_settings` (Cấu hình tùy chọn của người dùng)

| Trường | Kiểu dữ liệu | Ràng buộc | Diễn giải |
| :--- | :--- | :--- | :--- |
| `id` | UUID | PK, default uuid_generate_v4() | Khóa chính |
| `user_id` | UUID | FK -> `users.id`, UNIQUE, ON DELETE CASCADE | Liên kết tài khoản |
| `currency_symbol` | VARCHAR(10) | DEFAULT '$' | Ký hiệu tiền tệ ($, €, ₫, ¥...) |
| `default_unit` | VARCHAR(30) | DEFAULT 'pcs' | Đơn vị mặc định khi tạo item |
| `expiry_warning_days` | INT | DEFAULT 30 | Cảnh báo trước ngày hết hạn (ngày) |
| `notify_expiring` | BOOLEAN | DEFAULT TRUE | Bật/tắt cảnh báo sắp hết hạn |
| `notify_overdue` | BOOLEAN | DEFAULT TRUE | Bật/tắt cảnh báo mượn quá hạn |
| `notify_low_stock` | BOOLEAN | DEFAULT TRUE | Bật/tắt cảnh báo sắp hết hàng |
| `vibrate_on_scan` | BOOLEAN | DEFAULT TRUE | Rung khi quét barcode thành công |
| `auto_open_on_scan` | BOOLEAN | DEFAULT TRUE | Tự động mở chi tiết khi quét trúng |
| `updated_at` | TIMESTAMPTZ | DEFAULT NOW() | Thời điểm cập nhật |

#### 3.1.3. Bảng `categories` (Danh mục vật phẩm)

| Trường | Kiểu dữ liệu | Ràng buộc | Diễn giải |
| :--- | :--- | :--- | :--- |
| `id` | UUID | PK, default uuid_generate_v4() | Khóa chính |
| `user_id` | UUID | FK -> `users.id`, ON DELETE CASCADE | Sở hữu bởi user |
| `name` | VARCHAR(100) | NOT NULL | Tên danh mục |
| `icon_name` | VARCHAR(50) | DEFAULT 'devices' | Tên icon đại diện |
| `is_default` | BOOLEAN | DEFAULT FALSE | Danh mục mẫu hệ thống |
| `created_at` | TIMESTAMPTZ | DEFAULT NOW() | |

*Danh mục mẫu mặc định:*
`Electronics`, `Clothing`, `Tools & Hardware`, `Kitchen & Dining`, `Medicine & Health`, `Documents & Books`, `Gaming & Media`, `Office & Stationery`, `Personal & Misc`.

#### 3.1.4. Bảng `boxes` (Không gian lưu trữ / Thùng chứa)
Cấu trúc Self-Referencing cây phân cấp đa tầng.

| Trường | Kiểu dữ liệu | Ràng buộc | Diễn giải |
| :--- | :--- | :--- | :--- |
| `id` | UUID | PK, default uuid_generate_v4() | Khóa chính |
| `user_id` | UUID | FK -> `users.id`, ON DELETE CASCADE | Sở hữu bởi user |
| `parent_id` | UUID | FK -> `boxes.id`, NULL, ON DELETE SET NULL | ID thùng/vị trí cha (NULL = Root) |
| `name` | VARCHAR(150) | NOT NULL | Tên thùng/vị trí |
| `type` | VARCHAR(30) | NOT NULL DEFAULT 'box' | `room`, `cabinet`, `shelf`, `box`, `drawer`, `other` |
| `description` | TEXT | DEFAULT '' | Mô tả vị trí |
| `label` | VARCHAR(100) | NOT NULL | Mã định danh QR / nhãn in |
| `icon` | VARCHAR(50) | NULL | Tên icon tuỳ chọn |
| `created_at` | TIMESTAMPTZ | DEFAULT NOW() | |
| `updated_at` | TIMESTAMPTZ | DEFAULT NOW() | |

#### 3.1.5. Bảng `items` (Tài sản & Vật phẩm)

| Trường | Kiểu dữ liệu | Ràng buộc | Diễn giải |
| :--- | :--- | :--- | :--- |
| `id` | UUID | PK, default uuid_generate_v4() | Khóa chính |
| `user_id` | UUID | FK -> `users.id`, ON DELETE CASCADE | Sở hữu bởi user |
| `box_id` | UUID | FK -> `boxes.id`, ON DELETE RESTRICT | Vị trí cất giữ hiện tại |
| `category_id` | VARCHAR(100) | NOT NULL DEFAULT 'General' | Danh mục phân loại |
| `name` | VARCHAR(255) | NOT NULL | Tên vật phẩm |
| `description` | TEXT | DEFAULT '' | Ghi chú / thông số chi tiết |
| `quantity` | NUMERIC(12, 2) | DEFAULT 1.0, NOT NULL | Số lượng hiện có |
| `unit` | VARCHAR(30) | DEFAULT 'pcs', NOT NULL | Đơn vị tính (`pcs`, `kg`, `set`...) |
| **`min_quantity`** | **NUMERIC(12, 2)** | **NULL** | **Ngưỡng tồn kho tối thiểu riêng từng món** |
| `status` | VARCHAR(30) | DEFAULT 'stored' | `stored`, `inUse`, `lent`, `broken`, `disposed` |
| `purchase_price` | NUMERIC(15, 2) | NULL | Giá mua / giá trị tài sản |
| `purchase_date` | TIMESTAMPTZ | NULL | Ngày mua hàng |
| `warranty_expiry_date`| TIMESTAMPTZ | NULL | Ngày hết hạn bảo hành |
| `expiry_date` | TIMESTAMPTZ | NULL | Hạn sử dụng (thuốc, thực phẩm, pin...) |
| `serial_number` | VARCHAR(150) | NULL | Số sê-ri thiết bị |
| `barcode` | VARCHAR(150) | NULL | Mã vạch UPC/EAN/Code128 |
| `photos` | JSONB | DEFAULT '[]'::jsonb | Danh sách URL ảnh sản phẩm |
| `receipt_photos` | JSONB | DEFAULT '[]'::jsonb | Danh sách URL ảnh hóa đơn/bảo hành |
| **`custom_attributes`** | **JSONB** | **DEFAULT '{}'::jsonb** | **Thông số kỹ thuật động (Specifications & Attributes)** |
| `created_at` | TIMESTAMPTZ | DEFAULT NOW() | |
| `updated_at` | TIMESTAMPTZ | DEFAULT NOW() | |

> **Nguyên tắc Low Stock:**  
> Một Item được xác định là Low Stock khi: `min_quantity IS NOT NULL AND quantity + lentQuantity <= min_quantity` (`lentQuantity`: đơn vị đang cho mượn một phần).

#### 3.1.6. Bảng `lending_records` (Lịch sử mượn & trả đồ)

| Trường | Kiểu dữ liệu | Ràng buộc | Diễn giải |
| :--- | :--- | :--- | :--- |
| `id` | UUID | PK, default uuid_generate_v4() | Khóa chính |
| `user_id` | UUID | FK -> `users.id`, ON DELETE CASCADE | Người quản lý |
| `item_id` | UUID | FK -> `items.id`, ON DELETE CASCADE | Vật phẩm được mượn |
| `borrower_name` | VARCHAR(150) | NOT NULL | Tên người mượn |
| `borrower_contact` | VARCHAR(150) | DEFAULT '' | SĐT / Email người mượn |
| `lent_date` | TIMESTAMPTZ | DEFAULT NOW() | Ngày xuất mượn |
| `expected_return_date`| TIMESTAMPTZ | NULL | Ngày hẹn trả |
| `actual_return_date` | TIMESTAMPTZ | NULL | Ngày thực tế đã trả (NULL = Đang mượn) |
| `notes` | TEXT | DEFAULT '' | Ghi chú thêm |
| `created_at` | TIMESTAMPTZ | DEFAULT NOW() | |

---

### 3.2. Mã nguồn PostgreSQL DDL Migration hoàn chỉnh

```sql
-- Kích hoạt extension UUID
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- 1. Bảng tài khoản người dùng (Users)
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(150) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    password_hash VARCHAR(255) NULL, -- Cho phép NULL nếu đăng nhập qua Google
    provider VARCHAR(50) DEFAULT 'local', -- 'local', 'google', 'guest'
    provider_id VARCHAR(255) NULL,       -- Google Sub
    role VARCHAR(50) DEFAULT 'Owner',
    avatar_url TEXT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- 2. Bảng cấu hình người dùng (User Settings)
CREATE TABLE IF NOT EXISTS user_settings (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL UNIQUE REFERENCES users(id) ON DELETE CASCADE,
    currency_symbol VARCHAR(10) DEFAULT '$',
    default_unit VARCHAR(30) DEFAULT 'pcs',
    expiry_warning_days INT DEFAULT 30,
    notify_expiring BOOLEAN DEFAULT TRUE,
    notify_overdue BOOLEAN DEFAULT TRUE,
    notify_low_stock BOOLEAN DEFAULT TRUE,
    vibrate_on_scan BOOLEAN DEFAULT TRUE,
    auto_open_on_scan BOOLEAN DEFAULT TRUE,
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

-- 3. Bảng danh mục phân loại (Categories)
CREATE TABLE IF NOT EXISTS categories (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    icon_name VARCHAR(50) DEFAULT 'devices',
    is_default BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_categories_user ON categories(user_id);

-- 4. Bảng vị trí kho / Thùng chứa (Boxes - Self Referencing Tree)
CREATE TABLE IF NOT EXISTS boxes (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    parent_id UUID NULL REFERENCES boxes(id) ON DELETE SET NULL,
    name VARCHAR(150) NOT NULL,
    type VARCHAR(30) NOT NULL DEFAULT 'box', -- 'room', 'cabinet', 'shelf', 'box', 'drawer', 'other'
    description TEXT DEFAULT '',
    label VARCHAR(100) NOT NULL, -- Mã QR identifier
    icon VARCHAR(50) NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_boxes_user_parent ON boxes(user_id, parent_id);
CREATE INDEX IF NOT EXISTS idx_boxes_label ON boxes(label);

-- 5. Bảng vật phẩm / Tài sản (Items)
CREATE TABLE IF NOT EXISTS items (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    box_id UUID NOT NULL REFERENCES boxes(id) ON DELETE RESTRICT,
    category_id VARCHAR(100) NOT NULL DEFAULT 'General',
    name VARCHAR(255) NOT NULL,
    description TEXT DEFAULT '',
    quantity NUMERIC(12, 2) NOT NULL DEFAULT 1.0,
    unit VARCHAR(30) NOT NULL DEFAULT 'pcs',
    min_quantity NUMERIC(12, 2) NULL, -- Ngưỡng tồn kho tối thiểu riêng từng món
    status VARCHAR(30) NOT NULL DEFAULT 'stored', -- 'stored', 'inUse', 'lent', 'broken', 'disposed'
    purchase_price NUMERIC(15, 2) NULL,
    purchase_date TIMESTAMPTZ NULL,
    warranty_expiry_date TIMESTAMPTZ NULL,
    expiry_date TIMESTAMPTZ NULL,
    serial_number VARCHAR(150) NULL,
    barcode VARCHAR(150) NULL,
    photos JSONB DEFAULT '[]'::jsonb,
    receipt_photos JSONB DEFAULT '[]'::jsonb,
    custom_attributes JSONB DEFAULT '{}'::jsonb, -- Thông số kỹ thuật động (Specifications & Attributes)
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_items_user_box ON items(user_id, box_id);
CREATE INDEX IF NOT EXISTS idx_items_barcode ON items(barcode);
CREATE INDEX IF NOT EXISTS idx_items_serial ON items(serial_number);
CREATE INDEX IF NOT EXISTS idx_items_status ON items(status);
CREATE INDEX IF NOT EXISTS idx_items_custom_attributes ON items USING gin (custom_attributes);

-- 6. Bảng theo dõi mượn / trả (Lending Records)
CREATE TABLE IF NOT EXISTS lending_records (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    item_id UUID NOT NULL REFERENCES items(id) ON DELETE CASCADE,
    borrower_name VARCHAR(150) NOT NULL,
    borrower_contact VARCHAR(150) DEFAULT '',
    lent_date TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expected_return_date TIMESTAMPTZ NULL,
    actual_return_date TIMESTAMPTZ NULL,
    notes TEXT DEFAULT '',
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_lending_active ON lending_records(user_id, actual_return_date);
```

---

## 4. Danh sách API Endpoints chi tiết

### 4.1. Authentication (Xác thực tài khoản & Mạng xã hội)

#### `POST /api/v1/auth/register`
Đăng ký tài khoản người dùng bằng Email/Password. Mật khẩu tối thiểu **8 ký tự**. Email được chuẩn hoá `lower(trim())` khi đăng ký/đăng nhập/Google (unique index `uq_users_email_lower`; migration chỉ log cảnh báo nếu DB cũ còn email trùng khác hoa/thường). Email đã tồn tại và race unique đều trả cùng một 400 `REGISTRATION_FAILED` với thông báo chung (không lộ email nào đã đăng ký). Lỗi nội bộ/JWT không còn trả `err.Error()` ra client (token sai → `TOKEN_EXPIRED_OR_INVALID` "Token is invalid or has expired").
- **Yêu cầu xác thực:** Không (Public).
- **Rate limit:** `/auth/*` public giới hạn 30 request / 15 phút / IP; `/auth/login` thêm 10 lần / 15 phút / email. Vượt ngưỡng → 429 `RATE_LIMITED`. Bộ đếm in-memory theo instance — trên Lambda cần thêm API Gateway throttling hoặc WAF rate-based rule để giới hạn toàn cục.
- **Request Body:**
  ```json
  {
    "name": "Sara Wilson",
    "email": "sara.wilson@example.com",
    "password": "SecurePassword123!"
  }
  ```
- **Response 201 Created:**
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
    },
    "message": "Account registered successfully"
  }
  ```

#### `POST /api/v1/auth/login`
Đăng nhập bằng Email/Password.
- **Yêu cầu xác thực:** Không (Public).
- **Request Body:**
  ```json
  {
    "email": "sara.wilson@example.com",
    "password": "SecurePassword123!"
  }
  ```
- **Response 200 OK:**
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
    },
    "message": "Login successful"
  }
  ```

#### `POST /api/v1/auth/google`
Đăng nhập hoặc đăng ký tự động tài khoản người dùng bằng thông tin từ Google Sign-In.
- **Yêu cầu xác thực:** Không (Public).
- **Request Body:**
  ```json
  {
    "email": "sara.wilson@gmail.com",
    "name": "Sara Wilson",
    "avatar_url": "https://lh3.googleusercontent.com/a/default-user=s96-c",
    "id_token": "<Google ID token — bắt buộc>"
  }
  ```
- **Nghiệp vụ xử lý Backend:**
  0. Bắt buộc có `id_token`; verify với Google (`aud`/`azp` khớp `GOOGLE_CLIENT_ID`, `email_verified = true`). Email lấy từ token, không dùng email trong body. Thiếu/không hợp lệ → 400 `AUTH_FAILED`.
  1. Kiểm tra trong bảng `users` theo `email`.
  2. Nếu người dùng đã tồn tại: Cập nhật `name`, `avatar_url` (nếu có), lưu `provider = 'google'` và **xoá `password_hash`** (đăng ký local không xác minh email, nên mật khẩu có thể do kẻ khác đặt trước — chống pre-account takeover).
  3. Nếu chưa tồn tại: Tạo mới người dùng với `role = 'Owner'`, `provider = 'google'`, `password_hash = NULL`.
  4. Khởi tạo cấu hình mặc định trong `user_settings` và danh mục mặc định trong `categories` (nếu là tài khoản mới).
  5. Ký và trả về JWT Bearer Token.
- **Response 200 OK:**
  ```json
  {
    "success": true,
    "data": {
      "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
      "user": {
        "id": "a9d701e2-b883-4a11-8fc2-48a804ec62b1",
        "name": "Sara Wilson",
        "email": "sara.wilson@gmail.com",
        "role": "Owner",
        "avatarUrl": "https://lh3.googleusercontent.com/a/default-user=s96-c"
      }
    },
    "message": "Google authentication successful"
  }
  ```

#### `GET /api/v1/auth/me`
Lấy thông tin người dùng hiện tại từ JWT Bearer Token.
- **Yêu cầu xác thực:** Bearer Token.
- **Response 200 OK:**
  ```json
  {
    "success": true,
    "data": {
      "id": "c8699bf3-8ea7-4228-a3cb-465451e06fa5",
      "name": "Sara Wilson",
      "email": "sara.wilson@example.com",
      "role": "Owner",
      "avatarUrl": "https://storage.inventory.local/avatars/user_1.png"
    }
  }
  ```

#### `DELETE /api/v1/auth/me`
Xoá vĩnh viễn tài khoản hiện tại cùng toàn bộ dữ liệu (box, item kể cả trong thùng rác, phiếu mượn, lịch sử tồn kho, danh mục, cấu hình) trong một transaction, sau đó xoá file ảnh đã upload (best-effort).
- **Yêu cầu xác thực:** Bearer Token.
- **Response 200 OK:** `{ "success": true, "data": null, "message": "Account deleted" }`
- Token cũ trở nên vô hiệu (mọi request → 401).

#### `POST /api/v1/auth/refresh`
Cấp token mới cho user đang đăng nhập (cùng `token_version`). Response `data`: `{ "token": "...", "user": {...} }` như login.
- **Yêu cầu xác thực:** Bearer Token.

#### `POST /api/v1/auth/logout-all`
Thu hồi mọi token đã cấp cho tài khoản (tăng `users.token_version`). Token cũ → 401 `TOKEN_REVOKED`. Đăng nhập Google vào tài khoản local cũng tăng `token_version`.
- **Yêu cầu xác thực:** Bearer Token.

> JWT có claim `tv` (token version). Middleware so với `users.token_version`: lệch → 401 `TOKEN_REVOKED`; user đã xoá → 401. FE gặp 401 phải xoá phiên, chuyển về đăng nhập và **giữ nguyên hàng đợi offline**.

---

### 4.2. Profile & Settings (Thông tin cá nhân & Cấu hình)

#### `GET /api/v1/settings`
Lấy toàn bộ cấu hình hiển thị và tùy chọn cảnh báo của người dùng.
- **Yêu cầu xác thực:** Bearer Token.
- **Response 200 OK:**
  ```json
  {
    "success": true,
    "data": {
      "currencySymbol": "$",
      "defaultUnit": "pcs",
      "expiryWarningDays": 30,
      "warrantyWarningDays": 30,
      "timezone": "UTC",
      "notifyExpiring": true,
      "notifyOverdue": true,
      "notifyLowStock": true,
      "vibrateOnScan": true,
      "autoOpenOnScan": true
    }
  }
  ```

#### `PATCH /api/v1/settings`
Cập nhật tùy chọn hiển thị và cảnh báo của người dùng.
- **Yêu cầu xác thực:** Bearer Token.
- **Request Body:**
  ```json
  {
    "currencySymbol": "₫",
    "defaultUnit": "cái",
    "expiryWarningDays": 15,
    "warrantyWarningDays": 45,
    "timezone": "Asia/Ho_Chi_Minh",
    "notifyLowStock": true
  }
  ```
  `expiryWarningDays`/`warrantyWarningDays` trong khoảng 1..3650. `timezone` là tên IANA hợp lệ (không nhận `""`/`Local`); "hôm nay" của cảnh báo, báo cáo và lọc hạn dùng múi giờ này.
- **Response 200 OK:**
  ```json
  {
    "success": true,
    "data": { ... },
    "message": "Settings updated successfully"
  }
  ```

#### `PATCH /api/v1/user/profile`
Cập nhật thông tin tài khoản (Tên, vai trò, avatar).
- **Yêu cầu xác thực:** Bearer Token.
- **Request Body:**
  ```json
  {
    "name": "Sara Wilson",
    "role": "Inventory Lead",
    "avatarUrl": "https://cdn.example.com/avatars/sara.png"
  }
  ```
  *(Các trường là tùy chọn, chỉ gửi các trường cần cập nhật)*.
- **Response 200 OK:**
  ```json
  {
    "success": true,
    "data": {
      "id": "a9d701e2-b883-4a11-8fc2-48a804ec62b1",
      "name": "Sara Wilson",
      "email": "sara.wilson@gmail.com",
      "role": "Inventory Lead",
      "avatarUrl": "https://cdn.example.com/avatars/sara.png"
    },
    "message": "Profile updated successfully"
  }
  ```

---

### 4.3. Categories (Danh mục vật phẩm)

#### `GET /api/v1/categories`
Lấy danh sách tất cả danh mục (bao gồm danh mục hệ thống và danh mục người dùng tự tạo).
- **Yêu cầu xác thực:** Bearer Token.
- **Response 200 OK:**
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
Xóa danh mục tự tạo. Lỗi: 400 nếu là danh mục hệ thống (`is_default = true`), 404 nếu không tồn tại, 409 nếu còn vật phẩm đang dùng danh mục ("category is used by N item(s)").

Khi tạo/sửa item, `categoryId` (tên hoặc id) phải tồn tại trong danh mục mặc định hoặc của user, nếu không trả 400 `category does not exist`; giá trị lưu là **tên** danh mục.

---

### 4.4. Storage Spaces / Boxes (Khu vực & Thùng lưu trữ)

#### `GET /api/v1/boxes`
Lấy danh sách tất cả các thùng/vị trí lưu trữ. Hỗ trợ query theo cây phân cấp.
- **Query Params:**
  - `parentId`: UUID hoặc `root` để lấy các tầng cao nhất (Rooms / Floors).
  - `tree`: `true` để lấy cấu trúc lồng nhau dạng cây phân cấp hoàn chỉnh.
- **Response 200 OK:**
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
- **Response 200 OK:**
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
      ],
      "itemCount": 1,
      "itemsTruncated": false
    }
  }
  ```

> `items` tối đa 200 phần tử; `itemCount` là tổng thật và `itemsTruncated=true` khi còn nữa → phân trang qua `GET /items?boxId=<id>&page=N`.

#### `PUT /api/v1/boxes/:id`
Chỉnh sửa tên, phân loại (`room`, `cabinet`, `shelf`, `box`, `drawer`), mô tả của thùng chứa.

#### `PATCH /api/v1/boxes/:id/move`
Di chuyển thùng chứa sang một vị trí cha khác trong cây phân cấp (hỗ trợ kéo thả).
- **Request Body:**
  ```json
  {
    "newParentId": "b1b017f8-1111-4234-8c81-86a073f47262"
  }
  ```
  *(Truyền `null` hoặc chuỗi rỗng nếu muốn đưa vị trí này lên tầng gốc cao nhất - Root Level)*.
- **Ràng buộc:** Backend phải kiểm tra tránh tạo chu trình vòng lặp (vị trí cha mới không được trùng với chính nó hoặc nằm trong cây con cháu của nó).

#### `DELETE /api/v1/boxes/:id`
Xóa thùng chứa.
- **Query Params:**
  - `cascade`: `true` nếu xóa toàn bộ đồ bên trong, `false` để chặn báo lỗi nếu thùng còn đồ đạc.

---

### 4.5. Items / Asset Catalog (Vật phẩm, Tài sản & Thuộc tính động)

#### `GET /api/v1/items`
Tìm kiếm và lọc danh sách tài sản trong kho.
- **Query Params:**
  - `q`: Từ khóa tìm kiếm (tên, mô tả, barcode, serial number). Tên và mô tả không phân biệt hoa thường và **bỏ dấu tiếng Việt** (`ban` tìm được `Bàn`, `den` tìm được `Đèn`) nhờ hàm SQL `f_unaccent_vi`; ký tự `%` và `_` trong từ khoá được hiểu theo nghĩa đen.
  - `boxId`: Lọc theo vị trí lưu trữ.
  - `category`: Lọc theo danh mục.
  - `status`: Lọc theo trạng thái (`stored`, `inUse`, `lent`, `broken`, `disposed`).
  - `isLowStock`: `true` để lọc ra danh sách sắp hết hàng (`quantity + lentQuantity <= min_quantity`).
  - `isExpired`: `true` để lấy các món đã hết hạn sử dụng.
  - `isExpiring`: `true` để lấy các món sắp hết hạn trong N ngày.
  - `page`: Số trang (mặc định 1).
  - `limit`: Số item trên mỗi trang (mặc định 20, tối đa 200 — giá trị lớn hơn bị cắt về 200).
- **Response 200 OK:**
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
        "purchaseDate": "2026-07-31T00:00:00.000Z",
        "warrantyExpiryDate": "2027-07-31T00:00:00.000Z",
        "expiryDate": null,
        "serialNumber": null,
        "barcode": "6934177712345",
        "photos": ["https://storage.inventory.local/items/charger65w.jpg"],
        "receiptPhotos": [],
        "customAttributes": {
          "Max Output": "65W",
          "Ports": "2x USB-C",
          "Color": "Space Gray"
        },
        "createdAt": "2026-07-31T08:00:00.000Z"
      }
    ],
    "meta": { "total": 45, "page": 1, "limit": 20 }
  }
  ```

#### `POST /api/v1/items`
Tạo một vật phẩm mới trong kho kèm thông số kỹ thuật tùy biến (`customAttributes`).
- **Request Body:**
  ```json
  {
    "name": "MacBook Pro 16\"",
    "description": "Workstation laptop for mobile dev",
    "boxId": "b1b017f8-1111-4234-8c81-86a073f47262",
    "categoryId": "Electronics",
    "quantity": 1.0,
    "unit": "pcs",
    "status": "inUse",
    "minQuantity": 1.0,
    "purchasePrice": 2499.00,
    "purchaseDate": "2026-01-15T00:00:00.000Z",
    "warrantyExpiryDate": "2028-01-15T00:00:00.000Z",
    "serialNumber": "C02G40ZBMD6R",
    "barcode": "194252000000",
    "photos": [
      "https://storage.inventory.local/items/macbook16.jpg"
    ],
    "receiptPhotos": [],
    "customAttributes": {
      "Color": "Space Gray",
      "Storage": "512GB SSD",
      "RAM": "36GB Unified",
      "Processor": "Apple M3 Pro",
      "Display": "16.2-inch Liquid Retina XDR"
    }
  }
  ```

#### `GET /api/v1/items/:id`
Lấy chi tiết toàn bộ thông số của vật phẩm, breadcrumb vị trí lưu trữ, thông tin bảo hành, và lịch sử mượn trả.

#### `PUT /api/v1/items/:id`
Cập nhật toàn bộ thông tin vật phẩm (hỗ trợ chỉnh sửa số lượng, ngưỡng `minQuantity`, giá, ảnh, thông số `customAttributes`).
- `updatedAt` (tuỳ chọn): phiên bản mà bản sửa dựa trên. Nếu khác `items.updated_at` hiện tại (so ở mức mili giây) → 409 `CONFLICT`, không ghi gì. FE gửi khi phát lại thay đổi offline; không gửi = ghi đè như cũ. `PUT /boxes/:id` có cùng quy tắc.
- Đổi `boxId` ghi một dòng `stock_movements` kiểu move, note `"to <tên box>"` (giống `PATCH /items/:id/move`).
- Response item có thêm `lentQuantity` (tổng số lượng đang cho mượn từng phần) và `GET /items/:id` có `activeLoans` (mọi khoản mượn đang mở, kể cả từng phần). Low stock tính theo `quantity + lentQuantity <= minQuantity`.

#### `PATCH /api/v1/items/:id/quantity`
Cập nhật nhanh số lượng tồn kho (tăng/giảm trực tiếp khi xuất/nhập kho).
- **Request Body:**
  ```json
  {
    "quantity": 3.0,
    "reason": "consume",
    "note": "optional"
  }
  ```
  Hoặc gửi `delta` (số có dấu, cộng vào số lượng hiện tại trong transaction có khoá hàng) thay cho `quantity`; phải có **đúng một** trong hai, nếu không 400 `INVALID_INPUT`. Kết quả âm → 400. Với `delta`, `data` là `{ "id", "quantity" }`. FE xếp hàng thay đổi offline dưới dạng `delta` để nhiều thiết bị cộng dồn được.
  Với `delta` có thể kèm `idempotencyKey` (chuỗi ≤ 64 ký tự, duy nhất theo user): gửi lại cùng key thì không cộng lần hai (trả số lượng hiện tại), để phát lại hàng đợi offline an toàn.
  `reason` (tuỳ chọn, mặc định `adjust`): `restock | consume | damaged | lost | correction | adjust`; giá trị khác trả 400. Mỗi lần số lượng đổi ghi một dòng `stock_movements`.
- **Response 200 OK:**
  ```json
  {
    "success": true,
    "data": { ... },
    "message": "Quantity updated successfully"
  }
  ```

#### `GET /api/v1/items/:id/movements`
Lịch sử xuất/nhập kho (tối đa 200 dòng, mới nhất trước) và ước tính tiêu thụ.
- **Response 200 OK:**
  ```json
  {
    "success": true,
    "data": {
      "movements": [
        { "id": "...", "itemId": "...", "delta": -6, "quantityAfter": 4, "reason": "consume", "note": "", "createdAt": "..." }
      ],
      "avgDailyConsumption30d": 0.2,
      "estimatedDaysLeft": 20
    }
  }
  ```
  `reason` còn có `initial` (lúc tạo) và `move` (đổi vị trí, `delta = 0`). `avgDailyConsumption30d` chỉ tính các dòng `consume` trong 30 ngày; `estimatedDaysLeft = null` khi chưa có dữ liệu tiêu thụ. Xoá item xoá luôn lịch sử.

#### `PATCH /api/v1/items/:id/move`
Chuyển vật phẩm sang thùng/vị trí lưu trữ mới.
- **Request Body:**
  ```json
  {
    "targetBoxId": "box_storage_level_2"
  }
  ```
- **Response 200 OK:**
  ```json
  {
    "success": true,
    "data": { ... },
    "message": "Item moved successfully"
  }
  ```

#### `DELETE /api/v1/items/:id`
Xóa vĩnh viễn vật phẩm khỏi hệ thống. Bị chặn (409) nếu item còn khoản mượn đang mở; xoá vĩnh viễn từ thùng rác cũng vậy.

---

### 4.6. Lending & Borrowing Tracker (Quản lý Mượn / Trả)

#### `POST /api/v1/lending/lend`
Xuất mượn một vật phẩm.
- **Nghiệp vụ xử lý:** 
  1. Kiểm tra vật phẩm có đang ở trạng thái `stored` hoặc `inUse` không (chặn nếu đang `lent`, `broken`, `disposed`).
  2. Tạo bản ghi mới trong bảng `lending_records`.
  3. Cập nhật `item.status = 'lent'`.
  4. Nếu `id` gửi kèm đã tồn tại (khoản mượn khác, hoặc của user khác) → 409 `CONFLICT`.
- **Request Body:**
  ```json
  {
    "itemId": "e72ff0a4-39f5-4927-920f-0746e5ca9399",
    "borrowerName": "Alex Turner",
    "borrowerContact": "alex.turner@gmail.com",
    "lentDate": "2026-10-01T09:00:00.000Z",
    "expectedReturnDate": "2026-10-08T18:00:00.000Z",
    "notes": "Lent with charging brick and cable"
  }
  ```
- **Response 201 Created:**
  ```json
  {
    "success": true,
    "data": {
      "id": "lend_rec_9812",
      "itemId": "e72ff0a4-39f5-4927-920f-0746e5ca9399",
      "borrowerName": "Alex Turner",
      "borrowerContact": "alex.turner@gmail.com",
      "lentDate": "2026-10-01T09:00:00.000Z",
      "expectedReturnDate": "2026-10-08T18:00:00.000Z",
      "notes": "Lent with charging brick and cable"
    },
    "message": "Item lent successfully"
  }
  ```

#### `POST /api/v1/lending/:id/return`
Đánh dấu đã hoàn trả vật phẩm.
- **Nghiệp vụ xử lý:**
  1. Cập nhật `actual_return_date = NOW()`.
  2. Tự động chuyển trạng thái của item về `status = 'stored'`.
- **Request Body:** `{}` (Body rỗng).
- **Response 200 OK:**
  ```json
  {
    "success": true,
    "message": "Item returned successfully"
  }
  ```

#### `GET /api/v1/lending/active`
Lấy danh sách các tài sản đang cho mượn (`actual_return_date IS NULL`).

#### `GET /api/v1/lending/overdue`
Lấy danh sách các tài sản mượn đã quá hạn trả (`actual_return_date IS NULL` và **ngày** hẹn trả < hôm nay theo múi giờ user; khoản hẹn trả hôm nay chưa quá hạn).

#### `GET /api/v1/lending/history?itemId=...`
Lấy toàn bộ lịch sử các lần mượn trả của một món đồ. Bỏ `itemId` thì trả **mọi khoản đã trả** của người dùng, mới trả nhất trước, có phân trang (`page`, `limit` mặc định 50, tối đa 200; `meta.total`).

---

### 4.7. Barcode & QR Code Scanning (Quét mã thông minh)

#### `GET /api/v1/scan/lookup`
Tra cứu thông minh mã quét từ camera điện thoại hoặc máy đọc barcode.
- **Query Params:**
  - `code`: Chuỗi barcode hoặc nội dung QR vừa quét (VD: `loc_floor_1`, `6934177712345`, `SN-MBP14-M3-9821`).
- **Nghiệp vụ xử lý Backend:**
  1. Đối soát bảng `boxes` qua trường `label`. Nếu khớp -> trả về type `box`.
  2. Nếu không, đối soát bảng `items` qua trường `barcode`, `serial_number`, hoặc `id`. Nếu khớp -> trả về type `item`.
  3. Nếu không tìm thấy bất kỳ bản ghi nào -> trả về HTTP Status 404 với error code `NOT_FOUND`.
- **Response 200 OK (Khớp Box):**
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
- **Response 200 OK (Khớp Item):**
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
        "status": "stored",
        "isLowStock": true,
        "barcode": "6934177712345"
      }
    }
  }
  ```

---

### 4.8. Alerts & Dashboard (Trung tâm Cảnh báo & Sức khỏe kho)

#### `GET /api/v1/alerts/summary`
Lấy số đếm tổng hợp cho các huy hiệu cảnh báo (Badge count) trên trang chủ.
- **Response 200 OK:**
  ```json
  {
    "success": true,
    "data": {
      "totalAlerts": 7,
      "overdueCount": 1,
      "expiredCount": 2,
      "expiringSoonCount": 3,
      "lowStockCount": 1,
      "expiringWarrantyCount": 0,
      
      // Alias hỗ trợ tương thích ngược Frontend:
      "totalAlertCount": 7,
      "overdueLoansCount": 1,
      "warrantyEndingCount": 0
    }
  }
  ```

#### `GET /api/v1/alerts/all`
Lấy chi tiết danh sách tất cả các nhóm vật phẩm cần lưu ý:
1. `expiredItems`: Mặt hàng hết hạn dùng.
2. `expiringSoonItems`: Mặt hàng sắp hết hạn (trong vòng `expiryWarningDays` ngày).
3. `expiringWarranties`: Hết hạn bảo hành sắp tới (trong vòng 30 ngày).
4. `overdueLoans`: Khoản mượn quá hạn hẹn trả.
5. `lowStockItems`: Mặt hàng chạm hoặc dưới ngưỡng `min_quantity` riêng biệt (cộng cả số lượng đang cho mượn từng phần).

Lọc trạng thái: hết hạn/sắp hết hạn bỏ qua item `disposed|lost|broken`; bảo hành bỏ qua `disposed|lost`.

---

### 4.9. Reports & Inventory Audit (Báo cáo & Kiểm kê)

#### `GET /api/v1/reports/inventory-health`
Cung cấp dữ liệu phân tích sức khỏe kho tài sản:
- **Response 200 OK:**
  ```json
  {
    "success": true,
    "data": {
      "totalAssetsCount": 38,
      "totalEstimatedValue": 12850.00,
      "breakdown": {
        "stored": 28,
        "inUse": 7,
        "lent": 3,
        "lowStock": 2,
        "expired": 1,
        "expiringSoon": 2
      },
      "storageLocationsCount": 14
    }
  }
  ```

#### `GET /api/v1/reports/export`
Xuất file báo cáo kiểm kê tài sản (query `format=csv|json`, mặc định csv) để sao lưu hoặc kiểm kê định kỳ. Dữ liệu được stream theo lô 500 item (không giới hạn tổng số); CSV vô hiệu hóa công thức (`=`, `+`, `-`, `@` ở đầu ô được thêm `'`). Lỗi giữa chừng sẽ cắt ngắn file (đã gửi header).

---

### 4.10. Media Upload (Tải lên hình ảnh)

#### `POST /api/v1/media/upload`
Tải lên ảnh chụp thực tế đồ đạc hoặc ảnh hóa đơn mua hàng.
- **Content-Type:** `multipart/form-data`
- **Body:**
  - `file`: File nhị phân (jpg, png, webp, heic).
  - `folder`: `items` hoặc `receipts` (giá trị khác được coi là `items`).
- Tối đa 10 MB, nội dung file phải khớp đuôi file; giới hạn 60 lượt upload / người dùng / 10 phút (429 `RATE_LIMITED`).
- File được lưu trong thư mục riêng của người upload: `<folder>/<userID>/<uuid>.<ext>`.
- **Response 200 OK:**
  ```json
  {
    "success": true,
    "data": {
      "url": "https://storage.inventory.local/uploads/items/3f2b…-userID/6c1e…-uuid.webp"
    },
    "message": "File uploaded successfully"
  }
  ```
- **Quyền sở hữu ảnh:** khi tạo/sửa item, `photos` và `receiptPhotos` không được chứa URL nằm trong thư mục của người dùng khác (400 `CREATE_FAILED`/`UPDATE_FAILED`). Link ngoài và file cũ không có thư mục người dùng (upload trước khi có quy ước này) vẫn được chấp nhận; URL đã có sẵn trên item luôn được giữ. Xoá tài khoản hoặc dọn thùng rác bỏ qua file nằm trong thư mục của người khác.

---

## 5. Quy tắc nghiệp vụ bắt buộc (Business Logic Constraints)

1. **Ngưỡng tồn kho tối thiểu (`min_quantity`):**
   - Đây là trường tùy chọn trên từng món đồ, không bắt buộc.
   - Nếu `min_quantity IS NULL`: Món đồ không áp dụng theo dõi tồn kho thiếu hụt.
   - Nếu `min_quantity IS NOT NULL`: Khi `quantity <= min_quantity`, hệ thống tự động gắn nhãn `isLowStock = true` và đưa vào danh sách cảnh báo thiếu hụt.

2. **Chống lặp vòng cây vị trí (Cycle Prevention):**
   - Việc move chạy trong transaction có `pg_advisory_xact_lock` theo user nên hai lần move đồng thời không tạo được vòng; độ sâu tối đa 64.
   - Khi cập nhật `parent_id` của một `Box`, hệ thống phải kiểm tra đảm bảo `parent_id` mới không được trùng với chính nó và không phải là con cháu cấp dưới của nó.
   - Hỗ trợ gán `newParentId = null` để đưa vị trí về cấp cao nhất (Root).

3. **Toàn vẹn trạng thái mượn đồ (Lending Integrity):**
   - Không cho phép xuất mượn món đồ đang có trạng thái `lent`, `broken`, hoặc `disposed`.
   - Khi mượn, trạng thái trước đó được lưu vào `lending_records.previous_status`; khi trả đồ, item trở về đúng trạng thái đó (`stored`/`inUse`, mặc định `stored`) và `actual_return_date = NOW()`.
   - Trạng thái `lent` chỉ được đặt/gỡ qua API mượn/trả; `POST/PUT /items` không được chuyển item sang hoặc ra khỏi `lent`.

6. **Toàn vẹn dữ liệu & xoá:**
   - `type` của box chỉ nhận `room|cabinet|shelf|box|drawer|other`; `status` của item chỉ nhận `stored|inUse|lent|broken|disposed`; `quantity` và `minQuantity` không âm.
   - Để xoá `minQuantity` khi `PUT /items/:id`, gửi `"clearMinQuantity": true` (gửi `minQuantity: null` bị coi là "không đổi").
   - Xoá item đang `lent` → 409. Xoá box: mặc định `cascade=false` và trả 409 nếu box còn con/item; `cascade=true` xoá cả cây nhưng bị từ chối (409) nếu trong cây có item đang `lent`. Khoản mượn của item bị xoá được xoá theo.
   - Cập nhật item/box dùng optimistic lock theo `updated_at`: nếu bản ghi vừa bị sửa bởi request khác → 409 `CONFLICT`, client nên tải lại.
   - Khoá ngoại ở mức DB: `items.box_id`, `lending_records.item_id`, `boxes.parent_id`. Nếu dữ liệu cũ vi phạm, server dừng khi khởi động (cần dọn dữ liệu).
   - `GET /reports/export` stream CSV/JSON theo lô 500 bản ghi; ô CSV bắt đầu bằng `= + - @` được escape chống formula injection.

4. **Định danh mã vạch & QR Code:**
   - Trường `label` của bảng `boxes` là duy nhất **trong phạm vi từng người dùng** (unique index `(user_id, label)`) để in tem QR dán lên hộp đựng.
   - Trường `barcode` của `items` hỗ trợ các định dạng mã vạch thương mại chuẩn (UPC-A, EAN-13, Code 128).

5. **Xử lý Đăng nhập Mạng xã hội (Social Login Handling):**
   - Nếu email đăng nhập từ Google đã tồn tại trong cơ sở dữ liệu: Liên kết tài khoản bằng cách cập nhật `provider` tương ứng, cập nhật `avatar_url` (nếu có).
   - Nếu email chưa từng đăng ký: Tự động khởi tạo bản ghi `users`, cấu hình `user_settings` mặc định, và tạo các danh mục hệ thống `categories`.

---

## 6. Mô hình DTOs mã nguồn Go (Golang Structs)

Dưới đây là các định nghĩa Struct chuẩn mực dành cho Backend triển khai bằng **Go (Gin Framework)**:

```go
package models

import "time"

// Envelope Response chuẩn
type ApiResponse[T any] struct {
	Success bool   `json:"success"`
	Data    T      `json:"data,omitempty"`
	Message string `json:"message,omitempty"`
}

type ApiError struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Details []string `json:"details,omitempty"`
}

type ApiErrorResponse struct {
	Success bool     `json:"success"`
	Error   ApiError `json:"error"`
}

// 1. Social Auth Requests
type GoogleLoginRequest struct {
	Email     string  `json:"email" binding:"required,email"`
	Name      string  `json:"name" binding:"required"`
	AvatarURL *string `json:"avatar_url"`
}

type AuthResponseData struct {
	Token string   `json:"token"`
	User  UserView `json:"user"`
}

type UserView struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Email     string  `json:"email"`
	Role      string  `json:"role"`
	AvatarURL *string `json:"avatarUrl,omitempty"`
}

type UpdateProfileRequest struct {
	Name      *string `json:"name,omitempty"`
	Role      *string `json:"role,omitempty"`
	AvatarURL *string `json:"avatarUrl,omitempty"`
}

// 2. Item & Dynamic Specifications
type ItemDTO struct {
	ID                 string                 `json:"id"`
	BoxID              string                 `json:"boxId"`
	CategoryID         string                 `json:"categoryId"`
	Name               string                 `json:"name"`
	Description        string                 `json:"description"`
	Quantity           float64                `json:"quantity"`
	Unit               string                 `json:"unit"`
	MinQuantity        *float64               `json:"minQuantity,omitempty"`
	Status             string                 `json:"status"` // stored, inUse, lent, broken, disposed
	PurchasePrice      *float64               `json:"purchasePrice,omitempty"`
	PurchaseDate       *time.Time             `json:"purchaseDate,omitempty"`
	WarrantyExpiryDate *time.Time             `json:"warrantyExpiryDate,omitempty"`
	ExpiryDate         *time.Time             `json:"expiryDate,omitempty"`
	SerialNumber       *string                `json:"serialNumber,omitempty"`
	Barcode            *string                `json:"barcode,omitempty"`
	Photos             []string               `json:"photos"`
	ReceiptPhotos      []string               `json:"receiptPhotos"`
	CustomAttributes   map[string]interface{} `json:"customAttributes"` // Specifications & Attributes
	CreatedAt          time.Time              `json:"createdAt"`
	UpdatedAt          *time.Time             `json:"updatedAt,omitempty"`
}

// 3. Lending DTOs
type LendItemRequest struct {
	ItemID             string     `json:"itemId" binding:"required"`
	BorrowerName       string     `json:"borrowerName" binding:"required"`
	BorrowerContact    string     `json:"borrowerContact"`
	LentDate           time.Time  `json:"lentDate" binding:"required"`
	ExpectedReturnDate *time.Time `json:"expectedReturnDate"`
	Notes              string     `json:"notes"`
}

type LendingRecordDTO struct {
	ID                 string     `json:"id"`
	ItemID             string     `json:"itemId"`
	BorrowerName       string     `json:"borrowerName"`
	BorrowerContact    string     `json:"borrowerContact"`
	LentDate           time.Time  `json:"lentDate"`
	ExpectedReturnDate *time.Time `json:"expectedReturnDate,omitempty"`
	ActualReturnDate   *time.Time `json:"actualReturnDate,omitempty"`
	Notes              string     `json:"notes"`
}

// 4. Box DTOs
type MoveBoxRequest struct {
	NewParentID *string `json:"newParentId"`
}

type MoveItemRequest struct {
	TargetBoxID string `json:"targetBoxId" binding:"required"`
}

type UpdateQuantityRequest struct {
	Quantity float64 `json:"quantity" binding:"required,gte=0"`
}

// 5. Scan Lookup
type ScanLookupResponse struct {
	Type    string      `json:"type"` // "box" or "item"
	Payload interface{} `json:"payload"`
}
```

---

## 7. Kiến trúc công nghệ Backend & Triển khai

- **Ngôn ngữ & Framework:**
  - **Go 1.22+** với **Gin Web Framework**.
  - **GORM ORM** (với PostgreSQL driver) để quản lý tương tác dữ liệu, migration, JSONB serialization.
  - **golang-jwt/jwt/v5** cho ký và xác thực Bearer JWT Token.
- **Cơ sở dữ liệu:**
  - **PostgreSQL 15+** hỗ trợ UUID v4, JSONB serializer, GIN Indexing, và CTE đệ quy tra cứu cây phân cấp vị trí (`WITH RECURSIVE`).
- **File Storage Provider:**
  - **Local Storage:** Lưu trực tiếp trong thư mục `./uploads/` (cho môi trường phát triển local).
  - **Cloud Storage:** Sẵn sàng kết nối AWS S3 / Cloudflare R2 / MinIO cho môi trường production qua interface Storage Driver.
- **Serverless & Containerization:**
  - Hỗ trợ chạy chế độ HTTP Server thông thường (`cmd/api`) hoặc chế độ Serverless AWS Lambda (`cmd/lambda`) qua adapter `aws-lambda-go-api-proxy/gin`.
  - Docker & Docker Compose sẵn sàng đóng gói single binary container siêu nhẹ (Alpine/Distroless).

## Thùng rác (soft delete) — BIZ-5

- `items` và `boxes` có cột `deleted_at`; `DELETE` chỉ đánh dấu xóa mềm (cascade box dùng chung một `deleted_at` để khôi phục cả cây). Lịch sử mượn/kho được giữ nguyên.
- Index `uq_boxes_user_label_live` chỉ ràng buộc label với box còn sống.
- Endpoints (cần auth): `GET /trash`, `POST /trash/items/:id/restore`, `POST /trash/boxes/:id/restore`, `DELETE /trash/items/:id`, `DELETE /trash/boxes/:id`, `DELETE /trash?olderThanDays=N` (dọn thùng rác).
- Không xóa được item đang có khoản mượn mở.
- **Tự dọn:** Lambda nhận sự kiện EventBridge Scheduler/rule (`source: "aws.events"`, ví dụ `rate(1 day)`) và xóa vĩnh viễn mọi thứ đã nằm trong thùng rác quá 30 ngày (`app.TrashRetentionDays`) của tất cả user, kèm file ảnh/hóa đơn của item bị xóa. Item đang có khoản mượn mở vẫn được giữ lại. Cần tự tạo rule trên AWS trỏ vào function; server HTTP (`cmd/api`) không có job này.

## Mượn một phần — BIZ-1

- `POST /lending/lend` nhận thêm `quantity` (tùy chọn). Bỏ trống hoặc `>= quantity` hiện có → mượn nguyên item (status `lent`). `0 < quantity < tồn kho` → mượn một phần: trừ `items.quantity`, giữ nguyên status, `isPartial = true`; trả lại sẽ cộng số lượng về.
- `quantity` vượt tồn kho → 400. Nhiều khoản mượn một phần có thể cùng mở trên một item (`uq_lending_open_whole` chỉ chặn khoản mượn nguyên item).
- `lending_records` có thêm `quantity`, `is_partial`; stock movement có lý do `lend` / `return`.

## ID do client sinh (offline sync) — BIZ-10

- `POST /boxes`, `POST /items` và `POST /lending/lend` nhận thêm `id` (UUID, tùy chọn). App offline tự sinh ID để không cần ánh xạ ID tạm.
- Idempotent: nếu `id` đã tồn tại cho user này, trả lại bản ghi đã có (không tạo trùng, không lend lần hai). `id` đã thuộc user khác → 409.
