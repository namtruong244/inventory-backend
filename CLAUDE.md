# Backend — Go / Gin / GORM / PostgreSQL

Module `inventory_backend` (go.mod: `go 1.26`). Đặc tả đầy đủ endpoint, schema và DTO mẫu: [AGENTS.md](AGENTS.md) — đọc phần liên quan trước khi thêm/đổi endpoint. Rule chung BE↔FE: [../CLAUDE.md](../CLAUDE.md).

## Lệnh thường dùng

```bash
make db-up            # chỉ chạy PostgreSQL (docker compose)
make run              # go run ./cmd/api
make test             # go test -v ./...
go test ./internal/service/ -run TestBoxService   # chạy 1 nhóm test
make build            # bin/server
make package-lambda   # bin/bootstrap + deployment.zip (linux/arm64)
make docker-up        # toàn bộ stack qua Docker Compose
go vet ./... && gofmt -l .                         # kiểm tra trước khi kết thúc
```
Config đọc từ `.env` (copy từ `.env.example`) qua `internal/config`.

## Kiến trúc (layered, mỗi tầng một package)

```
cmd/api/main.go        HTTP server + graceful shutdown
cmd/lambda/main.go     AWS Lambda adapter (API Gateway v1/v2, Function URL)
internal/app/app.go    SetupRouter: DI thủ công repo → service → handler + toàn bộ route
internal/handler/      Gin handler: parse/validate request, gọi service, trả envelope
internal/service/      Business logic; interface + struct private + NewXxxService
internal/repository/   Truy vấn GORM; interface + struct private + NewXxxRepository
internal/models/       GORM model (cũng là JSON response), hooks BeforeCreate/AfterFind
internal/middleware/   AuthMiddleware (JWT), CORS, GetCurrentUserID
internal/response/     SendSuccess / SendSuccessWithMeta / SendError (nguồn gốc)
internal/storage/      StorageProvider: local (./uploads) hoặc R2/S3
internal/database/     InitDB, AutoMigrate, index, seed danh mục mặc định
internal/utils/        JWT, bcrypt password
```

- **Thêm route chỉ ở `internal/app/app.go`** — cả HTTP server và Lambda dùng chung `SetupRouter`, không sửa routing trong `cmd/`.
- Luồng phụ thuộc một chiều: handler → service → repository → models. Handler không gọi repository trực tiếp (ngoại lệ hiện có: dùng type `repository.ItemFilter`).
- Service/Repository luôn khai báo **interface public + implementation private** và constructor trả về interface — để test dùng mock (xem `internal/service/box_service_test.go`).
- `handler/response.go` chỉ alias lại `internal/response`. Trong handler dùng `SendSuccess/SendError`; middleware import `internal/response` trực tiếp (tránh import cycle).

## Quy ước code

- **Mỗi query repository phải lọc theo `user_id`** (`Where("id = ? AND user_id = ?", id, userID)`). Signature repo nhận `userID uuid.UUID`.
- Handler mẫu: lấy `userID, ok := middleware.GetCurrentUserID(c)` → nếu `!ok` trả 401 `UNAUTHORIZED` → `ShouldBindJSON` lỗi trả 400 `INVALID_INPUT` → gọi service → map lỗi sang code UPPER_SNAKE (`CREATE_FAILED`, `LEND_FAILED`, `NOT_FOUND`, `INTERNAL_ERROR`...).
- Service trả lỗi có loại bằng `internal/apperr` (`apperr.Invalid/NotFound/Conflict`); handler gọi `fail(c, "X_FAILED", err)` (`internal/handler/errors.go`) để map sang 400/404/409/500. Message của lỗi có loại đi ra `error.message`; lỗi khác trả 500 chung và chỉ log chi tiết.
- Request DTO đặt trong file service (`CreateItemRequest`, `UpdateItemRequest`) hoặc handler (`MoveItemRequest`). Update dùng field con trỏ (`*string`, `*float64`) để phân biệt "không gửi" và "giá trị zero".
- List luôn trả mảng rỗng `[]` chứ không phải `null`; slice/map JSONB phải được khởi tạo (model hooks đã lo khi đọc/tạo).
- Field ảo cho response (`IsLowStock`, `BoxName`) dùng tag `gorm:"-"`; `IsLowStock` được tính trong `Item.AfterFind` — dùng `ComputeIsLowStock()`, đừng tính lại ở nơi khác.
- Thời gian DB ở UTC (`NowFunc` UTC, DSN `TimeZone=UTC`).

