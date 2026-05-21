# LMS Gateway UI

UI test nhanh cho GraphQL gateway — Vite + React, không cần build tool gì thêm.

## Chạy

```bash
cd ui
npm install      # lần đầu
npm run dev      # → http://localhost:5173
```

Gateway phải đang chạy (`./scripts/run-lms.sh`). Vite proxy `/query` →
`localhost:8080` nên **không cần CORS, không sửa gateway**.

## Tính năng

- **Đổi role** (Khách / Student / Teacher / Admin) — JWT ký ngay trong trình
  duyệt (HS256, secret `lms-dev-secret`).
- **Preset truy vấn** — Đọc (list các entity, cross-service) + Ghi (tạo lớp,
  ghi danh, xoá user, `runAction`).
- **Console GraphQL** — gõ query/mutation bất kỳ, chạy với role đang chọn.
- Kết quả hiện dạng bảng (list) hoặc JSON; lỗi authz / business-rule hiện đỏ.

Đổi role rồi chạy lại cùng một mutation để thấy `@auth` / business rule chặn.
