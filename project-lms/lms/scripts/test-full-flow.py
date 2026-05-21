#!/usr/bin/env python3
"""Full-flow test for the LMS GraphQL gateway.

Covers, with PASS/FAIL assertions:
  A. Field-level authz (@policy on User.email, @auth on User.password_hash)
     across every viewer level: anon -> student -> teacher -> admin.
  B. Relation-path authz — the directive must also redact User.email when a
     User is reached through a relation (Class.teacher), not just top-level.
  C. Per-viewer isolation — the same query returns different data per viewer
     with no cross-viewer leak (cache key includes the viewer).
  D. Business rules — every guarded mutation, allow + deny paths.

Mutations that create rows tag created_by='fulltest'; the script deletes
them at the end so it is repeatable. Requires the LMS stack running
(./scripts/run-lms.sh) and mysql_container reachable via docker.
"""
import base64, hashlib, hmac, json, subprocess, sys, urllib.request

GATEWAY = "http://localhost:8080/query"
SECRET = b"lms-dev-secret"  # must match JWT_HMAC_SECRET in run-lms.sh


def mint(sub, role):
    b64 = lambda d: base64.urlsafe_b64encode(d).rstrip(b"=")
    h = b64(json.dumps({"alg": "HS256", "typ": "JWT"}, separators=(",", ":")).encode())
    p = b64(json.dumps({"sub": sub, "role": role}, separators=(",", ":")).encode())
    sig = b64(hmac.new(SECRET, h + b"." + p, hashlib.sha256).digest())
    return (h + b"." + p + b"." + sig).decode()


TOKENS = {
    "student": mint("3", "STUDENT"),   # user 3 — a STUDENT, enrolled in class 3
    "teacher": mint("2", "TEACHER"),   # user 2 — a TEACHER
    "admin":   mint("999", "ADMIN"),
}


def gql(query, viewer=None):
    """Run a GraphQL op. Returns (data, errors). viewer in {None,student,teacher,admin}."""
    body = json.dumps({"query": query}).encode()
    headers = {"Content-Type": "application/json"}
    if viewer:
        headers["Authorization"] = "Bearer " + TOKENS[viewer]
    req = urllib.request.Request(GATEWAY, body, headers)
    try:
        resp = json.loads(urllib.request.urlopen(req).read())
    except Exception as e:
        return None, [{"message": f"HTTP {e}"}]
    return resp.get("data"), resp.get("errors")



# mut runs a business-rule mutation as ADMIN — admin clears every
# @auth gate (role hierarchy), so the BUSINESS RULE is what gets tested,
# not the operation-level role gate (that is section F).
def mut(query):
    return gql(query, "admin")

def mysql(sql):
    subprocess.run(
        ["docker", "exec", "mysql_container", "mysql", "-uthaily", "-pTh@i2004", "-e", sql],
        capture_output=True,
    )


PASS = FAIL = 0


def check(name, ok, detail=""):
    global PASS, FAIL
    mark = "\033[32mPASS\033[0m" if ok else "\033[31mFAIL\033[0m"
    if ok:
        PASS += 1
    else:
        FAIL += 1
    print(f"  {mark}  {name}" + (f"  — {detail}" if detail and not ok else ""))


def section(t):
    print(f"\n\033[1m{t}\033[0m")


# ── A. Field-level authz ────────────────────────────────────────────────
section("A. Field authz — @policy(email) + @auth(password_hash)")

def users(viewer):
    data, _ = gql("{ listUser { items { id email password_hash } } }", viewer)
    return {u["id"]: u for u in data["listUser"]["items"]}

# listUser is op-gated @auth(role:"*") — anon can't call it at all.
anon_data, anon_err = gql("{ listUser { items { id } } }", None)
check("anon: KHÔNG gọi được listUser (chưa đăng nhập)",
      anon_data is None and bool(anon_err), str(anon_err))

stu = users("student")
check("student: CHỈ thấy email của chính mình (id=3)",
      stu["3"]["email"] is not None and all(
          u["email"] is None for i, u in stu.items() if i != "3"),
      f"thấy: {[i for i,u in stu.items() if u['email']]}")
check("student: KHÔNG thấy password_hash của ai",
      all(u["password_hash"] is None for u in stu.values()),
      f"leaked: {[i for i,u in stu.items() if u['password_hash']]}")

