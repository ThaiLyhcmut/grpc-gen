#!/usr/bin/env python3
"""Scenario test — exercises the LMS the way real users would.

A teacher sets up a class + exam; a student enrols, sits the exam, submits;
the teacher grades. Negative steps are woven in exactly where a real user
would hit a wall (a student trying to teach, double-enrolment, grading your
own paper, re-sitting past the attempt limit, ...).

Every step is a real GraphQL call as the correct role; ids are chained from
one step to the next. Created rows are tagged created_by='scenario' and
deleted at the end, so the script is repeatable.
"""
import base64, hashlib, hmac, json, subprocess, sys, urllib.request

GATEWAY = "http://localhost:8080/query"
SECRET = b"lms-dev-secret"


def mint(sub, role):
    b64 = lambda d: base64.urlsafe_b64encode(d).rstrip(b"=")
    h = b64(json.dumps({"alg": "HS256", "typ": "JWT"}, separators=(",", ":")).encode())
    p = b64(json.dumps({"sub": sub, "role": role}, separators=(",", ":")).encode())
    sig = b64(hmac.new(SECRET, h + b"." + p, hashlib.sha256).digest())
    return (h + b"." + p + b"." + sig).decode()


TOK = {
    "teacher":  mint("2", "TEACHER"),    # user 2 — teacher@x.com
    "student":  mint("4", "STUDENT"),    # user 4 — student2@x.com (the learner)
    "student3": mint("3", "STUDENT"),    # user 3 — a DIFFERENT student
    "admin":    mint("999", "ADMIN"),
}


def gql(query, who=None):
    body = json.dumps({"query": query}).encode()
    headers = {"Content-Type": "application/json"}
    if who:
        headers["Authorization"] = "Bearer " + TOK[who]
    try:
        resp = json.loads(urllib.request.urlopen(
            urllib.request.Request(GATEWAY, body, headers)).read())
    except Exception as e:
        return None, [{"message": f"HTTP {e}"}]
    return resp.get("data"), resp.get("errors")


def mysql(sql):
    subprocess.run(["docker", "exec", "mysql_container", "mysql",
                    "-uthaily", "-pTh@i2004", "-e", sql], capture_output=True)


PASS = FAIL = 0
def ok(name, cond, detail=""):
    global PASS, FAIL
    mark = "\033[32m✓\033[0m" if cond else "\033[31m✗\033[0m"
    PASS, FAIL = PASS + bool(cond), FAIL + (not cond)
    print(f"   {mark} {name}" + (f"   — {detail}" if detail and not cond else ""))

def act(t):
    print(f"\n\033[1m{t}\033[0m")

def is_auth_denied(e):
    return bool(e) and any(m in (x.get("message", "").lower())
                           for x in e for m in ("unauthenticated", "forbidden"))
def denied(e, frag):
    return bool(e) and any(frag.lower() in x.get("message", "").lower() for x in e)


# ════════════════════════════════════════════════════════════════════════
act("MÀN 1 — Giáo viên (user 2) dựng lớp + đề thi")

d, e = gql('mutation { createClass(input: {teacher_id:"2", name:"Lop Toan 12A scenario",'
           ' status:ACTIVE, visibility:PUBLIC, max_students:30, created_by:"scenario"}) { id } }', "teacher")
class_id = d and d["createClass"]["id"]
ok(f"GV tạo lớp → id={class_id}", class_id and not e, str(e))

d, e = gql(f'mutation {{ createLesson(input: {{class_id:"{class_id}", title:"Bai 1",'
           ' start_at:"2026-06-01T08:00:00Z", end_at:"2026-06-01T10:00:00Z",'
           ' mode:OFFLINE, status:SCHEDULED, created_by:"scenario"}) { id } }', "teacher")
lesson_id = d and d["createLesson"]["id"]
ok(f"GV tạo buổi học → id={lesson_id}", lesson_id and not e, str(e))

d, e = gql('mutation { createExam(input: {owner_teacher_id:"2", title:"Kiem tra giua ky",'
           ' duration_min:45, total_points:10.0, visibility:EXAM_CLASS,'
           ' shuffle_questions:false, shuffle_choices:false, show_result_mode:IMMEDIATELY,'
           ' created_by:"scenario"}) { id } }', "teacher")
