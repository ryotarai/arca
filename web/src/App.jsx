import { useEffect, useState } from 'react'
import { Link, Navigate, Route, Routes, useNavigate } from 'react-router-dom'

async function api(path, options = {}) {
  const response = await fetch(path, {
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
      ...(options.headers || {}),
    },
    ...options,
  })

  let body = null
  try {
    body = await response.json()
  } catch {
    body = null
  }

  return { response, body }
}

export function App() {
  const [loading, setLoading] = useState(true)
  const [user, setUser] = useState(null)

  useEffect(() => {
    const run = async () => {
      const { response, body } = await api('/api/auth/me')
      if (response.ok) {
        setUser(body.user)
      }
      setLoading(false)
    }
    run()
  }, [])

  const logout = async () => {
    await api('/api/auth/logout', { method: 'POST' })
    setUser(null)
  }

  if (loading) {
    return <main><p>Loading...</p></main>
  }

  return (
    <Routes>
      <Route path="/" element={<HomePage user={user} onLogout={logout} />} />
      <Route path="/login" element={<LoginPage user={user} onLogin={setUser} />} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}

function HomePage({ user, onLogout }) {
  if (user == null) {
    return (
      <main>
        <h1>Hayai</h1>
        <p><Link to="/login">Login</Link></p>
      </main>
    )
  }

  return (
    <main>
      <h1>Hayai</h1>
      <p>Signed in as {user.email}</p>
      <button type="button" onClick={onLogout}>Logout</button>
    </main>
  )
}

function LoginPage({ user, onLogin }) {
  const navigate = useNavigate()
  const [mode, setMode] = useState('login')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  if (user != null) {
    return <Navigate to="/" replace />
  }

  const submit = async (event) => {
    event.preventDefault()
    setError('')
    setNotice('')

    const endpoint = mode === 'register' ? '/api/auth/register' : '/api/auth/login'
    const { response, body } = await api(endpoint, {
      method: 'POST',
      body: JSON.stringify({ email, password }),
    })

    if (!response.ok) {
      setError(body?.error || 'request failed')
      return
    }

    if (mode === 'register') {
      setNotice('registered. please log in.')
      setMode('login')
      setPassword('')
      return
    }

    onLogin(body.user)
    setPassword('')
    navigate('/', { replace: true })
  }

  return (
    <main>
      <h1>Hayai</h1>
      <p>{mode === 'register' ? 'Create account' : 'Login'}</p>
      <form onSubmit={submit}>
        <label>
          Email
          <input
            type="email"
            value={email}
            onChange={(event) => setEmail(event.target.value)}
            required
          />
        </label>
        <label>
          Password
          <input
            type="password"
            value={password}
            onChange={(event) => setPassword(event.target.value)}
            minLength={8}
            required
          />
        </label>
        <button type="submit">{mode === 'register' ? 'Register' : 'Login'}</button>
      </form>
      <button
        type="button"
        onClick={() => {
          setMode(mode === 'register' ? 'login' : 'register')
          setError('')
          setNotice('')
        }}
      >
        {mode === 'register' ? 'Use login instead' : 'Create new account'}
      </button>
      {error !== '' && <p role="alert">{error}</p>}
      {notice !== '' && <p>{notice}</p>}
    </main>
  )
}
