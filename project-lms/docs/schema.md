# LMS Database Schema (MySQL 8) — grpc-gen compliant

> Charset: `utf8mb4` / Collation: `utf8mb4_unicode_ci`
> Engine: `InnoDB`
> Schema này được thiết kế khớp 100% với convention của tool **grpc-gen** (tham chiếu `/home/thaily/code/grpc-gen`).

---

## 0. Conventions (bắt buộc do tool quy định)

### 0.1 Primary key
- Mọi bảng có **duy nhất 1 cột PK**: `id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY`.
- Proto: `uint64 id = 1;` ở entity message.
- Client có thể supply `id` ở `CreateRequest` (hiếm dùng) hoặc để MySQL `AUTO_INCREMENT` (mặc định).

### 0.2 Audit columns (bắt buộc mọi bảng)
```sql
created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
created_by VARCHAR(100) NULL,
updated_by VARCHAR(100) NULL
```
Tool tự sinh code Create/Update set 4 cột này. `created_by`/`updated_by` lưu **service-level audit** (ai gọi API, có thể là user_id dạng string hoặc tên service); tách biệt với business FK như `teacher_id`, `owner_teacher_id`, `actor_id`.

### 0.3 Không hỗ trợ
| Không có | Lý do | Workaround |
|---|---|---|
| Soft delete (`deleted_at`) | Tool gen HARD DELETE | Dùng `status ENUM` (vd: `active`/`archived`) |
| Composite PK | Tool yêu cầu PK đơn `id` | Thêm `id` surrogate + UNIQUE constraint |
| `DECIMAL(p,s)` | Tool không có Go type tương đương | Dùng `DOUBLE` (chấp nhận float precision) |
| `JSON` column | Tool chưa support | `MEDIUMTEXT` lưu JSON string, app encode/decode |
| Foreign key constraints | Tool không gen FK | Tự thêm vào migration nếu muốn |
| Transaction | Tool không gen multi-statement TX | Hand-write handler nếu cần (vd: join class flow) |

### 0.4 ENUM strategy
- MySQL: `VARCHAR(20)` (lưu lowercase string)
- Proto: `enum` với value đầu tiên = `0` (proto3 default)
- Tool tự convert hai chiều: enum value `ACTIVE` ↔ DB string `"active"`

### 0.5 Proto convention
- Entity message liệt kê **đủ field** của table.
- `CreateXxxRequest` cũng phải có **đủ field** entity. Field nào không bắt buộc lúc Create (vd `email_verified_at`, `last_login_at`) → đánh `optional`.
- `UpdateXxxRequest` chỉ có field nào cho phép update, dạng `optional`.
- Field nhạy cảm (password, secret) đánh `[(common.filterable) = false]` để chặn khỏi WHERE filter.

### 0.6 Default audit cho mọi entity
Mỗi `CreateXxxRequest` cũng phải có:
- `string created_by = N;` (required — để tracking)

Mỗi `UpdateXxxRequest` cũng phải có:
- `string updated_by = N;` (required)

---

## 1. Service decomposition

| # | Service | Port | Tables | Module path suffix |
|---|---|---|---|---|
| 1 | `user` | 50051 | users, teacher_profiles, student_profiles, refresh_tokens, password_resets | `proto/user` |
| 2 | `class` | 50052 | classes, class_invite_keys, enrollments | `proto/class` |
| 3 | `lesson` | 50053 | lessons, lesson_videos, lesson_attendance | `proto/lesson` |
| 4 | `material` | 50054 | materials, material_classes, material_tags | `proto/material` |
| 5 | `exam` | 50055 | exams, exam_assignments, questions, question_choices, question_answers | `proto/exam` |
| 6 | `submission` | 50056 | exam_attempts, attempt_answers | `proto/submission` |
| 7 | `notification` | 50057 | notifications, community_reports | `proto/notification` |
| 8 | `audit` | 50058 | audit_logs | `proto/audit` |

Tổng: **21 bảng**.

---

## 2. User service (50051)