tea = users("teacher")
check("teacher: thấy mọi email (role TEACHER trong policy)",
      all(u["email"] is not None for u in tea.values()))
check("teacher: KHÔNG thấy password_hash (chỉ ADMIN)",
      all(u["password_hash"] is None for u in tea.values()),
      f"leaked: {[i for i,u in tea.items() if u['password_hash']]}")

adm = users("admin")
check("admin: thấy mọi email", all(u["email"] is not None for u in adm.values()))
check("admin: thấy mọi password_hash",
      all(u["password_hash"] is not None for u in adm.values()))

# ── B. Relation-path authz ──────────────────────────────────────────────
section("B. Authz qua relation — Class.teacher.email")

def class_teacher_email(viewer):
    data, _ = gql("{ listClass { items { teacher { id email } } } }", viewer)
    out = {}
    for c in data["listClass"]["items"]:
        t = c.get("teacher")
        if t:
            out[t["id"]] = t["email"]
    return out

bt_student = class_teacher_email("student")
# teacher of class 3 is user 2 — student 3 must NOT see user 2's email
check("student: email của teacher (qua relation) bị ẩn",
      all(v is None for v in bt_student.values()),
      f"leaked: {bt_student}")
bt_admin = class_teacher_email("admin")
check("admin: email của teacher (qua relation) hiện",
      all(v is not None for v in bt_admin.values()) and len(bt_admin) > 0)

# ── C. Per-viewer isolation (cross-viewer leak) ─────────────────────────
section("C. Cô lập theo viewer — không rò cache giữa các viewer")

# student query first (caches the student-scoped fetch), then teacher/admin —
# they must still get full emails, not the student's redacted view.
_ = users("student")
tea2 = users("teacher")
check("teacher sau student: vẫn thấy đủ email (cache không rò)",
      all(u["email"] is not None for u in tea2.values()))
adm2 = users("admin")
check("admin sau student: vẫn thấy password_hash (cache không rò)",
      all(u["password_hash"] is not None for u in adm2.values()))
# and the student view is still correctly restricted afterwards
stu2 = users("student")
check("student vẫn bị giới hạn sau khi teacher/admin query",
      all(u["email"] is None for i, u in stu2.items() if i != "3"))

# ── D. Business rules ───────────────────────────────────────────────────
section("D. Business rules — allow + deny")

def denied(errors, fragment):
    return errors and any(fragment.lower() in (e.get("message", "").lower()) for e in errors)

# Pre-clean: D1 assumes student 4 is NOT enrolled in class 3 — drop any
# leftover (e.g. an enrollment created from the UI) so the suite is
# deterministic regardless of prior DB state.
mysql("DELETE FROM lms.enrollment WHERE class_id=3 AND student_id=4;")

# D1 CreateEnrollment ALLOW — student 4, chưa enroll
d, e = mut('mutation { createEnrollment(input: {class_id:"3", student_id:"4",'
           ' joined_at:"2026-05-21T10:00:00Z", status:ENROLL_ACTIVE,'
           ' created_by:"fulltest"}) { id } }')
check("CreateEnrollment ALLOW (student 4 -> class 3)", d and not e,
      str(e))

# D2 CreateEnrollment DENY — trùng
_, e = mut('mutation { createEnrollment(input: {class_id:"3", student_id:"4",'
           ' joined_at:"2026-05-21T10:00:00Z", status:ENROLL_ACTIVE,'
           ' created_by:"fulltest"}) { id } }')
check("CreateEnrollment DENY trùng", denied(e, "da enroll"), str(e))

# D3 CreateEnrollment DENY — không phải student (user 2 là teacher)
_, e = mut('mutation { createEnrollment(input: {class_id:"3", student_id:"2",'
           ' joined_at:"2026-05-21T10:00:00Z", status:ENROLL_ACTIVE,'
           ' created_by:"fulltest"}) { id } }')
check("CreateEnrollment DENY không phải student", denied(e, "student"), str(e))

# D4 CreateEnrollment DENY — class không tồn tại
_, e = mut('mutation { createEnrollment(input: {class_id:"999", student_id:"4",'
           ' joined_at:"2026-05-21T10:00:00Z", status:ENROLL_ACTIVE,'
           ' created_by:"fulltest"}) { id } }')
check("CreateEnrollment DENY class không tồn tại", denied(e, "khong ton tai"), str(e))

