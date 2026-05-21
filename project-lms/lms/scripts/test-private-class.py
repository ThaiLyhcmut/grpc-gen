#!/usr/bin/env python3
"""Test lớp public/private + lời mời đích danh.

- Lớp PUBLIC: học sinh tự ghi danh.
- Lớp PRIVATE: chỉ vào được khi có ClassInviteKey ACTIVE giáo viên mời
  ĐÍCH DANH học sinh đó (target_student_id khớp student_id).

Tự dọn data (created_by='pvtest'), chạy lại được. Cần LMS đang chạy.
"""
import base64, hashlib, hmac, json, subprocess, sys, time, urllib.request


def mint(sub, role):
    b = lambda d: base64.urlsafe_b64encode(d).rstrip(b"=")
    h = b(json.dumps({"alg": "HS256", "typ": "JWT"}, separators=(",", ":")).encode())
    p = b(json.dumps({"sub": sub, "role": role}, separators=(",", ":")).encode())
    s = b(hmac.new(b"lms-dev-secret", h + b"." + p, hashlib.sha256).digest())
    return (h + b"." + p + b"." + s).decode()


TOK = {"teacher": mint("2", "TEACHER"), "s4": mint("4", "STUDENT"), "s3": mint("3", "STUDENT")}


def gql(q, who):
    body = json.dumps({"query": q}).encode()
    hd = {"Content-Type": "application/json", "Authorization": "Bearer " + TOK[who]}
    try:
        d = json.loads(urllib.request.urlopen(
            urllib.request.Request("http://localhost:8080/query", body, hd)).read())
        return d.get("data"), d.get("errors")
    except Exception as e:
        return None, [{"message": str(e)}]


P = F = 0
def ck(name, cond, detail=""):
    global P, F
    P, F = P + bool(cond), F + (not cond)
    print(("  \033[32m✓\033[0m " if cond else "  \033[31m✗\033[0m ") + name
          + ("" if cond else "  — " + detail))


# GraphQL dùng {} dày đặc nên tránh f-string — nối chuỗi thường.
K = str(int(time.time()))


def m_createClass(name, vis):
    return ('mutation { createClass(input: {teacher_id: "2", name: "' + name +
            '", status: ACTIVE, visibility: ' + vis + ', created_by: "pvtest"}) { id } }')


def m_enroll(class_id, student_id, invite=None):
    inv = (', invite_key_id: "' + invite + '"') if invite else ''
    return ('mutation { createEnrollment(input: {class_id: "' + class_id +
            '", student_id: "' + student_id + '"' + inv +
            ', joined_at: "2026-06-01T00:00:00Z", status: ENROLL_ACTIVE,'
            ' created_by: "pvtest"}) { id } }')


def m_invite(class_id, target, suffix):
    return ('mutation { createClassInviteKey(input: {class_id: "' + class_id +
            '", key_code: "K-' + K + '-' + suffix + '", created_by_teacher_id: "2",'
            ' target_student_id: "' + target + '", status: KEY_ACTIVE,'
            ' created_by: "pvtest"}) { id } }')


d, e = gql(m_createClass("Public " + K, "PUBLIC"), "teacher")
pub = d and d["createClass"]["id"]
ck("Tạo lớp PUBLIC → id=" + str(pub), pub and not e, str(e))

d, e = gql(m_enroll(pub, "4"), "s4")
ck("HS ghi danh lớp PUBLIC (không cần mời) → ALLOW", d and not e, str(e))

d, e = gql(m_createClass("Private " + K, "PRIVATE"), "teacher")
priv = d and d["createClass"]["id"]
ck("Tạo lớp PRIVATE → id=" + str(priv), priv and not e, str(e))

d, e = gql(m_enroll(priv, "4"), "s4")
ck("HS ghi danh PRIVATE KHÔNG có mời → DENY", e and "private" in str(e).lower(), str(e))

d, e = gql(m_invite(priv, "4", "4"), "teacher")
inv = d and d["createClassInviteKey"]["id"]
ck("GV tạo lời mời đích danh HS 4 → id=" + str(inv), inv and not e, str(e))

d, e = gql(m_enroll(priv, "4", inv), "s4")
ck("HS 4 ghi danh PRIVATE với lời mời đích danh → ALLOW", d and not e, str(e))

d, e = gql(m_enroll(priv, "3", inv), "s3")
ck("HS 3 dùng lời mời của HS 4 → DENY (sai đích danh)", e and "private" in str(e).lower(), str(e))

d, e = gql(m_invite(priv, "2", "bad"), "teacher")
ck("GV mời đích danh user 2 (là TEACHER) → DENY", e and "student" in str(e).lower(), str(e))

for tbl in ("enrollment", "classinvitekey", "class"):
    subprocess.run(["docker", "exec", "mysql_container", "mysql", "-uthaily", "-pTh@i2004",
                    "-e", "DELETE FROM lms." + tbl + " WHERE created_by='pvtest';"],
                   capture_output=True)
print("\n\033[1mTOTAL: " + str(P) + " pass, " + str(F) + " fail\033[0m")
sys.exit(0 if F == 0 else 1)
