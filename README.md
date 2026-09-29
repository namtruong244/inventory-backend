# Smart Inventory & Asset Tracking - Go Backend

Hệ thống Backend RESTful API hoàn chỉnh viết bằng **Go (Golang)** và **PostgreSQL**, được thiết kế để triển khai linh hoạt cả ở môi trường máy chủ truyền thống (Docker, VPS) và môi trường không máy chủ (**AWS Lambda**), hỗ trợ lưu trữ media phân tán qua **Cloudflare R2** (S3-compatible).

---

## 1. Công nghệ sử dụng (Tech Stack)

- **Ngôn ngữ:** Go (Golang 1.22+)
- **Web Framework:** [Gin Web Framework](https://github.com/gin-gonic/gin)
- **Serverless Runtime:** [AWS Lambda Go](https://github.com/aws/aws-lambda-go) & [aws-lambda-go-api-proxy](https://github.com/awslabs/aws-lambda-go-api-proxy)
- **Object Storage:** [Cloudflare R2](https://www.cloudflare.com/developer-platform/r2/) qua [AWS SDK for Go v2](https://github.com/aws/aws-sdk-go-v2)
- **Database & ORM:** PostgreSQL 16+ với [GORM](https://gorm.io/) & [pgx](https://github.com/jackc/pgx)
- **Xác thực:** Bearer JWT ([golang-jwt/jwt](https://github.com/golang-jwt/jwt))
- **Mật khẩu:** Bcrypt hashing ([golang.org/x/crypto/bcrypt](https://pkg.go.dev/golang.org/x/crypto/bcrypt))
- **CI/CD:** GitHub Actions tự động build binary `bootstrap` và deploy lên AWS Lambda

---

## 2. Cấu trúc thư mục (Project Structure)

```
Inventory_Backend/
├── .github/
│   └── workflows/
│       └── deploy.yml              # GitHub Actions tự động build & deploy lên AWS Lambda
├── cmd/
│   ├── api/
│   │   └── main.go                 # Entrypoint máy chủ HTTP cục bộ / Docker
│   └── lambda/
│       └── main.go                 # Entrypoint AWS Lambda (bootstrap)
├── internal/
│   ├── app/
│   │   └── app.go                  # SetupRouter tái sử dụng chung cho cả Server & Lambda
│   ├── config/
│   │   └── config.go               # Cấu hình hệ thống, Cloudflare R2, DB Pool, Lambda
│   ├── database/
│   │   ├── postgres.go             # Kết nối PostgreSQL, Auto-migrate, Pool tuning
│   │   └── seed.go                 # Tự động seed 9 danh mục hệ thống mặc định
│   ├── handler/                    # HTTP Handlers (Controller)
│   │   ├── auth_handler.go         # Đăng ký, Đăng nhập, Profile /me
│   │   ├── settings_handler.go     # Cấu hình người dùng & cập nhật avatar/role
│   │   ├── category_handler.go     # Quản lý danh mục
│   │   ├── box_handler.go          # Thùng / Vị trí lưu trữ, Di chuyển cây đa cấp
│   │   ├── item_handler.go         # Danh mục tài sản, lọc, tìm kiếm, cập nhật kho
│   │   ├── lending_handler.go      # Mượn/trả đồ, cảnh báo quá hạn, lịch sử mượn
│   │   ├── scan_handler.go         # Quét Barcode & QR Code thông minh
│   │   ├── alert_handler.go        # Tổng hợp cảnh báo & Dashboard badges
│   │   ├── report_handler.go       # Báo cáo sức khỏe kho & Xuất CSV / JSON
│   │   ├── media_handler.go        # Tải lên ảnh đồ đạc / hóa đơn
│   │   └── response.go             # Chuẩn hóa Envelope phản hồi JSON
│   ├── storage/                    # Object Storage Layer
│   │   ├── storage.go              # StorageProvider (Local Driver & Cloudflare R2 Driver)
│   │   └── storage_test.go         # Unit test cho storage driver
│   ├── response/
│   │   └── response.go             # Standard Response Envelope (Data, Meta, Error)
│   ├── middleware/
│   │   ├── auth.go                 # Xác thực Bearer JWT Token
│   │   └── cors.go                 # Cấu hình CORS
│   ├── models/                     # GORM Database Models
│   │   ├── user.go                 # Bảng users
│   │   ├── user_settings.go        # Bảng user_settings
│   │   ├── category.go             # Bảng categories
│   │   ├── box.go                  # Bảng boxes (cây thư mục đa cấp)
│   │   ├── item.go                 # Bảng items (ngưỡng tồn kho, hạn dùng, bảo hành)
│   │   └── lending.go              # Bảng lending_records
│   ├── repository/                 # Database Query Layer
│   │   ├── user_repo.go
│   │   ├── settings_repo.go
│   │   ├── category_repo.go
│   │   ├── box_repo.go             # CTE đệ quy tra cứu breadcrumbs & kiểm tra chu trình
│   │   ├── item_repo.go            # Lọc đa tiêu chí & tìm kiếm mờ
│   │   └── lending_repo.go
│   ├── service/                    # Business Logic Layer
│   │   ├── auth_service.go
│   │   ├── settings_service.go
│   │   ├── category_service.go
│   │   ├── box_service.go          # Chống lặp vòng (Cycle Prevention)
│   │   ├── item_service.go         # Tự động tính isLowStock
│   │   ├── lending_service.go      # Ràng buộc trạng thái mượn đồ
│   │   ├── scan_service.go         # Phân giải thông minh mã vạch/QR
│   │   ├── alert_service.go        # Tổng hợp cảnh báo hạn dùng, kho, quá hạn
│   │   ├── report_service.go       # Thống kê giá trị tài sản & xuất file
│   │   └── media_service.go        # Upload media ủy quyền cho StorageProvider
│   └── utils/
│       ├── jwt.go                  # Tạo và giải mã JWT
│       └── password.go             # Bcrypt hashing
├── uploads/                        # Thư mục lưu trữ media tĩnh (khi dùng STORAGE_DRIVER=local)
├── docker-compose.yml              # Docker Compose cho PostgreSQL & API
├── Dockerfile                      # Multi-stage Docker build
├── Makefile                        # Phím tắt lệnh tiện lợi (build, run, package-lambda)
├── .env.example                    # Mẫu biến môi trường đầy đủ
├── .env                            # Cấu hình dev
├── go.mod
├── go.sum
└── README.md
```

---

## 3. Cấu hình Môi trường (.env)

Hệ thống hỗ trợ 2 driver lưu trữ ảnh: `local` (lưu thư mục ổ cứng) hoặc `r2` (Cloudflare R2 Object Storage).

```env
# Server
PORT=8080
APP_ENV=development
BASE_URL=http://localhost:8080

# Database (PostgreSQL / AWS RDS / Neon / Supabase)
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=postgres
DB_NAME=inventory_db
DB_SSLMODE=disable

# Connection Pool (Tối ưu cho Lambda: 2-5, hoặc Server: 25)
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=5
RUN_MIGRATIONS=true

# JWT Auth
JWT_SECRET=super_secret_jwt_key_change_in_production_1234567890
JWT_EXPIRATION_HOURS=720

# Storage Driver: "local" hoặc "r2"
STORAGE_DRIVER=local
UPLOAD_DIR=./uploads

# Cloudflare R2 (Bật khi STORAGE_DRIVER=r2)
R2_ACCOUNT_ID=your_cloudflare_account_id
R2_ACCESS_KEY_ID=your_r2_access_key_id
R2_SECRET_ACCESS_KEY=your_r2_secret_access_key
R2_BUCKET_NAME=inventory-media
R2_PUBLIC_URL=https://pub-xxxxxx.r2.dev
```

---

## 4. Hướng dẫn Cloudflare R2 cho Ảnh (Media Storage)

Cloudflare R2 cung cấp chuẩn S3 API tương thích 100%, **miễn phí 10GB lưu trữ và hoàn toàn không tính phí băng thông tải về (Zero Egress Fees)**.

### Các bước lấy thông tin R2:
1. Đăng nhập [Cloudflare Dashboard](https://dash.cloudflare.com/) > chọn **R2 Object Storage**.
2. **Tạo Bucket:** Đặt tên bucket (ví dụ: `inventory-media`).
3. **Bật Public Access:**
   - Trong trang Bucket > Settings > **Public Access**.
   - Bật **R2.dev subdomain** (ví dụ: `https://pub-xxxx.r2.dev`) HOẶC kết nối **Custom Domain** (ví dụ: `https://media.yourdomain.com`).
   - Gán URL này vào biến `R2_PUBLIC_URL`.
4. **Tạo API Token:**
   - Tại trang R2 Overview > Chọn **Manage R2 API Tokens** > **Create API Token**.
   - Permissions: Chọn **Object Read & Write**.
   - Lưu lại `Access Key ID` và `Secret Access Key`.
   - `Account ID` hiển thị ở cột bên phải giao diện Overview của R2.
5. Cập nhật các giá trị vào file `.env` hoặc cấu hình Environment Variables trên AWS Lambda:
   ```env
   STORAGE_DRIVER=r2
   R2_ACCOUNT_ID=xxxxxx
   R2_ACCESS_KEY_ID=xxxxxx
   R2_SECRET_ACCESS_KEY=xxxxxx
   R2_BUCKET_NAME=inventory-media
   R2_PUBLIC_URL=https://pub-xxxx.r2.dev
   ```

---

## 5. Triển khai lên AWS Lambda

Backend đã được tích hợp sẵn adapter `UniversalHandler` trong [`cmd/lambda/main.go`](./cmd/lambda/main.go), hỗ trợ đồng thời cả **API Gateway HTTP API v2**, **API Gateway REST API v1**, và **Lambda Function URLs**.

### 5.1. Thiết lập Lambda Function trên AWS
1. Tạo một Lambda Function mới:
   - **Runtime:** `Custom runtime on Amazon Linux 2023` (`provided.al2023`)
   - **Architecture:** `arm64` (Khuyến nghị dùng Graviton để tiết kiệm 20% chi phí và hiệu năng cao hơn) hoặc `x86_64`.
   - **Handler:** `bootstrap`
2. Cấu hình biến môi trường (Environment Variables) trên Lambda:
   - `APP_ENV=production`
   - `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, `DB_SSLMODE` (kết nối tới AWS RDS PostgreSQL, Supabase, Neon...)
   - `DB_MAX_OPEN_CONNS=5`, `DB_MAX_IDLE_CONNS=2`
   - `JWT_SECRET=...`
   - `STORAGE_DRIVER=r2` và các biến `R2_*`
3. Tạo API Gateway (HTTP API v2) tích hợp Lambda với route `$default` trỏ vào Function.

### 5.2. Đóng gói thủ công bằng Makefile
```bash
# Biên dịch file bootstrap cho Linux ARM64 và nén thành deployment.zip
make package-lambda

# Deploy nhanh bằng AWS CLI (nếu có sẵn)
aws lambda update-function-code \
  --function-name your-inventory-lambda-function \
  --zip-file fileb://bin/deployment.zip
```

### 5.3. Tự động hóa CI/CD với GitHub Actions
File workflow đã được tạo sẵn tại [`.github/workflows/deploy.yml`](./.github/workflows/deploy.yml).

Chỉ cần cấu hình các **Repository Secrets** trong GitHub (Settings > Secrets and variables > Actions):
- `AWS_ACCESS_KEY_ID`: IAM Access Key có quyền `lambda:UpdateFunctionCode`
- `AWS_SECRET_ACCESS_KEY`: IAM Secret Access Key
- `AWS_REGION`: Vùng AWS của function (ví dụ: `ap-southeast-1`, `us-east-1`)
- `AWS_LAMBDA_FUNCTION_NAME`: Tên Lambda Function trên AWS

Mỗi khi bạn push code lên nhánh `main` (hoặc nhấn **Run workflow** thủ công qua tab Actions), GitHub Actions sẽ tự động:
1. Chạy toàn bộ Unit Tests.
2. Build binary `bootstrap` tối ưu (`-ldflags="-s -w"`).
3. Đóng gói `deployment.zip`.
4. Deploy trực tiếp lên AWS Lambda và chờ function cập nhật hoàn tất.

---

## 6. Chạy thử nghiệm cục bộ (Local Development)

### Cách 1: Chạy trực tiếp (Local DB Docker)
```bash
# 1. Khởi động DB
make db-up

# 2. Chạy ứng dụng
make run
```
API lắng nghe tại `http://localhost:8080`.

### Cách 2: Chạy toàn bộ qua Docker Compose
```bash
make docker-up
```

### Chạy Unit Tests
```bash
make test
```

---

## 7. Danh sách API Endpoints

- `GET /health` - Health check (trả về trạng thái, phiên bản, driver storage `local` hoặc `r2`)
- **Authentication:** `POST /api/v1/auth/register`, `POST /api/v1/auth/login`, `GET /api/v1/auth/me`
- **Settings & Profile:** `GET /api/v1/settings`, `PATCH /api/v1/settings`, `PATCH /api/v1/user/profile`
- **Categories:** `GET /api/v1/categories`, `POST /api/v1/categories`, `DELETE /api/v1/categories/:name`
- **Boxes (Kho & Thùng):** `GET /api/v1/boxes`, `POST /api/v1/boxes`, `GET /api/v1/boxes/:id`, `PUT /api/v1/boxes/:id`, `PATCH /api/v1/boxes/:id/move`, `DELETE /api/v1/boxes/:id`
- **Items (Tài sản):** `GET /api/v1/items`, `POST /api/v1/items`, `GET /api/v1/items/:id`, `PUT /api/v1/items/:id`, `PATCH /api/v1/items/:id/move`, `PATCH /api/v1/items/:id/quantity`, `DELETE /api/v1/items/:id`
- **Lending Tracker:** `POST /api/v1/lending/lend`, `POST /api/v1/lending/:id/return`, `GET /api/v1/lending/active`, `GET /api/v1/lending/overdue`, `GET /api/v1/lending/history`
- **Barcode & QR Scan:** `GET /api/v1/scan/lookup?code=...`
- **Alerts & Dashboard:** `GET /api/v1/alerts/summary`, `GET /api/v1/alerts/all`
- **Reports:** `GET /api/v1/reports/inventory-health`, `GET /api/v1/reports/export`
- **Media Upload:** `POST /api/v1/media/upload` (hỗ trợ upload trực tiếp lên Cloudflare R2 hoặc Local)
