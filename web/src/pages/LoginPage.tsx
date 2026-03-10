import { useEffect, useState } from 'react'
import { Navigate, useNavigate, useSearchParams } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { login, me, startOidcLogin } from '@/lib/api'
import { messageFromError } from '@/lib/errors'
import type { User } from '@/lib/types'

type LoginPageProps = {
  user: User | null
  onLogin: (user: User) => void
  oidcEnabled: boolean
  passwordLoginDisabled: boolean
  iapEnabled: boolean
}

export function LoginPage({ user, onLogin, oidcEnabled, passwordLoginDisabled, iapEnabled }: LoginPageProps) {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const nextPath = sanitizeNextPath(searchParams.get('next'))
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [oidcLoading, setOidcLoading] = useState(false)
  const [iapLoading, setIapLoading] = useState(iapEnabled)

  useEffect(() => {
    if (!iapEnabled) return
    let cancelled = false
    const tryIAP = async () => {
      try {
        const iapUser = await me()
        if (!cancelled && iapUser != null) {
          onLogin(iapUser)
        }
      } catch {
        // IAP auth failed, fall through to other methods
      } finally {
        if (!cancelled) setIapLoading(false)
      }
    }
    void tryIAP()
    return () => { cancelled = true }
  }, [iapEnabled, onLogin])

  useEffect(() => {
    if (user != null && nextPath !== '/') {
      window.location.replace(nextPath)
    }
  }, [nextPath, user])

  if (user != null) {
    if (nextPath !== '/') {
      return null
    }
    return <Navigate to="/" replace />
  }

  const showPasswordForm = !passwordLoginDisabled
  const showOidc = oidcEnabled
  const iapOnly = iapEnabled && !showPasswordForm && !showOidc

  if (iapLoading && iapOnly) {
    return (
      <main className="flex min-h-dvh items-center justify-center bg-slate-950 text-slate-100">
        <p className="text-sm text-slate-300">Authenticating...</p>
      </main>
    )
  }

  const submit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setError('')

    try {
      const loggedIn = await login(email, password)
      if (loggedIn == null) {
        setError('request failed')
        return
      }

      onLogin(loggedIn)
      setPassword('')
      if (nextPath !== '/') {
        window.location.assign(nextPath)
        return
      }
      void navigate('/', { replace: true })
    } catch (e) {
      setError(messageFromError(e))
    }
  }

  const startGoogleLogin = async () => {
    setError('')
    setOidcLoading(true)
    try {
      const redirectUri = `${window.location.origin}/login/oidc/callback`
      const authorizationURL = await startOidcLogin(redirectUri)
      if (authorizationURL.trim() === '') {
        throw new Error('failed to prepare oidc login')
      }
      window.location.assign(authorizationURL)
    } catch (e) {
      setError(messageFromError(e))
      setOidcLoading(false)
    }
  }

  return (
    <main className="relative flex min-h-dvh items-center justify-center overflow-hidden bg-slate-950 px-6 py-16 text-slate-100">
      <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_20%_20%,_rgba(56,189,248,0.12),_transparent_38%),radial-gradient(circle_at_80%_0%,_rgba(148,163,184,0.2),_transparent_48%)]" />
      <div className="pointer-events-none absolute inset-0 bg-[linear-gradient(rgba(255,255,255,0.04)_1px,transparent_1px),linear-gradient(90deg,rgba(255,255,255,0.04)_1px,transparent_1px)] bg-[size:48px_48px] [mask-image:radial-gradient(ellipse_at_center,black_35%,transparent_75%)]" />

      <section className="relative z-10 flex w-full max-w-4xl flex-col items-start gap-8 md:flex-row md:items-center md:justify-between">
        <div className="max-w-lg space-y-5 animate-in fade-in slide-in-from-left-3 duration-700">
          <h2 className="text-xs font-medium uppercase tracking-[0.28em] text-slate-400">Arca</h2>
          <h1 className="text-balance text-4xl font-semibold leading-tight text-white sm:text-5xl">
            Ship faster with confidence
          </h1>
          <p className="max-w-md text-pretty text-sm leading-6 text-slate-300 sm:text-base">
            A clean workspace for developers who care about speed, clarity, and reliable collaboration.
          </p>
        </div>

        <Card className="w-full max-w-md border-white/15 bg-white/[0.04] py-0 shadow-2xl shadow-black/35 backdrop-blur-xl animate-in fade-in zoom-in-95 duration-500">
          <CardHeader className="space-y-3 p-8 pb-4">
            <CardTitle className="text-2xl font-semibold text-white">Login</CardTitle>
            <CardDescription className="text-slate-300">
              {passwordLoginDisabled ? 'Sign in with your organization account.' : 'Use your provisioned email and password.'}
            </CardDescription>
          </CardHeader>
          <CardContent className="space-y-6 p-8 pt-2">
            {iapLoading && !iapOnly && (
              <p className="text-center text-sm text-slate-300">Authenticating via IAP...</p>
            )}
            {!passwordLoginDisabled && (
              <form className="space-y-4" onSubmit={submit}>
                <div className="space-y-2">
                  <Label htmlFor="email" className="text-slate-200">
                    Email
                  </Label>
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
                  <Label htmlFor="password" className="text-slate-200">
                    Password
                  </Label>
                  <Input
                    id="password"
                    type="password"
                    autoComplete="current-password"
                    value={password}
                    onChange={(event) => setPassword(event.target.value)}
                    minLength={8}
                    required
                    className="h-10 border-white/20 bg-white/10 text-slate-100 placeholder:text-slate-400 focus-visible:ring-sky-400/45"
                    placeholder="minimum 8 characters"
                  />
                </div>
                <Button type="submit" className="h-10 w-full bg-white text-slate-900 hover:bg-slate-100">
                  Login
                </Button>
              </form>
            )}
            {oidcEnabled && (
              <Button
                type="button"
                variant="secondary"
                className="h-10 w-full"
                disabled={oidcLoading}
                onClick={startGoogleLogin}
              >
                {oidcLoading ? 'Redirecting...' : 'Continue with Google'}
              </Button>
            )}

            {error !== '' && (
              <p role="alert" className="rounded-md border border-red-400/30 bg-red-500/12 px-3 py-2 text-sm text-red-200">
                {error}
              </p>
            )}
          </CardContent>
        </Card>
      </section>
    </main>
  )
}

function sanitizeNextPath(next: string | null): string {
  if (next == null || next === '') {
    return '/'
  }
  try {
    const parsed = new URL(next, window.location.origin)
    if (parsed.origin !== window.location.origin) {
      return '/'
    }
    if (!parsed.pathname.startsWith('/')) {
      return '/'
    }
    return `${parsed.pathname}${parsed.search}${parsed.hash}`
  } catch {
    return '/'
  }
}
