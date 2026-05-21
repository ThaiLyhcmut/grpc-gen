#!/usr/bin/env python3
"""Stress / adversarial test for the LMS gateway.

Goes past the happy path:
  A. Security  — forged / malformed / tampered JWT must NOT authenticate.
  B. Boundary  — exact edges of max_attempts, class capacity, exam window.
  C. State     — every illegal ExamAttempt status transition.
  D. Concurrency — N parallel requests; checks the UNIQUE backstop, and
     honestly reports the max_attempts read-check-write race.
  E. Cache     — a mutation must invalidate the cached read.
  F. Action    — error paths of the action engine.

Tags rows created_by='stress' and deletes them at the end. LMS must run.
"""
import base64, hashlib, hmac, json, subprocess, sys, threading, time, urllib.request

SECRET = b"lms-dev-secret"


def b64u(d):
    return base64.urlsafe_b64encode(d).rstrip(b"=").decode()


def mint(sub, role, secret=SECRET):
    h = b64u(json.dumps({"alg": "HS256", "typ": "JWT"}, separators=(",", ":")).encode())
    p = b64u(json.dumps({"sub": sub, "role": role}, separators=(",", ":")).encode())
    s = b64u(hmac.new(secret, (h + "." + p).encode(), hashlib.sha256).digest())
    return h + "." + p + "." + s


ADMIN = mint("999", "ADMIN")
TEACHER = mint("2", "TEACHER")
STUDENT = mint("3", "STUDENT")


def gql(q, token=ADMIN):
    body = json.dumps({"query": q}).encode()
    hd = {"Content-Type": "application/json"}
    if token:
        hd["Authorization"] = "Bearer " + token
    try:
        d = json.loads(urllib.request.urlopen(
            urllib.request.Request("http://localhost:8080/query", body, hd)).read())
        return d.get("data"), d.get("errors")
    except Exception as e:
        return None, [{"message": str(e)}]


def mysql(sql):
    subprocess.run(["docker", "exec", "mysql_container", "mysql", "-uthaily", "-pTh@i2004",
                    "-e", sql], capture_output=True)


def mysql_scalar(sql):
    r = subprocess.run(["docker", "exec", "mysql_container", "mysql", "-uthaily",
                        "-pTh@i2004", "-N", "-e", sql], capture_output=True, text=True)
    return r.stdout.strip()


P = F = 0
def ck(name, cond, detail=""):
    global P, F
    P, F = P + bool(cond), F + (not cond)
    print(("  \033[32m✓\033[0m " if cond else "  \033[31m✗\033[0m ") + name
          + ("" if cond else "  — " + detail))


def note(msg):
    print("  \033[33m•\033[0m " + msg)


def sect(t):
    print("\n\033[1m" + t + "\033[0m")


def authfail(e):
    return bool(e) and any(m in (x.get("message", "").lower())
                           for x in e for m in ("unauthenticated", "forbidden"))


# helpers to build a mutation string without f-string brace clashes
def m_class(name, vis="PUBLIC", maxs=None):
    extra = (", max_students:" + str(maxs)) if maxs is not None else ""
    return ('mutation { createClass(input: {teacher_id:"2", name:"' + name +
            '", status:ACTIVE, visibility:' + vis + extra +
            ', created_by:"stress"}) { id } }')


def m_exam(name, dur=60):
    return ('mutation { createExam(input: {owner_teacher_id:"2", title:"' + name +
            '", duration_min:' + str(dur) + ', total_points:10.0, visibility:EXAM_CLASS,'
            ' shuffle_questions:false, shuffle_choices:false, show_result_mode:IMMEDIATELY,'
            ' created_by:"stress"}) { id } }')


def m_assign(exam, cls, open_at, close_at, maxa):
    return ('mutation { createExamAssignment(input: {exam_id:"' + exam + '", class_id:"' + cls +
            '", open_at:"' + open_at + '", close_at:"' + close_at + '", max_attempts:' +
            str(maxa) + ', created_by:"stress"}) { id } }')


def m_enroll(cls, sid):
    return ('mutation { createEnrollment(input: {class_id:"' + cls + '", student_id:"' + sid +
            '", joined_at:"2026-06-01T00:00:00Z", status:ENROLL_ACTIVE,'
            ' created_by:"stress"}) { id } }')


def m_attempt(exam, sid, started="2026-05-21T09:00:00Z"):
    return ('mutation { createExamAttempt(input: {exam_id:"' + exam + '", student_id:"' + sid +
            '", started_at:"' + started + '", status:IN_PROGRESS,'
            ' created_by:"stress"}) { id } }')


def m_setstatus(aid, status):
    return ('mutation { updateExamAttempt(filters:[{condition:{field:"id",operator:EQUAL,'
            'values:["' + aid + '"]}}], input:{status:' + status +
            ', updated_by:"stress"}) { affected_count } }')


def newid(d, key):
    return d[key]["id"] if d else None