### 2.1 `users`
```sql
CREATE TABLE users (
  id                BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  email             VARCHAR(255) NOT NULL,
  phone             VARCHAR(20) NULL,
  password_hash     VARCHAR(255) NOT NULL,
  full_name         VARCHAR(120) NOT NULL,
  avatar_url        VARCHAR(500) NULL,
  role              VARCHAR(20) NOT NULL,
  status            VARCHAR(20) NOT NULL DEFAULT 'pending_verify',
  email_verified_at TIMESTAMP NULL,
  last_login_at     TIMESTAMP NULL,
  created_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  created_by        VARCHAR(100) NULL,
  updated_by        VARCHAR(100) NULL,
  UNIQUE KEY uk_users_email (email),
  UNIQUE KEY uk_users_phone (phone),
  INDEX idx_users_role_status (role, status)
);
```
- Enum `role`: `admin` | `teacher` | `student`
- Enum `status`: `active` | `banned` | `pending_verify`
- Block filter: `password_hash`

### 2.2 `teacher_profiles`
```sql
CREATE TABLE teacher_profiles (
  id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  user_id      BIGINT UNSIGNED NOT NULL,
  bio          MEDIUMTEXT NULL,
  subjects     MEDIUMTEXT NULL,           -- JSON-as-string: ["Toán","Vật lý"]
  years_exp    INT NULL,
  facebook_url VARCHAR(500) NULL,
  created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  created_by   VARCHAR(100) NULL,
  updated_by   VARCHAR(100) NULL,
  UNIQUE KEY uk_teacher_profile_user (user_id)
);
```

### 2.3 `student_profiles`
```sql
CREATE TABLE student_profiles (
  id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  user_id      BIGINT UNSIGNED NOT NULL,
  grade        VARCHAR(20) NULL,
  school_name  VARCHAR(200) NULL,
  parent_name  VARCHAR(120) NULL,
  parent_phone VARCHAR(20) NULL,
  created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  created_by   VARCHAR(100) NULL,
  updated_by   VARCHAR(100) NULL,
  UNIQUE KEY uk_student_profile_user (user_id)
);
```

### 2.4 `refresh_tokens`
```sql
CREATE TABLE refresh_tokens (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  user_id     BIGINT UNSIGNED NOT NULL,
  token_hash  CHAR(64) NOT NULL,
  user_agent  VARCHAR(500) NULL,
  ip_address  VARCHAR(45) NULL,
  expires_at  TIMESTAMP NOT NULL,
  revoked_at  TIMESTAMP NULL,
  created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  created_by  VARCHAR(100) NULL,
  updated_by  VARCHAR(100) NULL,
  UNIQUE KEY uk_refresh_token_hash (token_hash),
  INDEX idx_rt_user (user_id, revoked_at)
);
```
- Block filter: `token_hash`

### 2.5 `password_resets`
```sql
CREATE TABLE password_resets (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  user_id     BIGINT UNSIGNED NOT NULL,
  token_hash  CHAR(64) NOT NULL,
  expires_at  TIMESTAMP NOT NULL,
  used_at     TIMESTAMP NULL,
  created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  created_by  VARCHAR(100) NULL,
  updated_by  VARCHAR(100) NULL,
  UNIQUE KEY uk_pwreset_token_hash (token_hash),
  INDEX idx_pwreset_user (user_id, used_at)
);
```
- Block filter: `token_hash`

---

## 3. Class service (50052)

### 3.1 `classes`
```sql
CREATE TABLE classes (
  id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  teacher_id   BIGINT UNSIGNED NOT NULL,
  name         VARCHAR(200) NOT NULL,
  subject      VARCHAR(100) NULL,
  description  MEDIUMTEXT NULL,
  cover_url    VARCHAR(500) NULL,
  status       VARCHAR(20) NOT NULL DEFAULT 'active',
  max_students INT NULL,
  created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  created_by   VARCHAR(100) NULL,
  updated_by   VARCHAR(100) NULL,
  INDEX idx_classes_teacher (teacher_id, status)
);
```
- Enum `status`: `draft` | `active` | `archived`