## Khác biệt giữa code hiện tại và AGENTS.md (code là chân lý)

- `purchaseDate`, `warrantyExpiryDate`, `expiryDate` của item lưu dạng **chuỗi `varchar(30)`** (`*string`), không phải TIMESTAMPTZ; service chuẩn hoá về `YYYY-MM-DD` khi ghi (`normalizeDate`).
- Item có thêm `isConsumable bool` (chưa có trong spec).
- Không có file SQL migration: schema do `db.AutoMigrate` + các lệnh `ALTER`/`CREATE INDEX IF NOT EXISTS` trong `internal/database/postgres.go` (bật khi `RUN_MIGRATIONS=true`). Thêm cột/index mới → thêm vào model và, nếu cần, vào khối raw SQL đó (phải idempotent). DDL trong AGENTS.md chỉ để tham khảo.
- `boxes.label` duy nhất **theo từng user**: service kiểm tra bằng `GetByLabel(label, &userID)` và có unique index `uq_boxes_user_label (user_id, label)` (nếu DB cũ có nhãn trùng thì index không tạo được, chỉ log warning — cần dọn dữ liệu rồi chạy lại).
- Lending dùng transaction + `SELECT ... FOR UPDATE` trong `LendingRepository.LendItem/ReturnLoan` và partial unique index `uq_lending_open_item` (mỗi item tối đa 1 khoản mượn đang mở). `GET /items` giới hạn `limit` ≤ 200 (FE dùng `getAllItems()` để phân trang).
- Item status có thêm `lost` (không cho mượn, không tính vào tổng giá trị). Settings có `warrantyWarningDays` và `timezone` (IANA); "hôm nay" tính bằng `utils.TodayIn(tz)`.
- `categoryId` của item lưu **tên** danh mục; `ItemService` kiểm tra qua `CategoryRepository.Resolve`, xoá danh mục đang dùng trả 409.
- `stock_movements` ghi lịch sử số lượng (best-effort, không làm hỏng thao tác chính); `GET /items/:id/movements`.
- Index trigram (`pg_trgm`) cho tìm kiếm tên/mô tả là best-effort: thiếu extension chỉ log cảnh báo.
- Lambda dựng DB một lần mỗi cold start (`init()` trong `cmd/lambda/main.go`), không mở lại mỗi request.
- Upload (`/media/upload`): tối đa 10 MB, nội dung file được sniff và phải khớp đuôi file.

## Auth

- JWT HS256 (`golang-jwt/v5`), claims gồm `UserID`, `Email`, `Role`; middleware set vào context key `userID`.
- Google login (không còn Apple): **bắt buộc** `id_token`, verify với Google (timeout 5s, kiểm tra `aud`/`azp` = `GOOGLE_CLIENT_ID` và `email_verified`); email lấy từ token, không tin email trong body. Token `mock_*` chỉ được chấp nhận khi `APP_ENV=test`. User mới → tạo `user_settings` + danh mục mặc định; user đã tồn tại → link provider, giữ tên cũ.
- `APP_ENV=production` bắt buộc `JWT_SECRET` riêng, ≥ 32 ký tự (`config.Validate()` sẽ `log.Fatalf` nếu để mặc định). `CORS_ALLOWED_ORIGINS` (CSV): trống = cho phép mọi origin ở dev, tắt CORS ở production; không bao giờ bật credentials.
- `password_hash` nullable (user social login).

## Test

- Unit test cạnh file nguồn (`*_test.go`), package `xxx_test`, mock repository bằng struct in-memory tự viết — không có DB thật, không dùng thư viện mock.
- Logic mới ở service (đặc biệt cycle prevention, lending integrity, low stock) phải có test.

- Test tích hợp Postgres thật: `internal/app/integration_test.go`, chỉ chạy khi có `INTEGRATION_DB_NAME` (dùng DB tạm): `INTEGRATION_DB_NAME=inventory_it go test ./internal/app/`.

## Deploy

- `.github/workflows/ci.yml`: PR/branch khác main chạy gofmt, vet, test.
- `.github/workflows/deploy.yml`: push `main`/`master` → `go test` → build `cmd/lambda` → `aws lambda update-function-code`. Đừng push khi chưa được yêu cầu.
- Lambda: giữ `DB_MAX_OPEN_CONNS` thấp (2–5); storage nên là `r2` vì filesystem Lambda không bền.