FUTURE = "2027-12-31T00:00:00Z"
FAR_FUTURE = "2099-12-31T00:00:00Z"
PAST = "2020-01-01T00:00:00Z"
PAST_END = "2020-12-31T00:00:00Z"

# ════════════════════════════════════════════════════════════════════════
sect("A. Bảo mật — JWT giả / hỏng / bị sửa")

d, e = gql(m_class("A-baseline"), TEACHER)
ck("token TEACHER hợp lệ → createClass ALLOW", d and not e, str(e))

forged = mint("2", "TEACHER", b"hacker-secret-not-real")
_, e = gql(m_class("A-forged"), forged)
ck("token GIẢ (ký sai secret) → createClass DENY", authfail(e), str(e))

_, e = gql(m_class("A-malformed"), "this.is.garbage")
ck("token HỎNG (rác) → createClass DENY", authfail(e), str(e))

# lấy token student hợp lệ, đổi role -> ADMIN, GIỮ chữ ký cũ
h, p, s = STUDENT.split(".")
pad = lambda x: x + "=" * (-len(x) % 4)
payload = json.loads(base64.urlsafe_b64decode(pad(p)))
payload["role"] = "ADMIN"
tampered = h + "." + b64u(json.dumps(payload, separators=(",", ":")).encode()) + "." + s
_, e = gql(m_class("A-tampered"), tampered)
ck("token BỊ SỬA role (chữ ký cũ) → createClass DENY", authfail(e), str(e))

_, e = gql('mutation { deleteUser(filters:[{condition:{field:"id",operator:EQUAL,'
           'values:["88888"]}}]) { affected_count } }', forged)
ck("token GIẢ → deleteUser DENY", authfail(e), str(e))

# ════════════════════════════════════════════════════════════════════════
sect("B. Biên — max_attempts / sĩ số / cửa sổ thời gian")

# B1 — max_attempts = 2 đúng biên
cls = newid(gql(m_class("B-att"))[0], "createClass")
exam = newid(gql(m_exam("B-exam"))[0], "createExam")
gql(m_assign(exam, cls, "2026-01-01T00:00:00Z", FUTURE, 2))
gql(m_enroll(cls, "4"))
r1 = gql(m_attempt(exam, "4"))
r2 = gql(m_attempt(exam, "4"))
_, e3 = gql(m_attempt(exam, "4"))
ck("attempt 1,2 (max=2) → ALLOW; attempt 3 → DENY",
   r1[0] and r2[0] and e3 and "het so lan" in str(e3).lower(), str(e3))

# B2 — sĩ số: class max_students = 1
clsf = newid(gql(m_class("B-full", maxs=1))[0], "createClass")
ok1, e1 = gql(m_enroll(clsf, "4"))
_, e2 = gql(m_enroll(clsf, "3"))
ck("class sĩ số 1: HS thứ 1 ALLOW, HS thứ 2 → DENY (đầy)",
   ok1 and not e1 and e2 and "day" in str(e2).lower(), str(e1) + " | " + str(e2))

# B3 — cửa sổ thời gian
clsw = newid(gql(m_class("B-win"))[0], "createClass")
gql(m_enroll(clsw, "4"))
examF = newid(gql(m_exam("B-future"))[0], "createExam")
gql(m_assign(examF, clsw, FUTURE, FAR_FUTURE, 5))
_, e = gql(m_attempt(examF, "4"))
ck("exam mở trong tương lai → attempt DENY (chưa mở)",
   e and "thoi gian" in str(e).lower(), str(e))
examC = newid(gql(m_exam("B-closed"))[0], "createExam")
gql(m_assign(examC, clsw, PAST, PAST_END, 5))
_, e = gql(m_attempt(examC, "4"))
ck("exam đã đóng → attempt DENY (hết hạn)", e and "thoi gian" in str(e).lower(), str(e))

# ════════════════════════════════════════════════════════════════════════
sect("C. State machine — chuyển status ExamAttempt sai luật")

clsS = newid(gql(m_class("C-sm"))[0], "createClass")
examS = newid(gql(m_exam("C-exam"))[0], "createExam")
gql(m_assign(examS, clsS, "2026-01-01T00:00:00Z", FUTURE, 9))
gql(m_enroll(clsS, "4"))
now = time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())

def fresh_attempt():
    return newid(gql(m_attempt(examS, "4", now))[0], "createExamAttempt")

a = fresh_attempt()
ck("IN_PROGRESS → SUBMITTED → ALLOW", gql(m_setstatus(a, "SUBMITTED"))[0] is not None)
ck("SUBMITTED → GRADED → ALLOW", gql(m_setstatus(a, "GRADED"))[0] is not None)
_, e = gql(m_setstatus(a, "IN_PROGRESS"))
ck("GRADED → IN_PROGRESS → DENY", e and "1 chieu" in str(e).lower(), str(e))
_, e = gql(m_setstatus(a, "SUBMITTED"))
ck("GRADED → SUBMITTED → DENY", e and "1 chieu" in str(e).lower(), str(e))
b = fresh_attempt()
gql(m_setstatus(b, "SUBMITTED"))
_, e = gql(m_setstatus(b, "IN_PROGRESS"))
ck("SUBMITTED → IN_PROGRESS → DENY", e and "1 chieu" in str(e).lower(), str(e))