### 3.2 `class_invite_keys`
```sql
CREATE TABLE class_invite_keys (
  id                    BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  class_id              BIGINT UNSIGNED NOT NULL,
  key_code              VARCHAR(32) NOT NULL,
  created_by_teacher_id BIGINT UNSIGNED NOT NULL,
  note                  VARCHAR(200) NULL,
  expires_at            TIMESTAMP NULL,
  used_at               TIMESTAMP NULL,
  used_by_student_id    BIGINT UNSIGNED NULL,
  status                VARCHAR(20) NOT NULL DEFAULT 'active',
  created_at            TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at            TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  created_by            VARCHAR(100) NULL,
  updated_by            VARCHAR(100) NULL,
  UNIQUE KEY uk_invite_key_code (key_code),
  INDEX idx_invite_keys_class_status (class_id, status),
  INDEX idx_invite_keys_lookup (key_code, status)
);
```
- Enum `status`: `active` | `used` | `revoked` | `expired`
- **Note**: Logic "join class atomically" cần transaction → hand-write handler sau, basic CRUD tool gen được CRUD đơn giản trước.

### 3.3 `enrollments`
```sql
CREATE TABLE enrollments (
  id                 BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  class_id           BIGINT UNSIGNED NOT NULL,
  student_id         BIGINT UNSIGNED NOT NULL,
  invite_key_id      BIGINT UNSIGNED NULL,
  joined_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  status             VARCHAR(20) NOT NULL DEFAULT 'active',
  removed_at         TIMESTAMP NULL,
  removed_by_user_id BIGINT UNSIGNED NULL,
  created_at         TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at         TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  created_by         VARCHAR(100) NULL,
  updated_by         VARCHAR(100) NULL,
  UNIQUE KEY uk_enrollment_active (class_id, student_id),
  INDEX idx_enrollments_student (student_id, status)
);
```
- Enum `status`: `active` | `removed`

---

## 4. Lesson service (50053)

### 4.1 `lessons`
```sql
CREATE TABLE lessons (
  id           BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  class_id     BIGINT UNSIGNED NOT NULL,
  title        VARCHAR(200) NOT NULL,
  description  MEDIUMTEXT NULL,
  start_at     DATETIME NOT NULL,
  end_at       DATETIME NOT NULL,
  mode         VARCHAR(20) NOT NULL DEFAULT 'offline',
  location     VARCHAR(300) NULL,
  meeting_url  VARCHAR(500) NULL,
  status       VARCHAR(20) NOT NULL DEFAULT 'scheduled',
  created_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  created_by   VARCHAR(100) NULL,
  updated_by   VARCHAR(100) NULL,
  INDEX idx_lessons_class_start (class_id, start_at)
);
```
- Enum `mode`: `offline` | `online` | `hybrid`
- Enum `status`: `scheduled` | `ongoing` | `done` | `cancelled`

### 4.2 `lesson_videos`
```sql
CREATE TABLE lesson_videos (
  id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  lesson_id     BIGINT UNSIGNED NOT NULL,
  title         VARCHAR(200) NOT NULL,
  storage_key   VARCHAR(500) NOT NULL,
  duration_sec  INT NULL,
  size_bytes    BIGINT NULL,
  mime_type     VARCHAR(100) NULL,
  thumbnail_key VARCHAR(500) NULL,
  status        VARCHAR(20) NOT NULL DEFAULT 'uploading',
  uploaded_at   TIMESTAMP NULL,
  created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  created_by    VARCHAR(100) NULL,
  updated_by    VARCHAR(100) NULL,
  INDEX idx_videos_lesson (lesson_id)
);
```
- Enum `status`: `uploading` | `processing` | `ready` | `failed`

### 4.3 `lesson_attendance`
> Original schema dùng composite PK `(lesson_id, student_id)` — đổi sang surrogate `id` + UNIQUE.

```sql
CREATE TABLE lesson_attendance (
  id                    BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  lesson_id             BIGINT UNSIGNED NOT NULL,
  student_id            BIGINT UNSIGNED NOT NULL,
  status                VARCHAR(20) NOT NULL,
  note                  VARCHAR(300) NULL,
  marked_by_teacher_id  BIGINT UNSIGNED NOT NULL,
  marked_at             TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_at            TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at            TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  created_by            VARCHAR(100) NULL,
  updated_by            VARCHAR(100) NULL,
  UNIQUE KEY uk_lesson_attendance (lesson_id, student_id)
);
```
- Enum `status`: `present` | `absent` | `late` | `excused`

---

## 5. Material service (50054)