# D5 CreateExamAttempt ALLOW — student 3 enrolled, exam 1, còn lượt (prior=1<2)
d, e = mut('mutation { createExamAttempt(input: {exam_id:"1", student_id:"3",'
           ' started_at:"2026-05-21T10:00:00Z", status:IN_PROGRESS,'
           ' created_by:"fulltest"}) { id } }')
check("CreateExamAttempt ALLOW (student 3 enrolled, còn lượt)", d and not e, str(e))

# D6 CreateExamAttempt DENY — student 99 chưa enrolled (student 4 đã bị D1
# enroll, nên dùng một student chắc chắn không thuộc lớp).
_, e = mut('mutation { createExamAttempt(input: {exam_id:"1", student_id:"99",'
           ' started_at:"2026-05-21T10:00:00Z", status:IN_PROGRESS,'
           ' created_by:"fulltest"}) { id } }')
check("CreateExamAttempt DENY chưa enrolled", denied(e, "chua enrolled"), str(e))

# D7 CreateExamAttempt DENY — hết lượt (sau D5: prior=2, max=2)
_, e = mut('mutation { createExamAttempt(input: {exam_id:"1", student_id:"3",'
           ' started_at:"2026-05-21T10:00:00Z", status:IN_PROGRESS,'
           ' created_by:"fulltest"}) { id } }')
check("CreateExamAttempt DENY hết lượt làm bài", denied(e, "het so lan"), str(e))

# D8 CreateExamAssignment DENY — open > close
_, e = mut('mutation { createExamAssignment(input: {exam_id:"1", class_id:"3",'
           ' open_at:"2026-12-01T00:00:00Z", close_at:"2026-01-01T00:00:00Z",'
           ' max_attempts:2, created_by:"fulltest"}) { id } }')
check("CreateExamAssignment DENY open>close", denied(e, "open_at"), str(e))

# D9 CreateExamAssignment DENY — đã giao cho lớp
_, e = mut('mutation { createExamAssignment(input: {exam_id:"1", class_id:"3",'
           ' open_at:"2026-01-01T00:00:00Z", close_at:"2026-12-01T00:00:00Z",'
           ' max_attempts:2, created_by:"fulltest"}) { id } }')
check("CreateExamAssignment DENY đã giao cho lớp", denied(e, "da duoc giao"), str(e))

# D10 CreateLesson DENY — start > end
_, e = mut('mutation { createLesson(input: {class_id:"3", title:"T",'
           ' start_at:"2026-06-01T10:00:00Z", end_at:"2026-06-01T08:00:00Z",'
           ' mode:OFFLINE, status:SCHEDULED, created_by:"fulltest"}) { id } }')
check("CreateLesson DENY start>end", denied(e, "start_at"), str(e))

# D11 CreateLessonAttendance DENY — student không thuộc lớp của buổi học
# lesson 1 thuộc class của nó; student 4 chưa enroll -> DENY
_, e = mut('mutation { createLessonAttendance(input: {lesson_id:"1", student_id:"4",'
           ' status:PRESENT, marked_by_teacher_id:"2",'
           ' marked_at:"2026-05-21T10:00:00Z", created_by:"fulltest"}) { id } }')
check("CreateLessonAttendance DENY student ngoài lớp", denied(e, "khong thuoc"), str(e))

# ── E. Full ruleset — referential / role / state-machine / delete-dep ───
section("E. Ruleset đầy đủ — referential / role / state-machine / delete-dep")

# E1 referential — CreateQuestion exam không tồn tại
_, e = mut('mutation { createQuestion(input: {exam_id:"999", position:1, type:SINGLE,'
           ' content:"x", points:1.0, created_by:"fulltest"}) { id } }')
check("CreateQuestion DENY exam không tồn tại", denied(e, "exam khong ton tai"), str(e))

# E2 role — CreateClass teacher_id trỏ vào student (user 3)
_, e = mut('mutation { createClass(input: {teacher_id:"3", name:"X", status:ACTIVE, visibility:PUBLIC,'
           ' created_by:"fulltest"}) { id } }')
check("CreateClass DENY teacher_id không phải TEACHER", denied(e, "role teacher"), str(e))

