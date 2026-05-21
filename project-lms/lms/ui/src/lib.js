// In-browser JWT (HS256) — secret must match JWT_HMAC_SECRET in run-lms.sh.
const SECRET = 'lms-dev-secret'

function b64url(bytes) {
  let s = ''
  for (const b of bytes) s += String.fromCharCode(b)
  return btoa(s).replace(/=/g, '').replace(/\+/g, '-').replace(/\//g, '_')
}

// mintJWT builds a dev token for a given user id + role. No expiry — dev only.
export async function mintJWT(sub, role) {
  const enc = new TextEncoder()
  const head = b64url(enc.encode(JSON.stringify({ alg: 'HS256', typ: 'JWT' })))
  const body = b64url(enc.encode(JSON.stringify({ sub, role })))
  const key = await crypto.subtle.importKey(
    'raw', enc.encode(SECRET), { name: 'HMAC', hash: 'SHA-256' }, false, ['sign'])
  const sig = await crypto.subtle.sign('HMAC', key, enc.encode(head + '.' + body))
  return head + '.' + body + '.' + b64url(new Uint8Array(sig))
}

// gql POSTs a query to the gateway (proxied to :8080 by vite.config.js).
export async function gql(query, token) {
  const headers = { 'Content-Type': 'application/json' }
  if (token) headers.Authorization = 'Bearer ' + token
  const res = await fetch('/query', {
    method: 'POST',
    headers,
    body: JSON.stringify({ query }),
  })
  return res.json()
}