### 5.1 `materials`
```sql
CREATE TABLE materials (
  id                BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  owner_teacher_id  BIGINT UNSIGNED NOT NULL,
  title             VARCHAR(200) NOT NULL,
  description       MEDIUMTEXT NULL,
  storage_key       VARCHAR(500) NOT NULL,
  file_name         VARCHAR(255) NOT NULL,
  file_type         VARCHAR(50) NOT NULL,
  size_bytes        BIGINT NOT NULL,
  visibility        VARCHAR(20) NOT NULL,
  community_status  VARCHAR(20) NULL,
  reject_reason     MEDIUMTEXT NULL,
  download_count    INT UNSIGNED NOT NULL DEFAULT 0,
  created_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  created_by        VARCHAR(100) NULL,
  updated_by        VARCHAR(100) NULL,
  INDEX idx_materials_owner (owner_teacher_id, visibility),
  INDEX idx_materials_community (visibility, community_status)
);
```
- Enum `visibility`: `class` | `community`
- Enum `community_status`: `draft` | `pending` | `approved` | `rejected`

### 5.2 `material_classes`
> Composite PK đổi sang surrogate.

```sql
CREATE TABLE material_classes (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  material_id BIGINT UNSIGNED NOT NULL,
  class_id    BIGINT UNSIGNED NOT NULL,
  attached_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  created_by  VARCHAR(100) NULL,
  updated_by  VARCHAR(100) NULL,
  UNIQUE KEY uk_material_class (material_id, class_id),
  INDEX idx_mat_class (class_id)
);
```

### 5.3 `material_tags`
> Composite PK đổi sang surrogate.

```sql
CREATE TABLE material_tags (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  material_id BIGINT UNSIGNED NOT NULL,
  tag         VARCHAR(50) NOT NULL,
  created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  created_by  VARCHAR(100) NULL,
  updated_by  VARCHAR(100) NULL,
  UNIQUE KEY uk_material_tag (material_id, tag),
  INDEX idx_mat_tag (tag)
);
```

---

## 6. Exam service (50055)

### 6.1 `exams`
```sql
CREATE TABLE exams (
  id                BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  owner_teacher_id  BIGINT UNSIGNED NOT NULL,
  title             VARCHAR(200) NOT NULL,
  description       MEDIUMTEXT NULL,
  subject           VARCHAR(100) NULL,
  duration_min      INT NOT NULL,
  total_points      DOUBLE NOT NULL DEFAULT 0,
  visibility        VARCHAR(20) NOT NULL,
  community_status  VARCHAR(20) NULL,
  reject_reason     MEDIUMTEXT NULL,
  shuffle_questions BOOLEAN NOT NULL DEFAULT FALSE,
  shuffle_choices   BOOLEAN NOT NULL DEFAULT FALSE,
  show_result_mode  VARCHAR(20) NOT NULL DEFAULT 'immediately',
  created_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  created_by        VARCHAR(100) NULL,
  updated_by        VARCHAR(100) NULL,
  INDEX idx_exams_owner (owner_teacher_id, visibility),
  INDEX idx_exams_community (visibility, community_status)
);
```
- Enum `visibility`: `class` | `community`
- Enum `community_status`: `draft` | `pending` | `approved` | `rejected`
- Enum `show_result_mode`: `immediately` | `after_close` | `manual`
- `total_points`: DECIMAL → DOUBLE (mất chính xác sau dấu phẩy nhưng tool support).

### 6.2 `exam_assignments`
```sql
CREATE TABLE exam_assignments (
  id            BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  exam_id       BIGINT UNSIGNED NOT NULL,
  class_id      BIGINT UNSIGNED NOT NULL,
  open_at       DATETIME NOT NULL,
  close_at      DATETIME NOT NULL,
  max_attempts  INT UNSIGNED NOT NULL DEFAULT 1,
  created_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at    TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  created_by    VARCHAR(100) NULL,
  updated_by    VARCHAR(100) NULL,
  UNIQUE KEY uk_exam_class (exam_id, class_id),
  INDEX idx_assign_class_window (class_id, open_at, close_at)
);
```
- `max_attempts`: original schema dùng `TINYINT UNSIGNED` → tool chưa support uint8 → dùng `INT UNSIGNED` (proto `uint32`).

