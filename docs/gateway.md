# GraphQL Gateway + Business Engine

`grpc-gen add-gateway` sinh trọn vẹn gateway layer phía trên các gRPC service đã có
sẵn (qua `add-service`). Output gồm 3 phần:

1. **GraphQL schema + resolvers** — type-safe, gqlgen-backed
2. **Business rule engine** — Mongo-backed, hot-reload, không cần redeploy khi sửa rule
3. **RLS (read scope) + Action engine** — declarative authz + computed workflows

Mọi thứ trong tầng 2-3 là **DATA edit**, không phải code edit.

---

## 1. Setup

```bash
# Trong project root đã chạy add-service trước đó
grpc-gen add-gateway
```

Yêu cầu:
- `proto/<service>/<service>.proto` đã có (sinh qua add-service)
- `gateway-config.yaml` ở root project (sample tự sinh nếu chưa có)

Output:
```
src/server/
├── main.go                    # wire everything — gRPC clients, gateway, REST, MinIO...
├── graph/
│   ├── schema/*.graphqls      # GraphQL schema (one file per service)
│   ├── generated/             # gqlgen output — DO NOT EDIT
│   ├── model/                 # GraphQL models
│   ├── resolver/*.resolvers.go  # mutations + queries + relations
│   ├── client/                # gRPC client wrappers + Redis cache
│   ├── dataloader/            # N+1 protection
│   ├── auth/                  # Viewer + JWT decode
│   ├── policy/                # RoleEnforcer + MongoEnforcer (op_auth + field_auth)
│   ├── directive/             # @auth GraphQL directive
│   ├── convert/               # PB ↔ Model + enum maps
│   └── domain/                # ⭐ Business engine — Mongo-backed rules + actions + scopes
└── ...
```

## 2. `gateway-config.yaml`

Khai báo từng entity với `op_auth` (op-level role), `field_auth` (field-level role),
`relations` (nested resolvers).

```yaml
entities:
  Student:
    service: user
    op_auth:
      create: ADMIN
      update: ADMIN
      delete: ADMIN
      list:   "*"        # bất kỳ ai authenticated
      get:    "*"
    field_auth:
      email: TEACHER     # field-level: chỉ TEACHER+ đọc được
      phone: TEACHER
    relations:
      enrollments:
        target: Enrollment
        kind: hasMany
        foreign_key: student_id
      major:
        target: Major
        kind: belongsTo
        local_key: major_id

  Enrollment:
    service: thesis
    op_auth: { create: TEACHER, update: TEACHER, delete: ADMIN, list: "*", get: "*" }
    relations:
      student: { target: Student,  kind: belongsTo, local_key: student_id }
      topic:   { target: Topic,    kind: belongsTo, local_key: topic_id }
      midterm: { target: Midterm,  kind: hasOne,    foreign_key: enrollment_id }
      final:   { target: Final,    kind: hasOne,    foreign_key: enrollment_id }
```

Kind hỗ trợ: `belongsTo`, `hasOne`, `hasMany`.

---

## 3. Business rule engine — `domain/`

Engine ở `src/server/graph/domain/`. **DATA-driven**, no Go code per rule.

### Cấu trúc

| File | Vai trò |
|---|---|
| `store.go` | Mongo loader (RuleStore) — pulls business rules at gateway startup + on reload |
| `engine.go` | Rule evaluator (fetch + expr-lang condition + viewer + scope chain resolver) |
| `interceptor.go` | gRPC client interceptor — chạy rules trên mutations + scope filter trên List |
| `action.go` | Action engine (declarative fetch/compute/write pipeline) |
| `triggers.go` | Post-mutation interceptor — fan-out triggered actions |
| `system_context.go` | `WithSystemContext` / `IsSystemContext` — bypass RLS cho engine internal fetches |
| `read_scope.go` | (hand-written nếu cần RLS) — ReadScopeStore + ChainStep |
| `registry.go` | Per-entity ListFunc registry — generated từ protos |

### Cách rule hoạt động

