import { useEffect, useState } from 'react'
import { Link, Navigate, Route, Routes, useNavigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

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
      <main className="flex min-h-dvh items-center justify-center bg-[radial-gradient(circle_at_top_left,_#f8fafc_10%,_#e2e8f0_55%,_#cbd5e1_100%)] px-6">
        <div className="w-full max-w-md rounded-2xl border border-slate-300/70 bg-white/90 p-10 shadow-xl shadow-slate-900/10 backdrop-blur">
          <p className="text-xs font-medium uppercase tracking-[0.24em] text-slate-500">Hayai</p>
          <h1 className="mt-2 text-3xl font-semibold text-slate-900">Welcome back</h1>
          <p className="mt-3 text-sm text-slate-600">Sign in to access your workspace.</p>
          <Button asChild className="mt-8 w-full">
            <Link to="/login">Login</Link>
          </Button>
        </div>
      </main>
    )
  }

  return (
    <main className="flex min-h-dvh items-center justify-center bg-slate-950 px-6 py-16">
      <div className="w-full max-w-lg rounded-2xl border border-white/10 bg-white/[0.03] p-8 text-slate-100 shadow-2xl shadow-black/40 backdrop-blur">
        <p className="text-xs font-medium uppercase tracking-[0.24em] text-slate-400">Hayai</p>
        <h1 className="mt-2 text-2xl font-semibold">Dashboard</h1>
        <p className="mt-3 text-sm text-slate-300">Signed in as {user.email}</p>
        <Button type="button" variant="secondary" className="mt-6" onClick={onLogout}>
          Logout
        </Button>
      </div>
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
    <main className="relative flex min-h-dvh items-center justify-center overflow-hidden bg-slate-950 px-6 py-16 text-slate-100">
      <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_20%_20%,_rgba(56,189,248,0.12),_transparent_38%),radial-gradient(circle_at_80%_0%,_rgba(148,163,184,0.2),_transparent_48%)]" />
      <div className="pointer-events-none absolute inset-0 bg-[linear-gradient(rgba(255,255,255,0.04)_1px,transparent_1px),linear-gradient(90deg,rgba(255,255,255,0.04)_1px,transparent_1px)] bg-[size:48px_48px] [mask-image:radial-gradient(ellipse_at_center,black_35%,transparent_75%)]" />

      <section className="relative z-10 flex w-full max-w-4xl flex-col items-start gap-8 md:flex-row md:items-center md:justify-between">
        <div className="max-w-lg space-y-5 animate-in fade-in slide-in-from-left-3 duration-700">
          <h2 className="text-xs font-medium uppercase tracking-[0.28em] text-slate-400">Hayai</h2>
          <h1 className="text-balance text-4xl font-semibold leading-tight text-white sm:text-5xl">
            {mode === 'register' ? 'Build fast with secure auth' : 'Ship faster with confidence'}
          </h1>
          <p className="max-w-md text-pretty text-sm leading-6 text-slate-300 sm:text-base">
            A clean workspace for developers who care about speed, clarity, and reliable collaboration.
          </p>
        </div>

        <Card className="w-full max-w-md border-white/15 bg-white/[0.04] py-0 shadow-2xl shadow-black/35 backdrop-blur-xl animate-in fade-in zoom-in-95 duration-500">
          <CardHeader className="space-y-3 p-8 pb-4">
            <CardTitle className="text-2xl font-semibold text-white">
              {mode === 'register' ? 'Create account' : 'Login'}
            </CardTitle>
            <CardDescription className="text-slate-300">
              {mode === 'register'
                ? 'Use your work email and a secure password.'
                : 'Use your registered email and password.'}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-6 p-8 pt-2">
            <form className="space-y-4" onSubmit={submit}>
              <div className="space-y-2">
                <Label htmlFor="email" className="text-slate-200">Email</Label>
                <Input
                  id="email"
                  type="email"
                  autoComplete="email"
                  value={email}
                  onChange={(event) => setEmail(event.target.value)}
                  required
                  className="h-10 border-white/20 bg-white/10 text-slate-100 placeholder:text-slate-400 focus-visible:ring-sky-400/45"
                  placeholder="you@company.dev"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="password" className="text-slate-200">Password</Label>
                <Input
                  id="password"
                  type="password"
                  autoComplete={mode === 'register' ? 'new-password' : 'current-password'}
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  minLength={8}
                  required
                  className="h-10 border-white/20 bg-white/10 text-slate-100 placeholder:text-slate-400 focus-visible:ring-sky-400/45"
                  placeholder="minimum 8 characters"
                />
              </div>
              <Button type="submit" className="h-10 w-full bg-white text-slate-900 hover:bg-slate-100">
                {mode === 'register' ? 'Register' : 'Login'}
              </Button>
            </form>

            <Button
              type="button"
              variant="ghost"
              className="h-10 w-full text-slate-200 hover:bg-white/10 hover:text-white"
              onClick={() => {
                setMode(mode === 'register' ? 'login' : 'register')
                setError('')
                setNotice('')
              }}
            >
              {mode === 'register' ? 'Use login instead' : 'Create new account'}
            </Button>

            {error !== '' && (
              <p role="alert" className="rounded-md border border-red-400/30 bg-red-500/12 px-3 py-2 text-sm text-red-200">
                {error}
              </p>
            )}
            {notice !== '' && (
              <p className="rounded-md border border-emerald-400/30 bg-emerald-500/12 px-3 py-2 text-sm text-emerald-200">
                {notice}
              </p>
            )}
          </CardContent>
        </Card>
      </section>
    </main>
  )
}
