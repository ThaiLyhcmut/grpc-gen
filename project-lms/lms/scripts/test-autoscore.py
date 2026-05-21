#!/usr/bin/env python3
"""Test auto-scoring — action SubmitAnswer chấm từng câu trắc nghiệm.

SubmitAnswer fetch các choice của câu hỏi, so với choice học sinh chọn,
gán auto_score = points (đúng) hoặc 0 (sai). SubmitExam cộng tổng.

Tự dọn data (created_by='scoretest'). Cần LMS đang chạy.
"""
import base64, hashlib, hmac, json, subprocess, sys, urllib.request


def mint(sub, role):
    b = lambda d: base64.urlsafe_b64encode(d).rstrip(b"=")
    h = b(json.dumps({"alg": "HS256", "typ": "JWT"}, separators=(",", ":")).encode())
    p = b(json.dumps({"sub": sub, "role": role}, separators=(",", ":")).encode())
    s = b(hmac.new(b"lms-dev-secret", h + b"." + p, hashlib.sha256).digest())
    return (h + b"." + p + b"." + s).decode()


ADMIN = mint("999", "ADMIN")


def gql(q):
    try:
        d = json.loads(urllib.request.urlopen(urllib.request.Request(
            "http://localhost:8080/query", json.dumps({"query": q}).encode(),
            {"Content-Type": "application/json", "Authorization": "Bearer " + ADMIN})).read())
        return d.get("data"), d.get("errors")
    except Exception as e:
        return None, [{"message": str(e)}]


P = F = 0
def ck(name, cond, detail=""):
    global P, F
    P, F = P + bool(cond), F + (not cond)
    print(("  \033[32m✓\033[0m " if cond else "  \033[31m✗\033[0m ") + name
          + ("" if cond else "  — " + detail))


def one(data, key):
    return data[key]["id"] if data else None


# ── setup ───────────────────────────────────────────────────────────────
d, _ = gql('mutation { createClass(input: {teacher_id:"2", name:"AutoScore", '
           'status:ACTIVE, visibility:PUBLIC, created_by:"scoretest"}) { id } }')
cls = one(d, "createClass")
d, _ = gql('mutation { createExam(input: {owner_teacher_id:"2", title:"Exam cham tu dong", '
           'duration_min:60, total_points:20.0, visibility:EXAM_CLASS, shuffle_questions:false, '
           'shuffle_choices:false, show_result_mode:IMMEDIATELY, created_by:"scoretest"}) { id } }')
exam = one(d, "createExam")


def make_question(pos):
    d, _ = gql('mutation { createQuestion(input: {exam_id:"' + exam + '", position:' + str(pos) +
               ', type:SINGLE, content:"Cau ' + str(pos) + '", points:10.0, '
               'created_by:"scoretest"}) { id } }')
    q = one(d, "createQuestion")
    ch = {}
    for label, correct in (("A", "true"), ("B", "false")):
        d, _ = gql('mutation { createQuestionChoice(input: {question_id:"' + q + '", position:1, '
                   'content:"' + label + '", is_correct:' + correct + ', '
                   'created_by:"scoretest"}) { id } }')
        ch[label] = one(d, "createQuestionChoice")
    return q, ch  # ch["A"] = đáp án đúng, ch["B"] = sai


q1, c1 = make_question(1)
q2, c2 = make_question(2)
gql('mutation { createExamAssignment(input: {exam_id:"' + exam + '", class_id:"' + cls +
    '", open_at:"2026-01-01T00:00:00Z", close_at:"2027-12-31T00:00:00Z", max_attempts:5, '
    'created_by:"scoretest"}) { id } }')
gql('mutation { createEnrollment(input: {class_id:"' + cls + '", student_id:"4", '
    'joined_at:"2026-06-01T00:00:00Z", status:ENROLL_ACTIVE, created_by:"scoretest"}) { id } }')
d, _ = gql('mutation { createExamAttempt(input: {exam_id:"' + exam + '", student_id:"4", '
           'started_at:"2026-05-21T09:00:00Z", status:IN_PROGRESS, created_by:"scoretest"}) { id } }')
att = one(d, "createExamAttempt")
print("setup: class=%s exam=%s attempt=%s (2 câu, mỗi câu 10đ)" % (cls, exam, att))

# ── chấm từng câu ───────────────────────────────────────────────────────
def submit_answer(question, choice):
    return gql('mutation { runAction(name:"SubmitAnswer", input:{attempt_id:"' + att +
               '", question_id:"' + question + '", choice_id:"' + choice + '"}) }')

d, e = submit_answer(q1, c1["A"])  # đúng
r = d and d.get("runAction")
ck("Câu 1 chọn đáp án ĐÚNG → correct=true, score=10",
   r and r.get("correct") is True and r.get("score") == 10.0, str(e or r))

d, e = submit_answer(q2, c2["B"])  # sai
r = d and d.get("runAction")
ck("Câu 2 chọn đáp án SAI → correct=false, score=0",
   r and r.get("correct") is False and r.get("score") == 0.0, str(e or r))

# ── nộp bài: tổng = 10 + 0 ──────────────────────────────────────────────
d, e = gql('mutation { runAction(name:"SubmitExam", input:{attempt_id:"' + att + '"}) }')
r = d and d.get("runAction")
ck("SubmitExam → tổng điểm = 10", r and r.get("score") == 10.0, str(e or r))

d, _ = gql('{ listExamAttempt(search:{filters:[{condition:{field:"id",operator:EQUAL,'
           'values:["' + att + '"]}}]}) { items { status total_score } } }')
it = d["listExamAttempt"]["items"][0] if d and d["listExamAttempt"]["items"] else {}
ck("Bài thi: status=SUBMITTED, total_score=10",
   it.get("status") == "SUBMITTED" and it.get("total_score") == 10.0, str(it))

for tbl in ("attemptanswer", "examattempt", "examassignment", "enrollment",
            "questionchoice", "question", "exam", "class"):
    subprocess.run(["docker", "exec", "mysql_container", "mysql", "-uthaily", "-pTh@i2004",
                    "-e", "DELETE FROM lms." + tbl + " WHERE created_by='scoretest';"],
                   capture_output=True)
print("\n\033[1mTOTAL: " + str(P) + " pass, " + str(F) + " fail\033[0m")
sys.exit(0 if F == 0 else 1)