1. FE gọi `mutation createTopic(...)`.
2. Resolver → gRPC client → interceptor chain.
3. **`UnaryInterceptor`** detect mutation → call `Engine.Check(ctx, "CreateTopic", reqMap, reqFilters)`.
4. Engine loop qua rules `ForOperation("CreateTopic")`:
   - Mỗi rule có optional `fetch` steps + `condition` expr-lang.
   - Fetch: List entity với WHERE refs (`$req.<field>`, `$viewer.id`, `$<prev>.0.<field>`).
   - Condition: expr-lang bool, env có `req`, `now`, `viewer`, + fetch results.
   - Pass → next rule. Fail → return `RuleError`.
5. Nếu mọi rule pass → invoker chạy thực sự (DB write).
6. **`TriggerInterceptor`** detect mutation thành công → fan-out actions có `Triggers: ["CreateTopic"]`.

### Schema rule (Mongo)

```js
{
  rule:      "topic_capacity",         // unique key
  operation: "CreateEnrollment",       // triggers this rule
  enabled:   true,
  priority:  20,                       // higher runs first
  fetch: [
    { as: "t", entity: "Topic",
      where: [{ field: "id", op: "EQUAL", value: "$req.topic_id" }] },
    { as: "enrs", entity: "Enrollment",
      where: [{ field: "topic_id", op: "EQUAL", value: "$req.topic_id" }] }
  ],
  condition: 'len(t) > 0 && (t[0].max_students == nil || t[0].max_students == 0 || len(enrs) < t[0].max_students)',
  message:   "Đề tài đã hết suất đăng ký"
}
```

Edit doc trong Mongo → POST `/api/v1/admin/engine/reload` (auto generated admin endpoint) →
rule mới active ngay.

### expr-lang reference quick

- Refs: `$req.<field>`, `$viewer.{id,role,email}`, `$now` (RFC3339 string), `$<fetchAs>.<idx>.<field>`
- Operators: `==`, `!=`, `<`, `<=`, `>`, `>=`, `&&`, `||`, `!`, `in`, ternary `? :`
- Functions: `len()`, `sum()`, `map()`, `filter()`, `string()`, `minutesSince(rfc3339)`,
  `verifyPassword(hash, plain)`
- String matching: `s matches "regex"` (Go regexp — **escape `.` dùng `[.]` không phải `\.`**)

### Fail policy

- Rule condition false → `FailedPrecondition` (business-invalid)
- Rule infra error (Mongo down, bad expr) → **fail OPEN** + log loud (dead engine ≠ block writes)
- Authz enforcer ngược lại: fail CLOSED

---

## 4. Read-scope (RLS)

Per-(entity, viewer.role) filter auto AND-prepend vào `List<Entity>.Search.Filters`.

### Direct (0-hop)

```js
{
  entity: "Student", viewer_role: "STUDENT",
  filters: [{ field: "id", op: "EQUAL", value: "$viewer.id" }],
  chain: [], enabled: true,
  description: "SV chỉ thấy profile chính mình"
}
```

### Chain (multi-hop)

Entity → intermediate(s) → viewer.

```js
{
  entity: "Midterm", viewer_role: "TEACHER",
  chain: [
    // Step 1: lấy Topic.id của viewer
    { via_entity: "Topic",
      via_where: [{ field: "supervisor_teacher_id", op: "EQUAL", value: "$viewer.id" }],
      via_select: "id" },
    // Step 2: lấy Enrollment.id từ those topic IDs
    { via_entity: "Enrollment",
      via_where: [{ field: "topic_id", op: "IN", value: "$prev_ids" }],
      via_select: "id",
      local_field: "enrollment_id" }   // last step → emit filter `enrollment_id IN <ids>`
  ],
  enabled: true,
  description: "GV chỉ thấy Midterm của Enrollment thuộc Topic mình supervise (2-hop)"
}
```

**Magic refs:**
- `$viewer.id` — ID của viewer hiện tại
- `$prev_ids` — comma-joined IDs từ step trước (engine auto split → multi-value IN)

**Self-join supported:** `via_entity` có thể chính là entity scoped (engine wrap `WithSystemContext` → bypass scope recursion).

**Empty chain result:** engine inject sentinel `{local_field EQUAL "0"}` → 0 row (fail closed).

**ADMIN bypass:** viewer role `ADMIN` skip scope hoàn toàn.