# E3 state-machine — UpdateClass ACTIVE -> DRAFT (lùi)
_, e = mut('mutation { updateClass(filters:[{condition:{field:"id",operator:EQUAL,values:["3"]}}],'
           ' input:{status:DRAFT, updated_by:"fulltest"}) { affected_count } }')
check("UpdateClass DENY status lùi ACTIVE->DRAFT", denied(e, "1 chieu"), str(e))

# E4 state-machine — UpdateExamAttempt GRADED -> IN_PROGRESS (lùi)
_, e = mut('mutation { updateExamAttempt(filters:[{condition:{field:"id",operator:EQUAL,values:["1"]}}],'
           ' input:{status:IN_PROGRESS, updated_by:"fulltest"}) { affected_count } }')
check("UpdateExamAttempt DENY status lùi GRADED->IN_PROGRESS", denied(e, "1 chieu"), str(e))

# E5 delete-dependency — DeleteClass id=3 còn enrollment
_, e = mut('mutation { deleteClass(filters:[{condition:{field:"id",operator:EQUAL,values:["3"]}}])'
           ' { affected_count } }')
check("DeleteClass DENY còn enrollment", denied(e, "con enrollment"), str(e))

# E6 referential PASS — CreateQuestion exam 1 tồn tại
d, e = mut('mutation { createQuestion(input: {exam_id:"1", position:99, type:SINGLE,'
           ' content:"full-flow test q", points:1.0, created_by:"fulltest"}) { id } }')
check("CreateQuestion ALLOW exam tồn tại", d and not e, str(e))


# ── F. Operation-level role gating (@auth on mutations) ─────────────────
section("F. Role gating trên operation — @auth(role)")

def autherr(e):
    return bool(e) and any(
        ("unauthenticated" in x.get("message", "").lower()
         or "forbidden" in x.get("message", "").lower()) for x in e)

# createClass — @auth(role: TEACHER)
for who, exp_block in [(None, True), ("student", True), ("teacher", False), ("admin", False)]:
    _, e = gql('mutation { createClass(input: {teacher_id:"2", name:"FF", status:ACTIVE, visibility:PUBLIC,'
               ' created_by:"fulltest"}) { id } }', who)
    label = who or "anon"
    check(f"createClass [{label}] — {'CHẶN' if exp_block else 'QUA'} @auth(TEACHER)",
          autherr(e) == exp_block, str(e))

# deleteUser — @auth(role: ADMIN)
for who, exp_block in [(None, True), ("teacher", True), ("admin", False)]:
    _, e = gql('mutation { deleteUser(filters:[{condition:{field:"id",operator:EQUAL,'
               'values:["99999"]}}]) { affected_count } }', who)
    label = who or "anon"
    check(f"deleteUser [{label}] — {'CHẶN' if exp_block else 'QUA'} @auth(ADMIN)",
          autherr(e) == exp_block, str(e))

# createEnrollment — @auth(role: STUDENT): teacher KHÔNG kế thừa student
for who, exp_block in [("teacher", True), ("student", False), ("admin", False)]:
    _, e = gql('mutation { createEnrollment(input: {class_id:"3", student_id:"4",'
               ' joined_at:"2026-05-21T10:00:00Z", status:ENROLL_ACTIVE,'
               ' created_by:"fulltest"}) { id } }', who)
    check(f"createEnrollment [{who}] — {'CHẶN' if exp_block else 'QUA'} @auth(STUDENT)",
          autherr(e) == exp_block, str(e))

# runAction — @auth(role: "*") = bất kỳ ai đã đăng nhập
for who, exp_block in [(None, True), ("student", False)]:
    _, e = gql('mutation { runAction(name:"__nope__", input:{}) }', who)
    label = who or "anon"
    check(f"runAction [{label}] — {'CHẶN' if exp_block else 'QUA'} @auth(*)",
          autherr(e) == exp_block, str(e))

# ── cleanup ─────────────────────────────────────────────────────────────
section("Cleanup — xoá data do test tạo (created_by='fulltest')")
for tbl in ("enrollment", "examattempt", "examassignment", "lesson",
            "lessonattendance", "question", "class"):
    mysql(f"DELETE FROM lms.{tbl} WHERE created_by='fulltest';")
print("  done")

# ── summary ─────────────────────────────────────────────────────────────
print(f"\n\033[1mTOTAL: {PASS} pass, {FAIL} fail\033[0m")
sys.exit(0 if FAIL == 0 else 1)