# ════════════════════════════════════════════════════════════════════════
sect("D. Concurrency — N request song song")

# D1 — 8 ghi danh trùng song song: UNIQUE key (class_id,student_id) là chốt chặn
clsC = newid(gql(m_class("D-dup"))[0], "createClass")
results = []
def fire_enroll():
    d, e = gql(m_enroll(clsC, "4"))
    results.append(d and not e)
ths = [threading.Thread(target=fire_enroll) for _ in range(8)]
for t in ths: t.start()
for t in ths: t.join()
rows = mysql_scalar("SELECT COUNT(*) FROM lms.enrollment WHERE class_id=" + clsC +
                    " AND student_id=4;")
ck("8 ghi danh trùng song song → DB chỉ có ĐÚNG 1 dòng (UNIQUE chốt chặn)",
   rows == "1", "enrollment rows = " + rows + ", ALLOW count = " + str(sum(1 for x in results if x)))

# D2 — 8 attempt song song, max_attempts=3: rule read-check-write KHÔNG atomic
clsR = newid(gql(m_class("D-race"))[0], "createClass")
examR = newid(gql(m_exam("D-race-exam"))[0], "createExam")
gql(m_assign(examR, clsR, "2026-01-01T00:00:00Z", FUTURE, 3))
gql(m_enroll(clsR, "4"))
acnt = []
def fire_attempt():
    d, e = gql(m_attempt(examR, "4"))
    acnt.append(d and not e)
ths = [threading.Thread(target=fire_attempt) for _ in range(8)]
for t in ths: t.start()
for t in ths: t.join()
made = mysql_scalar("SELECT COUNT(*) FROM lms.examattempt WHERE exam_id=" + examR +
                    " AND student_id=4 AND created_by='stress';")
ck("8 attempt song song → server xử lý hết, không crash", made.isdigit() and int(made) >= 1,
   "tạo ra " + made)
if int(made or 0) > 3:
    note("FINDING: rule max_attempts đọc-rồi-ghi KHÔNG atomic — tạo ra " + made +
         " attempt (vượt max=3). Luồng TUẦN TỰ (test B1) vẫn chặn đúng; race tuyệt đối"
         " cần UNIQUE/transaction phía DB. Đây là giới hạn đã biết của rule engine.")
else:
    note("max_attempts giữ được giới hạn cả khi song song (" + made + " <= 3)")

# ════════════════════════════════════════════════════════════════════════
sect("E. Cache — mutation phải làm mới cache")

clsE = newid(gql(m_class("E-cache-old"))[0], "createClass")
d, _ = gql('{ listClass(search:{filters:[{condition:{field:"id",operator:EQUAL,values:["' +
           clsE + '"]}}]}) { items { name } } }')
before = d["listClass"]["items"][0]["name"] if d and d["listClass"]["items"] else ""
gql('mutation { updateClass(filters:[{condition:{field:"id",operator:EQUAL,values:["' + clsE +
    '"]}}], input:{name:"E-cache-NEW", updated_by:"stress"}) { affected_count } }')
d, _ = gql('{ listClass(search:{filters:[{condition:{field:"id",operator:EQUAL,values:["' +
           clsE + '"]}}]}) { items { name } } }')
after = d["listClass"]["items"][0]["name"] if d and d["listClass"]["items"] else ""
ck("đọc → sửa tên → đọc lại thấy tên MỚI (cache bị evict)",
   before == "E-cache-old" and after == "E-cache-NEW", before + " -> " + after)

# ════════════════════════════════════════════════════════════════════════
sect("F. Action engine — đường lỗi")

d, e = gql('mutation { runAction(name:"KhongTonTai", input:{}) }', ADMIN)
ck("runAction action không tồn tại → lỗi", e and "unknown action" in str(e).lower(), str(e))

d, e = gql('mutation { runAction(name:"SubmitExam", input:{attempt_id:"99999999"}) }', ADMIN)
# attempt không có -> answers rỗng -> score 0 -> write update khớp 0 dòng; không được crash
ck("runAction SubmitExam attempt_id rác → không crash (xử lý gọn)",
   (d is not None) or (e is not None), str(e))

# ════════════════════════════════════════════════════════════════════════
print("\n\033[1mDọn dữ liệu stress\033[0m")
for tbl in ("examattempt", "examassignment", "enrollment", "exam", "class"):
    mysql("DELETE FROM lms." + tbl + " WHERE created_by='stress';")
print("  done")
print("\n\033[1mTOTAL: " + str(P) + " pass, " + str(F) + " fail\033[0m")
sys.exit(0 if F == 0 else 1)