### Wiring

Trong `main.go`:

```go
opts := []domain.EngineOption{
    domain.WithViewerExtractor(viewerForRule),
    domain.WithReadScopeStore(scopeStore),  // ← auto-included từ scaffold v0.2+
}
ruleEngine = domain.NewEngine(ruleStore, registry, opts...)
```

---

## 5. Action engine

Declarative pipeline: `fetch → compute → write`. Lưu trong Mongo collection `action`.

```js
{
  name: "ComputeFinalGrade",
  steps: [
    { kind: "fetch", as: "enr", entity: "Enrollment",
      where: [{ field: "id", op: "EQUAL", value: "$input.enrollment_id" }] },
    { kind: "fetch", as: "topic", entity: "Topic",
      where: [{ field: "id", op: "EQUAL", value: "$enr.0.topic_id" }] },
    { kind: "fetch", as: "midterm", entity: "Midterm",
      where: [{ field: "enrollment_id", op: "EQUAL", value: "$input.enrollment_id" }] },
    { kind: "fetch", as: "grades", entity: "GradeDefence",
      where: [{ field: "enrollment_id", op: "EQUAL", value: "$input.enrollment_id" }] },
    { kind: "compute", as: "council_avg",
      expr: "len(grades) == 0 ? 0.0 : sum(map(grades, .total_score)) / len(grades)" },
    { kind: "compute", as: "final",
      expr: "(midterm[0].grade * topic[0].percent_stage1 + council_avg * topic[0].percent_stage2) / 100.0" },
    { kind: "compute", as: "status",
      expr: 'final >= 5.0 ? "FINAL_PASSED" : "FINAL_FAILED"' },
    { kind: "write", op: "CreateFinal",
      set: {
        enrollment_id: "$input.enrollment_id",
        final_grade: "$final",
        status: "$status",
        created_by: "action:ComputeFinalGrade"
      }
    }
  ],
  output: "{ok: true, final_grade: final, status: status}"
}
```

Call: `mutation { runAction(name: "ComputeFinalGrade", input: { enrollment_id: "200" }) }`.

### Triggers (auto-run)

Field `triggers` chứa op names — action tự chạy sau mỗi op thành công:

```js
{
  name: "RecomputeGradeTotal",
  triggers: ["CreateGradeDefenceCriterion"],
  steps: [
    { kind: "fetch", as: "siblings", entity: "GradeDefenceCriterion",
      where: [{ field: "grade_defence_id", op: "EQUAL", value: "$input.grade_defence_id" }] },
    // ... compute weighted total ...
    { kind: "write", op: "UpdateGradeDefence",
      where: [{ field: "id", op: "EQUAL", value: "$input.grade_defence_id" }],
      set: { total_score: "$total", updated_by: "action:RecomputeGradeTotal" }
    }
  ]
}
```

→ FE chỉ cần `createGradeDefenceCriterion`. Backend tự recompute. Không cần FE coordination.

**Failure policy của trigger:** log loud + skip — mutation parent đã commit, không nên rollback.

---

## 6. System-context bypass

Khi engine làm internal fetch (rule's `fetch`, action's `fetch`, scope chain `via_entity`),
ctx được wrap với `WithSystemContext`:

```go
sysCtx := WithSystemContext(ctx)
rows, err := lf(sysCtx, search)
```

Trong interceptor (scope inject):

```go
func (e *Engine) ScopeFiltersFor(ctx context.Context, entity string) []*commonpb.FilterCriteria {
    if IsSystemContext(ctx) {
        return nil  // ← bypass RLS cho engine internal calls
    }
    // ...
}
```

**Vì sao cần:**
1. **Tránh recursion**: scope chain step gọi `List(ViaEntity)` không nên bị scope của chính ViaEntity filter.
2. **Tránh false-deny**: rule fetch "topic_exists" không nên bị scope của viewer ẩn target row.

---

## 7. Workflow thực tế (sau khi scaffold đã sẵn)

