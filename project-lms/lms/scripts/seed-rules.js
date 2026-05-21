// Seed the full LMS business-rule set into lms_policy.business_rule.
// Run:  docker exec -i mongodb_container mongosh -u thaily -p 'Th@i2004' \
//         --authenticationDatabase admin < scripts/seed-rules.js
//
// Rules are 100% data — this file is just the initial seed; edit rules in
// Mongo (or a dashboard) afterwards, no redeploy. Engine = generic interpreter.
const db = db.getSiblingDB("lms_policy");

// ── helpers ──────────────────────────────────────────────────────────────
const F = (field, op, value) => ({ field, op, value });
const rule = (operation, name, fetch, condition, message, priority = 10) =>
  ({ operation, rule: name, enabled: true, priority, fetch, condition, message });

// fetch the exact row(s) an Update/Delete targets, exposed as `current`
const CURRENT = (entity) =>
  [{ as: "current", entity, where: [], use_request_filters: true }];

// build a forward-only state-machine condition over `field`.
// table: { FROM: [TO, ...] }. Allows: not touching the field, no-op, or a
// declared transition.
function transition(field, table) {
  const parts = [`req.${field} == nil`, `req.${field} == current[0].${field}`];
  for (const from of Object.keys(table)) {
    const tos = table[from].map((t) => `req.${field} == "${t}"`).join(" || ");
    parts.push(`(current[0].${field} == "${from}" && (${tos}))`);
  }
  return `len(current) == 0 || ` + parts.join(" || ");
}
const stateRule = (op, entity, field, table, msg) =>
  rule(op, "status_transition", CURRENT(entity), transition(field, table), msg, 5);

// delete-dependency: block delete while dependent rows reference the victim.
const deleteDep = (op, victimEntity, depEntity, depFK, msg) =>
  rule(op, "no_dependents",
    [{ as: "victim", entity: victimEntity, where: [], use_request_filters: true },
     { as: "deps", entity: depEntity, where: [F(depFK, "EQUAL", "$victim.0.id")] }],
    "len(victim) == 0 || len(deps) == 0", msg, 5);