exam_id = d and d["createExam"]["id"]
ok(f"GV tạo đề thi → id={exam_id}", exam_id and not e, str(e))

d, e = gql(f'mutation {{ createQuestion(input: {{exam_id:"{exam_id}", position:1,'
           ' type:SINGLE, content:"1+1=?", points:10.0, created_by:"scenario"}) { id } }', "teacher")
question_id = d and d["createQuestion"]["id"]
ok(f"GV tạo câu hỏi → id={question_id}", question_id and not e, str(e))

d, e = gql(f'mutation {{ createExamAssignment(input: {{exam_id:"{exam_id}", class_id:"{class_id}",'
           ' open_at:"2026-01-01T00:00:00Z", close_at:"2027-12-31T00:00:00Z", max_attempts:2,'
           ' created_by:"scenario"}) { id } }', "teacher")
assign_id = d and d["createExamAssignment"]["id"]
ok(f"GV giao đề cho lớp → id={assign_id}", assign_id and not e, str(e))

# ════════════════════════════════════════════════════════════════════════
act("MÀN 2 — Học sinh KHÔNG làm được việc của giáo viên")

_, e = gql('mutation { createClass(input: {teacher_id:"2", name:"hack", status:ACTIVE, visibility:PUBLIC,'
           ' created_by:"scenario"}) { id } }', "student")
ok("HS tạo lớp → bị chặn (@auth TEACHER)", is_auth_denied(e), str(e))

_, e = gql(f'mutation {{ createQuestion(input: {{exam_id:"{exam_id}", position:2, type:SINGLE,'
           ' content:"x", points:1.0, created_by:"scenario"}) { id } }', "student")
ok("HS thêm câu hỏi → bị chặn (@auth TEACHER)", is_auth_denied(e), str(e))

_, e = gql('mutation { createClass(input: {teacher_id:"2", name:"x", status:ACTIVE, visibility:PUBLIC,'
           ' created_by:"scenario"}) { id } }', None)
ok("Khách vãng lai tạo lớp → bị chặn (chưa đăng nhập)", is_auth_denied(e), str(e))

# ════════════════════════════════════════════════════════════════════════
act("MÀN 3 — Học sinh (user 4) ghi danh + làm bài")

d, e = gql(f'mutation {{ createEnrollment(input: {{class_id:"{class_id}", student_id:"4",'
           ' joined_at:"2026-05-21T08:00:00Z", status:ENROLL_ACTIVE,'
           ' created_by:"scenario"}) { id } }', "student")
enroll_id = d and d["createEnrollment"]["id"]
ok(f"HS ghi danh vào lớp → id={enroll_id}", enroll_id and not e, str(e))

_, e = gql(f'mutation {{ createEnrollment(input: {{class_id:"{class_id}", student_id:"4",'
           ' joined_at:"2026-05-21T08:00:00Z", status:ENROLL_ACTIVE,'
           ' created_by:"scenario"}) { id } }', "student")
ok("HS ghi danh LẦN 2 → bị chặn (trùng)", denied(e, "da enroll"), str(e))

d, e = gql(f'mutation {{ createExamAttempt(input: {{exam_id:"{exam_id}", student_id:"4",'
           ' started_at:"2026-05-21T09:00:00Z", status:IN_PROGRESS,'
           ' created_by:"scenario"}) { id } }', "student")
attempt_id = d and d["createExamAttempt"]["id"]
ok(f"HS bắt đầu làm bài → attempt id={attempt_id}", attempt_id and not e, str(e))

d, e = gql(f'mutation {{ createAttemptAnswer(input: {{attempt_id:"{attempt_id}",'
           f' question_id:"{question_id}", text_answer:"2", auto_score:9.0,'
           ' created_by:"scenario"}) { id } }', "student")
ok("HS nộp câu trả lời (auto_score 9.0)", d and not e, str(e))