```bash
# A. Add entity mới (cold edit — cần regen + rebuild)
vim proto/<svc>/<svc>.proto       # thêm message + RPCs
make proto-<svc>                  # protoc
make gen-<svc>                    # scaffold handler.go + filterable
../grpc-gen add-gateway           # GraphQL schema + resolvers + scope/Triggers tự include
go build ./... && bash scripts/run-*.sh

# B. Sửa business logic (hot edit — chỉ Mongo)
mongosh ".../my_policy" --eval 'db.business_rule.updateOne({rule:"x"}, {$set:{condition:"..."}})'
curl -X POST localhost:8081/api/v1/admin/engine/reload -H "Authorization: Bearer $ADMIN"
# → rule mới active ngay, không restart
```

---

## 8. Khái niệm "cold" vs "hot" edit

| Edit type | Tier | Cần gì |
|---|---|---|
| Add new field on existing entity | Cold | proto + regen + DB migrate + rebuild |
| Add new entity | Cold | proto + regen + DB migrate + add to gateway-config + rebuild |
| Change op_auth role | Hot | Mongo `policy.updateOne(...)` + reload |
| Change field_auth role | Hot | Mongo `policy.updateOne(...)` + reload |
| Add/edit business rule | Hot | Mongo `business_rule.updateOne(...)` + reload |
| Add/edit action | Hot | Mongo `action.updateOne(...)` + reload |
| Add/edit read scope | Hot | Mongo `read_scope.updateOne(...)` + reload |
| Add/edit action trigger | Hot | Mongo `action.updateOne(...)` + reload |

→ **Hot edit là 90% công việc maintain hệ thống đã ship.** Cold edit chỉ khi thực sự đổi schema.

---

## 9. Admin REST API (auto-generated)

Khi `LVTN_MODE=dev` (hoặc tương đương trong project khác):

| Endpoint | Function |
|---|---|
| `POST /api/v1/admin/engine/reload` | Reload rules + actions + policies + scopes |
| `POST /api/v1/admin/rules` / GET / PUT / DELETE | CRUD business_rule |
| `POST /api/v1/admin/actions` / GET / PUT / DELETE | CRUD action |
| `POST /api/v1/admin/policies` / GET / PUT / DELETE | CRUD op_auth + field_auth |
| `POST /api/v1/admin/read-scopes` / GET / PUT / DELETE | CRUD read_scope |
| `POST /api/v1/admin/rules/trace` | Why was this denied? (rule eval w/ per-rule diagnostics) |
| `GET  /api/v1/admin/audit_log` | Recent rule/action evaluations |
| `GET  /api/v1/admin/entity-schema` | Introspection: list entities + fields + relations |

→ FE dashboard có thể CRUD business logic mà không cần backend dev.

---

## 10. Reference

- `domain/engine.go` — `Engine.Check`, `Engine.ScopeFiltersFor`, `Engine.resolveChain`
- `domain/action.go` — `ActionEngine.Run`, `ActionStore.ActionsForTrigger`
- `domain/interceptor.go` — `UnaryInterceptor` (rule + scope inject)
- `domain/triggers.go` — `TriggerInterceptor` (post-mutation actions)
- `domain/system_context.go` — `WithSystemContext` / `IsSystemContext`
- [expr-lang docs](https://expr-lang.org/) — condition + compute syntax
- [gqlgen docs](https://gqlgen.com/) — GraphQL framework underneath

---

## 11. FAQ

**Q: Có thể seed business rules từ file không?**
A: Có. Convention: `scripts/seed-rules*.js` (mongo shell scripts). Sample patterns trong các project sample.

**Q: Rule fire nhiều lần — có cache không?**
A: Không. Mỗi mutation chạy fresh evaluation (data hiện tại). RuleStore cache rules in-memory, reload via admin endpoint.

**Q: Action trigger có infinite-loop risk không?**
A: Có nếu action's write triggers thêm action với cùng op. Best practice: action target khác op so với trigger.

**Q: RLS chỉ work trên List?**
A: Đúng — auto inject vào `Search.Filters` của List ops. Get-by-id không bị scope (caller phải biết ID); thường wrap Get qua nested resolver được scope qua parent.

**Q: ADMIN bypass RLS hoàn toàn?**
A: Đúng — `viewer.role == "ADMIN"` → `ScopeFiltersFor` return nil. ADMIN cũng bypass field_auth (trong `MongoEnforcer.HasRole`).