### 6.3 `questions`
```sql
CREATE TABLE questions (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  exam_id     BIGINT UNSIGNED NOT NULL,
  position    INT NOT NULL,
  type        VARCHAR(20) NOT NULL,
  content     MEDIUMTEXT NOT NULL,
  image_key   VARCHAR(500) NULL,
  points      DOUBLE NOT NULL DEFAULT 1,
  explanation MEDIUMTEXT NULL,
  created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  created_by  VARCHAR(100) NULL,
  updated_by  VARCHAR(100) NULL,
  INDEX idx_questions_exam (exam_id, position)
);
```
- Enum `type`: `single` | `multiple` | `short_answer` | `essay`

### 6.4 `question_choices`
```sql
CREATE TABLE question_choices (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  question_id BIGINT UNSIGNED NOT NULL,
  position    INT NOT NULL,
  content     MEDIUMTEXT NOT NULL,
  is_correct  BOOLEAN NOT NULL DEFAULT FALSE,
  created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  created_by  VARCHAR(100) NULL,
  updated_by  VARCHAR(100) NULL,
  INDEX idx_choices_q (question_id, position)
);
```

### 6.5 `question_answers`
```sql
CREATE TABLE question_answers (
  id              BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  question_id     BIGINT UNSIGNED NOT NULL,
  accepted_answer VARCHAR(500) NOT NULL,
  case_sensitive  BOOLEAN NOT NULL DEFAULT FALSE,
  created_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at      TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  created_by      VARCHAR(100) NULL,
  updated_by      VARCHAR(100) NULL,
  INDEX idx_answers_q (question_id)
);
```

---

## 7. Submission service (50056)

### 7.1 `exam_attempts`
```sql
CREATE TABLE exam_attempts (
  id                   BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  exam_id              BIGINT UNSIGNED NOT NULL,
  student_id           BIGINT UNSIGNED NOT NULL,
  assignment_id        BIGINT UNSIGNED NULL,
  started_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  submitted_at         TIMESTAMP NULL,
  auto_score           DOUBLE NULL,
  manual_score         DOUBLE NULL,
  total_score          DOUBLE NULL,
  status               VARCHAR(20) NOT NULL DEFAULT 'in_progress',
  graded_by_teacher_id BIGINT UNSIGNED NULL,
  graded_at            TIMESTAMP NULL,
  created_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  created_by           VARCHAR(100) NULL,
  updated_by           VARCHAR(100) NULL,
  INDEX idx_attempts_student (student_id, exam_id),
  INDEX idx_attempts_assignment (assignment_id, status)
);
```
- Enum `status`: `in_progress` | `submitted` | `graded`

### 7.2 `attempt_answers`
```sql
CREATE TABLE attempt_answers (
  id                  BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  attempt_id          BIGINT UNSIGNED NOT NULL,
  question_id         BIGINT UNSIGNED NOT NULL,
  selected_choice_ids MEDIUMTEXT NULL,   -- JSON-as-string: "[1,3]"
  text_answer         MEDIUMTEXT NULL,
  uploaded_file_key   VARCHAR(500) NULL,
  auto_score          DOUBLE NULL,
  manual_score        DOUBLE NULL,
  teacher_comment     MEDIUMTEXT NULL,
  graded_at           TIMESTAMP NULL,
  created_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at          TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  created_by          VARCHAR(100) NULL,
  updated_by          VARCHAR(100) NULL,
  UNIQUE KEY uk_attempt_question (attempt_id, question_id)
);
```

---

## 8. Notification service (50057)

### 8.1 `notifications`
```sql
CREATE TABLE notifications (
  id         BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  user_id    BIGINT UNSIGNED NOT NULL,
  type       VARCHAR(50) NOT NULL,
  title      VARCHAR(200) NOT NULL,
  body       MEDIUMTEXT NULL,
  payload    MEDIUMTEXT NULL,            -- JSON-as-string
  read_at    TIMESTAMP NULL,
  created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  created_by VARCHAR(100) NULL,
  updated_by VARCHAR(100) NULL,
  INDEX idx_notif_user_unread (user_id, read_at)
);
```