const rules = [
  // ══ CreateEnrollment ════════════════════════════════════════════════
  rule("CreateEnrollment", "class_exists",
    [{ as: "cls", entity: "Class", where: [F("id", "EQUAL", "$req.class_id")] }],
    "len(cls) > 0", "Lop khong ton tai", 10),
  rule("CreateEnrollment", "class_active",
    [{ as: "cls", entity: "Class", where: [F("id", "EQUAL", "$req.class_id")] }],
    'len(cls) > 0 && cls[0].status == "ACTIVE"', "Lop chua mo hoac da dong", 20),
  rule("CreateEnrollment", "student_is_student",
    [{ as: "u", entity: "User", where: [F("id", "EQUAL", "$req.student_id")] }],
    'len(u) > 0 && u[0].role == "STUDENT"', "Chi student moi enroll duoc", 30),
  rule("CreateEnrollment", "no_duplicate",
    [{ as: "existing", entity: "Enrollment", where: [
      F("class_id", "EQUAL", "$req.class_id"), F("student_id", "EQUAL", "$req.student_id")] }],
    "len(existing) == 0", "Student da enroll lop nay roi", 40),
  rule("CreateEnrollment", "class_not_full",
    [{ as: "cls", entity: "Class", where: [F("id", "EQUAL", "$req.class_id")] },
     { as: "members", entity: "Enrollment", where: [F("class_id", "EQUAL", "$req.class_id")] }],
    "len(cls) == 0 || cls[0].max_students == nil || len(members) < int(cls[0].max_students)",
    "Lop da day si so", 50),
  // Lớp PRIVATE: chỉ vào được khi có ClassInviteKey ACTIVE giáo viên mời
  // ĐÍCH DANH học sinh này (target_student_id khớp). Lớp PUBLIC bỏ qua.
  rule("CreateEnrollment", "private_needs_invite",
    [{ as: "cls", entity: "Class", where: [F("id", "EQUAL", "$req.class_id")] },
     { as: "invite", entity: "ClassInviteKey", where: [
       F("class_id", "EQUAL", "$req.class_id"), F("id", "EQUAL", "$req.invite_key_id")] }],
    'len(cls) == 0 || cls[0].visibility != "PRIVATE" || ' +
    '(len(invite) > 0 && invite[0].status == "KEY_ACTIVE" && ' +
    'invite[0].target_student_id == req.student_id)',
    "Lop private — can loi moi dich danh cua giao vien", 25),

  // ══ CreateExamAttempt ═══════════════════════════════════════════════
  rule("CreateExamAttempt", "exam_exists",
    [{ as: "ex", entity: "Exam", where: [F("id", "EQUAL", "$req.exam_id")] }],
    "len(ex) > 0", "Exam khong ton tai", 10),
  rule("CreateExamAttempt", "must_be_enrolled",
    [{ as: "assignment", entity: "ExamAssignment", where: [F("exam_id", "EQUAL", "$req.exam_id")] },
     { as: "enroll", entity: "Enrollment", where: [
       F("student_id", "EQUAL", "$req.student_id"), F("class_id", "EQUAL", "$assignment.0.class_id")] }],
    "len(enroll) > 0", "Student chua enrolled lop cua exam nay", 20),
  rule("CreateExamAttempt", "within_window",
    [{ as: "assignment", entity: "ExamAssignment", where: [F("exam_id", "EQUAL", "$req.exam_id")] }],
    "len(assignment) > 0 && now >= assignment[0].open_at && now <= assignment[0].close_at",
    "Ngoai thoi gian mo cua exam", 30),
  rule("CreateExamAttempt", "max_attempts",
    [{ as: "assignment", entity: "ExamAssignment", where: [F("exam_id", "EQUAL", "$req.exam_id")] },
     { as: "prior", entity: "ExamAttempt", where: [
       F("exam_id", "EQUAL", "$req.exam_id"), F("student_id", "EQUAL", "$req.student_id")] }],
    "len(assignment) == 0 || len(prior) < int(assignment[0].max_attempts)",
    "Da het so lan lam bai cho phep", 40),

  // ══ CreateExamAssignment ════════════════════════════════════════════
  rule("CreateExamAssignment", "valid_window", [],
    "req.open_at != nil && req.close_at != nil && req.open_at < req.close_at",
    "open_at phai truoc close_at", 10),
  rule("CreateExamAssignment", "exam_exists",
    [{ as: "ex", entity: "Exam", where: [F("id", "EQUAL", "$req.exam_id")] }],
    "len(ex) > 0", "Exam khong ton tai", 20),
  rule("CreateExamAssignment", "class_exists",
    [{ as: "cls", entity: "Class", where: [F("id", "EQUAL", "$req.class_id")] }],
    "len(cls) > 0", "Lop khong ton tai", 25),
  rule("CreateExamAssignment", "not_already_assigned",
    [{ as: "dup", entity: "ExamAssignment", where: [
      F("exam_id", "EQUAL", "$req.exam_id"), F("class_id", "EQUAL", "$req.class_id")] }],
    "len(dup) == 0", "Exam da duoc giao cho lop nay", 30),

  // ══ CreateLesson ════════════════════════════════════════════════════
  rule("CreateLesson", "class_exists",
    [{ as: "cls", entity: "Class", where: [F("id", "EQUAL", "$req.class_id")] }],
    "len(cls) > 0", "Lop khong ton tai", 10),
  rule("CreateLesson", "valid_time", [],
    "req.start_at != nil && req.end_at != nil && req.start_at < req.end_at",
    "start_at phai truoc end_at", 20),

  // ══ CreateLessonVideo ═══════════════════════════════════════════════
  rule("CreateLessonVideo", "lesson_exists",
    [{ as: "ls", entity: "Lesson", where: [F("id", "EQUAL", "$req.lesson_id")] }],
    "len(ls) > 0", "Buoi hoc khong ton tai", 10),

  // ══ CreateLessonAttendance ══════════════════════════════════════════
  rule("CreateLessonAttendance", "lesson_exists",
    [{ as: "ls", entity: "Lesson", where: [F("id", "EQUAL", "$req.lesson_id")] }],
    "len(ls) > 0", "Buoi hoc khong ton tai", 10),
  rule("CreateLessonAttendance", "student_enrolled",
    [{ as: "ls", entity: "Lesson", where: [F("id", "EQUAL", "$req.lesson_id")] },
     { as: "enroll", entity: "Enrollment", where: [
       F("student_id", "EQUAL", "$req.student_id"), F("class_id", "EQUAL", "$ls.0.class_id")] }],
    "len(enroll) > 0", "Student khong thuoc lop cua buoi hoc nay", 20),
  rule("CreateLessonAttendance", "no_duplicate",
    [{ as: "dup", entity: "LessonAttendance", where: [
      F("lesson_id", "EQUAL", "$req.lesson_id"), F("student_id", "EQUAL", "$req.student_id")] }],
    "len(dup) == 0", "Da diem danh buoi hoc nay roi", 30),

  // ══ CreateAttemptAnswer ═════════════════════════════════════════════
  rule("CreateAttemptAnswer", "attempt_in_progress",
    [{ as: "att", entity: "ExamAttempt", where: [F("id", "EQUAL", "$req.attempt_id")] }],
    'len(att) > 0 && att[0].status == "IN_PROGRESS"', "Bai lam khong con mo de tra loi", 10),

  // ══ Profiles / tokens — referential + role consistency ═════════════
  rule("CreateTeacherProfile", "user_is_teacher",
    [{ as: "u", entity: "User", where: [F("id", "EQUAL", "$req.user_id")] }],
    'len(u) > 0 && u[0].role == "TEACHER"', "TeacherProfile chi cho user role TEACHER", 10),
  rule("CreateStudentProfile", "user_is_student",
    [{ as: "u", entity: "User", where: [F("id", "EQUAL", "$req.user_id")] }],
    'len(u) > 0 && u[0].role == "STUDENT"', "StudentProfile chi cho user role STUDENT", 10),
  rule("CreateRefreshToken", "user_exists",
    [{ as: "u", entity: "User", where: [F("id", "EQUAL", "$req.user_id")] }],
    "len(u) > 0", "User khong ton tai", 10),
  rule("CreatePasswordReset", "user_exists",
    [{ as: "u", entity: "User", where: [F("id", "EQUAL", "$req.user_id")] }],
    "len(u) > 0", "User khong ton tai", 10),

  // ══ Class / invite key ═════════════════════════════════════════════
  rule("CreateClass", "teacher_valid",
    [{ as: "u", entity: "User", where: [F("id", "EQUAL", "$req.teacher_id")] }],
    'len(u) > 0 && u[0].role == "TEACHER"', "teacher_id phai la user role TEACHER", 10),
  rule("CreateClassInviteKey", "class_exists",
    [{ as: "cls", entity: "Class", where: [F("id", "EQUAL", "$req.class_id")] }],
    "len(cls) > 0", "Lop khong ton tai", 10),
  rule("CreateClassInviteKey", "target_is_student",
    [{ as: "u", entity: "User", where: [F("id", "EQUAL", "$req.target_student_id")] }],
    'req.target_student_id == nil || (len(u) > 0 && u[0].role == "STUDENT")',
    "target_student_id phai la user role STUDENT", 20),

  // ══ Material ════════════════════════════════════════════════════════
  rule("CreateMaterial", "owner_is_teacher",
    [{ as: "u", entity: "User", where: [F("id", "EQUAL", "$req.owner_teacher_id")] }],
    'len(u) > 0 && u[0].role == "TEACHER"', "owner_teacher_id phai la user role TEACHER", 10),
  // Tài liệu URL phải có url; tài liệu file phải có storage_key.
  rule("CreateMaterial", "source_valid", [],
    '(req.source_type == "SOURCE_URL" && req.url != nil && req.url != "") || ' +
    '(req.source_type == "SOURCE_FILE" && req.storage_key != nil && req.storage_key != "")',
    "Material URL can field url; material FILE can field storage_key", 20),
  rule("CreateMaterialClass", "material_exists",
    [{ as: "m", entity: "Material", where: [F("id", "EQUAL", "$req.material_id")] }],
    "len(m) > 0", "Material khong ton tai", 10),
  rule("CreateMaterialClass", "class_exists",
    [{ as: "cls", entity: "Class", where: [F("id", "EQUAL", "$req.class_id")] }],
    "len(cls) > 0", "Lop khong ton tai", 20),
  rule("CreateMaterialTag", "material_exists",
    [{ as: "m", entity: "Material", where: [F("id", "EQUAL", "$req.material_id")] }],
    "len(m) > 0", "Material khong ton tai", 10),

  // ══ Exam / question ════════════════════════════════════════════════
  rule("CreateExam", "owner_is_teacher",
    [{ as: "u", entity: "User", where: [F("id", "EQUAL", "$req.owner_teacher_id")] }],
    'len(u) > 0 && u[0].role == "TEACHER"', "owner_teacher_id phai la user role TEACHER", 10),
  rule("CreateQuestion", "exam_exists",
    [{ as: "ex", entity: "Exam", where: [F("id", "EQUAL", "$req.exam_id")] }],
    "len(ex) > 0", "Exam khong ton tai", 10),
  rule("CreateQuestionChoice", "question_exists",
    [{ as: "q", entity: "Question", where: [F("id", "EQUAL", "$req.question_id")] }],
    "len(q) > 0", "Question khong ton tai", 10),
  rule("CreateQuestionAnswer", "question_exists",
    [{ as: "q", entity: "Question", where: [F("id", "EQUAL", "$req.question_id")] }],
    "len(q) > 0", "Question khong ton tai", 10),

  // ══ Notification / report ══════════════════════════════════════════
  rule("CreateNotification", "user_exists",
    [{ as: "u", entity: "User", where: [F("id", "EQUAL", "$req.user_id")] }],
    "len(u) > 0", "User khong ton tai", 10),
  rule("CreateCommunityReport", "reporter_exists",
    [{ as: "u", entity: "User", where: [F("id", "EQUAL", "$req.reporter_user_id")] }],
    "len(u) > 0", "Reporter khong ton tai", 10),

  // ══ State machines (Update) ════════════════════════════════════════
  stateRule("UpdateExamAttempt", "ExamAttempt", "status",
    { IN_PROGRESS: ["SUBMITTED", "GRADED"], SUBMITTED: ["GRADED"] },
    "Status attempt chi di 1 chieu: IN_PROGRESS -> SUBMITTED -> GRADED"),
  // Nộp bài (status -> SUBMITTED) phải trong thời lượng exam.duration_min
  // tính từ started_at của attempt.
  rule("UpdateExamAttempt", "within_duration",
    [{ as: "current", entity: "ExamAttempt", where: [], use_request_filters: true },
     { as: "exam", entity: "Exam", where: [F("id", "EQUAL", "$current.0.exam_id")] }],
    'req.status != "SUBMITTED" || len(current) == 0 || len(exam) == 0 || ' +
    'minutesSince(current[0].started_at) <= exam[0].duration_min',
    "Da qua thoi luong lam bai cho phep", 15),
  stateRule("UpdateClass", "Class", "status",
    { DRAFT: ["ACTIVE", "ARCHIVED"], ACTIVE: ["ARCHIVED"] },
    "Status lop chi di 1 chieu: DRAFT -> ACTIVE -> ARCHIVED"),
  stateRule("UpdateEnrollment", "Enrollment", "status",
    { ENROLL_ACTIVE: ["ENROLL_REMOVED"] },
    "Enrollment chi di tu ENROLL_ACTIVE -> ENROLL_REMOVED"),
  stateRule("UpdateClassInviteKey", "ClassInviteKey", "status",
    { KEY_ACTIVE: ["KEY_USED", "KEY_REVOKED", "KEY_EXPIRED"] },
    "Invite key da o trang thai cuoi, khong doi duoc"),
  stateRule("UpdateLesson", "Lesson", "status",
    { SCHEDULED: ["ONGOING", "CANCELLED"], ONGOING: ["DONE", "CANCELLED"] },
    "Status buoi hoc khong hop le"),
  stateRule("UpdateLessonVideo", "LessonVideo", "status",
    { UPLOADING: ["PROCESSING", "FAILED"], PROCESSING: ["READY", "FAILED"] },
    "Status video khong hop le"),
  stateRule("UpdateMaterial", "Material", "community_status",
    { COMM_DRAFT: ["COMM_PENDING"], COMM_PENDING: ["COMM_APPROVED", "COMM_REJECTED"] },
    "community_status material khong hop le"),
  stateRule("UpdateExam", "Exam", "community_status",
    { EXAM_DRAFT: ["EXAM_PENDING"], EXAM_PENDING: ["EXAM_APPROVED", "EXAM_REJECTED"] },
    "community_status exam khong hop le"),
  stateRule("UpdateCommunityReport", "CommunityReport", "status",
    { RPT_OPEN: ["RPT_DISMISSED", "RPT_ACTIONED"] },
    "Report da xu ly roi, khong doi duoc"),

  // ══ Delete-dependency ══════════════════════════════════════════════
  deleteDep("DeleteClass", "Class", "Enrollment", "class_id",
    "Khong xoa duoc lop con enrollment"),
  deleteDep("DeleteExam", "Exam", "ExamAssignment", "exam_id",
    "Khong xoa duoc exam da giao cho lop"),
  deleteDep("DeleteLesson", "Lesson", "LessonAttendance", "lesson_id",
    "Khong xoa duoc buoi hoc da co diem danh"),
  deleteDep("DeleteQuestion", "Question", "AttemptAnswer", "question_id",
    "Khong xoa duoc question da co bai lam tra loi"),
  deleteDep("DeleteUser", "User", "Class", "teacher_id",
    "Khong xoa duoc user dang la teacher cua lop"),
];

db.business_rule.deleteMany({});
db.business_rule.insertMany(rules);
print("business_rule total: " + db.business_rule.countDocuments());
print("operations guarded: " + db.business_rule.distinct("operation").length);