d, e = gql(f'mutation {{ runAction(name:"SubmitExam", input:{{attempt_id:"{attempt_id}"}}) }}', "student")
submitted_score = d and d.get("runAction", {}).get("score")
ok(f"HS nộp bài (runAction SubmitExam) → score={submitted_score}",
   d and not e and submitted_score == 9.0, str(e))

d, _ = gql(f'{{ listExamAttempt(search:{{filters:[{{condition:{{field:"id",operator:EQUAL,'
           f'values:["{attempt_id}"]}}}}]}}) {{ items{{ status total_score }} }} }}', "teacher")
att = d["listExamAttempt"]["items"][0] if d and d["listExamAttempt"]["items"] else {}
ok("Bài thi sau nộp: status=SUBMITTED, total_score=9.0",
   att.get("status") == "SUBMITTED" and att.get("total_score") == 9.0, str(att))

# ════════════════════════════════════════════════════════════════════════
act("MÀN 4 — Học sinh KHÔNG tự chấm, KHÔNG thi lớp chưa ghi danh")

_, e = gql(f'mutation {{ updateExamAttempt(filters:[{{condition:{{field:"id",operator:EQUAL,'
           f'values:["{attempt_id}"]}}}}], input:{{status:GRADED, updated_by:"scenario"}})'
           ' { affected_count } }', "student")
ok("HS tự chấm bài mình → bị chặn (@auth TEACHER)", is_auth_denied(e), str(e))

# student 3 — không ghi danh lớp mới — thử thi đề của lớp đó
_, e = gql(f'mutation {{ createExamAttempt(input: {{exam_id:"{exam_id}", student_id:"3",'
           ' started_at:"2026-05-21T09:00:00Z", status:IN_PROGRESS,'
           ' created_by:"scenario"}) { id } }', "student3")
ok("HS khác (chưa ghi danh) thi đề này → bị chặn (chưa enrolled)",
   denied(e, "chua enrolled"), str(e))

# ════════════════════════════════════════════════════════════════════════
act("MÀN 5 — Giáo viên chấm điểm")

d, e = gql(f'mutation {{ updateExamAttempt(filters:[{{condition:{{field:"id",operator:EQUAL,'
           f'values:["{attempt_id}"]}}}}], input:{{status:GRADED, manual_score:9.0,'
           ' updated_by:"scenario"}) { affected_count } }', "teacher")
ok("GV chấm bài → status GRADED", d and not e, str(e))

_, e = gql(f'mutation {{ updateExamAttempt(filters:[{{condition:{{field:"id",operator:EQUAL,'
           f'values:["{attempt_id}"]}}}}], input:{{status:IN_PROGRESS, updated_by:"scenario"}})'
           ' { affected_count } }', "teacher")
ok("GV lùi bài đã chấm về IN_PROGRESS → bị chặn (state-machine)",
   denied(e, "1 chieu"), str(e))

# ════════════════════════════════════════════════════════════════════════
act("MÀN 6 — Giới hạn số lần thi (max_attempts = 2)")

d, e = gql(f'mutation {{ createExamAttempt(input: {{exam_id:"{exam_id}", student_id:"4",'
           ' started_at:"2026-05-21T11:00:00Z", status:IN_PROGRESS,'
           ' created_by:"scenario"}) { id } }', "student")
ok("HS thi LẦN 2 (còn lượt) → cho phép", d and not e, str(e))

_, e = gql(f'mutation {{ createExamAttempt(input: {{exam_id:"{exam_id}", student_id:"4",'
           ' started_at:"2026-05-21T12:00:00Z", status:IN_PROGRESS,'
           ' created_by:"scenario"}) { id } }', "student")
ok("HS thi LẦN 3 → bị chặn (hết lượt)", denied(e, "het so lan"), str(e))

# ════════════════════════════════════════════════════════════════════════
print("\n\033[1mDọn dữ liệu kịch bản\033[0m")
for tbl in ("attemptanswer", "examattempt", "examassignment", "question",
            "enrollment", "lesson", "exam", "class"):
    mysql(f"DELETE FROM lms.{tbl} WHERE created_by='scenario';")
print("   done")

print(f"\n\033[1mTOTAL: {PASS} pass, {FAIL} fail\033[0m")
sys.exit(0 if FAIL == 0 else 1)
