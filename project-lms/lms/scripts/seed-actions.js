// Seed dynamic actions vào lms_policy.action.
// Run: docker exec -i mongodb_container mongosh -u thaily -p 'Th@i2004' \
//        --authenticationDatabase admin < scripts/seed-actions.js
//
// Action = pipeline fetch/compute/write, gọi qua mutation runAction(name,input).
const db = db.getSiblingDB("lms_policy");

db.action.deleteMany({});
db.action.insertMany([
  // ── SubmitAnswer: chấm tự động 1 câu trắc nghiệm SINGLE ────────────────
  // input: attempt_id, question_id, choice_id (id đáp án học sinh chọn)
  {
    name: "SubmitAnswer",
    steps: [
      { kind: "fetch", as: "q", entity: "Question",
        where: [{ field: "id", op: "EQUAL", value: "$input.question_id" }] },
      { kind: "fetch", as: "choices", entity: "QuestionChoice",
        where: [{ field: "question_id", op: "EQUAL", value: "$input.question_id" }] },
      // đúng nếu choice học sinh chọn nằm trong các choice is_correct
      { kind: "compute", as: "correct",
        expr: "len(q) > 0 && len(filter(choices, .id == input.choice_id && .is_correct)) > 0" },
      { kind: "compute", as: "score",
        expr: "correct ? q[0].points : 0.0" },
      { kind: "write", op: "CreateAttemptAnswer",
        set: {
          attempt_id: "$input.attempt_id",
          question_id: "$input.question_id",
          selected_choice_ids: "$input.choice_id",
          auto_score: "$score",
          created_by: "action",
        } },
    ],
    output: "{ok: true, correct: correct, score: score}",
  },

  // ── SubmitExam: tổng điểm = Σ auto_score các câu, chuyển bài sang SUBMITTED ─
  // input: attempt_id
  {
    name: "SubmitExam",
    steps: [
      { kind: "fetch", as: "attempt", entity: "ExamAttempt",
        where: [{ field: "id", op: "EQUAL", value: "$input.attempt_id" }] },
      { kind: "fetch", as: "answers", entity: "AttemptAnswer",
        where: [{ field: "attempt_id", op: "EQUAL", value: "$input.attempt_id" }] },
      { kind: "compute", as: "score",
        expr: "sum(map(answers, .auto_score == nil ? 0.0 : .auto_score))" },
      { kind: "write", op: "UpdateExamAttempt",
        where: [{ field: "id", op: "EQUAL", value: "$input.attempt_id" }],
        set: {
          status: "SUBMITTED",
          auto_score: "$score",
          total_score: "$score",
          updated_by: "action",
        } },
    ],
    output: "{ok: true, attempt_id: input.attempt_id, score: score}",
  },
]);

print("actions: " + db.action.countDocuments() + " — " +
      db.action.distinct("name").join(", "));
