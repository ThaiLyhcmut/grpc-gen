import React, { useEffect, useState } from 'react'
import { mintJWT, gql } from './lib.js'

// Dev roles — each mints a JWT in the browser. anon = no token.
const ROLES = {
  anon:    { label: 'Khách (anon)', sub: null, role: null },
  student: { label: 'Student · id 3', sub: '3', role: 'STUDENT' },
  teacher: { label: 'Teacher · id 2', sub: '2', role: 'TEACHER' },
  admin:   { label: 'Admin', sub: '999', role: 'ADMIN' },
}

const PRESETS = [
  ['Đọc', 'Lớp học', '{ listClass { items { id name status visibility teacher_id max_students } total } }'],
  ['Đọc', 'Người dùng', '{ listUser { items { id full_name role status email password_hash } total } }'],
  ['Đọc', 'Đề thi', '{ listExam { items { id title duration_min total_points } total } }'],
  ['Đọc', 'Buổi học', '{ listLesson { items { id title class_id } total } }'],
  ['Đọc', 'Ghi danh', '{ listEnrollment { items { id class_id student_id status } total } }'],
  ['Đọc', 'Bài thi', '{ listExamAttempt { items { id exam_id student_id status total_score } total } }'],
  ['Đọc', 'Lớp + GV + buổi học (cross-service)',
    '{ listClass { items { id name teacher { id full_name role } lessons { id title } } } }'],
  ['Ghi', 'Tạo lớp PUBLIC (cần TEACHER)',
    'mutation {\n  createClass(input: {teacher_id: "2", name: "Lớp public", status: ACTIVE, visibility: PUBLIC, created_by: "ui"}) {\n    id name visibility\n  }\n}'],
  ['Ghi', 'Tạo lớp PRIVATE (cần TEACHER)',
    'mutation {\n  createClass(input: {teacher_id: "2", name: "Lớp private", status: ACTIVE, visibility: PRIVATE, created_by: "ui"}) {\n    id name visibility\n  }\n}'],
  ['Ghi', 'Ghi danh (cần STUDENT)',
    'mutation {\n  createEnrollment(input: {class_id: "3", student_id: "4", joined_at: "2026-06-01T00:00:00Z", status: ENROLL_ACTIVE, created_by: "ui"}) {\n    id\n  }\n}'],
  ['Ghi', 'Xoá user (cần ADMIN)',
    'mutation {\n  deleteUser(filters: [{condition: {field: "id", operator: EQUAL, values: ["99999"]}}]) {\n    affected_count\n  }\n}'],
  ['Ghi', 'runAction · SubmitExam',
    'mutation {\n  runAction(name: "SubmitExam", input: {attempt_id: "1"})\n}'],
]

function ResultView({ result }) {
  if (!result) return <div className="hint">Chọn truy vấn bên trái hoặc gõ rồi bấm Chạy.</div>
  const errs = result.errors || []
  const data = result.data
  // find a list-shaped payload { items: [...] }
  let table = null
  if (data) {
    for (const k of Object.keys(data)) {
      const v = data[k]
      if (v && Array.isArray(v.items)) { table = { name: k, rows: v.items, meta: v }; break }
    }
  }
  return (
    <div className="result">
      {errs.length > 0 && (
        <div className="errs">
          {errs.map((e, i) => <div key={i} className="err">⛔ {e.message}</div>)}
        </div>
      )}
      {table && <DataTable {...table} />}
      {data && !table && <pre className="json">{JSON.stringify(data, null, 2)}</pre>}
      {!data && errs.length === 0 && <pre className="json">{JSON.stringify(result, null, 2)}</pre>}
    </div>
  )
}

function DataTable({ name, rows, meta }) {
  if (rows.length === 0) return <div className="hint">{name}: 0 dòng.</div>
  const cols = [...new Set(rows.flatMap((r) => Object.keys(r)))]
  const cell = (v) => {
    if (v === null || v === undefined) return <span className="null">∅</span>
    if (typeof v === 'object') return <span className="obj">{JSON.stringify(v)}</span>
    return String(v)
  }
  return (
    <div>
      <div className="tmeta">
        {name} — {rows.length} dòng
        {meta.total != null && ` / total ${meta.total}`}
      </div>
      <div className="twrap">
        <table>
          <thead><tr>{cols.map((c) => <th key={c}>{c}</th>)}</tr></thead>
          <tbody>
            {rows.map((r, i) => (
              <tr key={i}>{cols.map((c) => <td key={c}>{cell(r[c])}</td>)}</tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

export default function App() {
  const [roleKey, setRoleKey] = useState('teacher')
  const [token, setToken] = useState(null)
  const [query, setQuery] = useState(PRESETS[0][2])
  const [result, setResult] = useState(null)
  const [loading, setLoading] = useState(false)
  const [ms, setMs] = useState(null)

  // (re)mint the token whenever the role changes
  useEffect(() => {
    const r = ROLES[roleKey]
    if (!r.sub) { setToken(null); return }
    mintJWT(r.sub, r.role).then(setToken)
  }, [roleKey])

  async function run(q) {
    const text = q ?? query
    setLoading(true)
    const t0 = performance.now()
    try {
      const res = await gql(text, token)
      setResult(res)
    } catch (e) {
      setResult({ errors: [{ message: String(e) }] })
    } finally {
      setMs(Math.round(performance.now() - t0))
      setLoading(false)
    }
  }

  return (
    <div className="app">
      <header>
        <div className="brand">LMS <span>Gateway UI</span></div>
        <div className="roles">
          {Object.entries(ROLES).map(([k, r]) => (
            <button
              key={k}
              className={'role' + (k === roleKey ? ' on' : '')}
              onClick={() => setRoleKey(k)}
            >{r.label}</button>
          ))}
        </div>
        <div className="who" title={token || ''}>
          {token ? '🔑 ' + roleKey : '— chưa đăng nhập —'}
        </div>
      </header>

      <main>
        <aside>
          {['Đọc', 'Ghi'].map((g) => (
            <div key={g} className="pgroup">
              <h3>{g}</h3>
              {PRESETS.filter((p) => p[0] === g).map((p, i) => (
                <button key={i} className="preset" onClick={() => { setQuery(p[2]); run(p[2]) }}>
                  {p[1]}
                </button>
              ))}
            </div>
          ))}
        </aside>

        <section className="console">
          <textarea
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            spellCheck={false}
          />
          <div className="bar">
            <button className="run" onClick={() => run()} disabled={loading}>
              {loading ? '…' : 'Chạy ▶'}
            </button>
            {ms != null && <span className="ms">{ms} ms · role: {roleKey}</span>}
          </div>
          <ResultView result={result} />
        </section>
      </main>
    </div>
  )
}