### 8.2 `community_reports`
```sql
CREATE TABLE community_reports (
  id                   BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  reporter_user_id     BIGINT UNSIGNED NOT NULL,
  target_type          VARCHAR(20) NOT NULL,
  target_id            BIGINT UNSIGNED NOT NULL,
  reason               VARCHAR(50) NOT NULL,
  note                 MEDIUMTEXT NULL,
  status               VARCHAR(20) NOT NULL DEFAULT 'open',
  resolved_by_admin_id BIGINT UNSIGNED NULL,
  resolved_at          TIMESTAMP NULL,
  created_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at           TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  created_by           VARCHAR(100) NULL,
  updated_by           VARCHAR(100) NULL,
  INDEX idx_reports_target (target_type, target_id),
  INDEX idx_reports_status (status, created_at)
);
```
- Enum `target_type`: `material` | `exam`
- Enum `status`: `open` | `dismissed` | `actioned`
- `reason` để free-form VARCHAR (giá trị gợi ý: `copyright`/`spam`/`wrong_answer`/`inappropriate`/`other`)

---

## 9. Audit service (50058)

### 9.1 `audit_logs`
```sql
CREATE TABLE audit_logs (
  id          BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY,
  actor_id    BIGINT UNSIGNED NOT NULL,
  action      VARCHAR(80) NOT NULL,
  target_type VARCHAR(50) NULL,
  target_id   BIGINT UNSIGNED NULL,
  meta        MEDIUMTEXT NULL,            -- JSON-as-string
  created_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at  TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
  created_by  VARCHAR(100) NULL,
  updated_by  VARCHAR(100) NULL,
  INDEX idx_audit_target (target_type, target_id),
  INDEX idx_audit_actor (actor_id, created_at)
);
```

---

## 10. Tổng bảng

| # | Bảng | Service | Note |
|---|---|---|---|
| 1 | `users` | user | block filter `password_hash` |
| 2 | `teacher_profiles` | user | JSON: `subjects` |
| 3 | `student_profiles` | user | |
| 4 | `refresh_tokens` | user | block filter `token_hash` |
| 5 | `password_resets` | user | block filter `token_hash` |
| 6 | `classes` | class | |
| 7 | `class_invite_keys` | class | TX cần hand-write riêng |
| 8 | `enrollments` | class | |
| 9 | `lessons` | lesson | |
| 10 | `lesson_videos` | lesson | |
| 11 | `lesson_attendance` | lesson | surrogate id (đổi từ composite) |
| 12 | `materials` | material | |
| 13 | `material_classes` | material | surrogate id |
| 14 | `material_tags` | material | surrogate id |
| 15 | `exams` | exam | DECIMAL→DOUBLE |
| 16 | `exam_assignments` | exam | TINYINT→INT |
| 17 | `questions` | exam | DECIMAL→DOUBLE |
| 18 | `question_choices` | exam | |
| 19 | `question_answers` | exam | |
| 20 | `exam_attempts` | submission | DECIMAL→DOUBLE |
| 21 | `attempt_answers` | submission | JSON: `selected_choice_ids` |
| 22 | `notifications` | notification | JSON: `payload` |
| 23 | `community_reports` | notification | |
| 24 | `audit_logs` | audit | JSON: `meta` |

Tổng: **24 bảng** (giữ nguyên số bảng so với schema gốc, các bảng composite-PK đã đổi sang surrogate id).

---

## 11. Lưu ý implementation

1. **Setup workflow chuẩn mỗi service:**
   ```bash
   grpc-gen add-service <name> <port>
   # Edit proto/<name>/<name>.proto theo entity của service đó
   ./generate-certs.sh <name>
   make proto-common
   make proto-<name>
   make gen-<name>
   go build ./src/service/<name>
   ```

2. **MySQL setup**: tạo 8 database riêng (vd `lms_user`, `lms_class`, ...) hoặc share 1 database `lms` cho cả 8 services. Cấu hình qua env file của mỗi service.

3. **Composite key tables (lesson_attendance, material_classes, material_tags)**: business invariant qua `UNIQUE KEY`. Insert duplicate sẽ trả `AlreadyExists` (tool đã handle qua "Duplicate entry" check).

4. **Transaction-required flows** (chưa làm với tool gen):
   - Join class bằng invite key (atomic update key + insert enrollment)
   - Submit exam attempt (validate + compute score + update status)
   - Auth login flow (verify password + create refresh_token + audit log)

   Các flow này hand-write handler sau khi CRUD cơ bản đã chạy.

5. **Cross-service calls**: Notification service cần biết user (lấy email) → gọi UserService.ListUser qua gRPC. Tool chưa gen client stubs; tự setup `grpc.Dial` ở mỗi handler nếu cần.
