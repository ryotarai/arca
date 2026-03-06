import { useMemo, useState } from 'react'
import { Link, Navigate, useSearchParams } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { completeUserSetup } from '@/lib/api'
import { messageFromError } from '@/lib/errors'
import type { User } from '@/lib/types'

type UserSetupPageProps = {
  user: User | null
}

export function UserSetupPage({ user }: UserSetupPageProps) {
  const [searchParams] = useSearchParams()
  const initialToken = useMemo(() => searchParams.get('token') ?? '', [searchParams])
  const [setupToken, setSetupToken] = useState(initialToken)
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [completed, setCompleted] = useState(false)

  if (user != null) {
    return <Navigate to="/" replace />
  }

  const submit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setError('')
    if (password.length < 8) {
      setError('password must be at least 8 characters')
      return
    }
    if (password !== confirmPassword) {
      setError('password confirmation does not match')
      return
    }

    setLoading(true)
    try {
      await completeUserSetup(setupToken.trim(), password)
      setCompleted(true)
      setPassword('')
      setConfirmPassword('')
    } catch (err) {
      setError(messageFromError(err))
    } finally {
      setLoading(false)
    }
  }

  return (
    <main className="relative flex min-h-dvh items-center justify-center overflow-hidden bg-slate-950 px-6 py-16 text-slate-100">
      <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_20%_20%,_rgba(56,189,248,0.12),_transparent_38%),radial-gradient(circle_at_80%_0%,_rgba(148,163,184,0.2),_transparent_48%)]" />
      <div className="pointer-events-none absolute inset-0 bg-[linear-gradient(rgba(255,255,255,0.04)_1px,transparent_1px),linear-gradient(90deg,rgba(255,255,255,0.04)_1px,transparent_1px)] bg-[size:48px_48px] [mask-image:radial-gradient(ellipse_at_center,black_35%,transparent_75%)]" />

      <Card className="relative z-10 w-full max-w-md border-white/15 bg-white/[0.04] py-0 shadow-2xl shadow-black/35 backdrop-blur-xl">
        <CardHeader className="space-y-2 p-6 pb-3">
          <CardTitle className="text-xl text-white">Complete account setup</CardTitle>
          <CardDescription className="text-slate-300">Use your one-time setup token to set a password.</CardDescription>
        </CardHeader>
        <CardContent className="p-6 pt-3">
          {completed ? (
            <div className="space-y-3">
              <p className="text-sm text-emerald-300">Password is set. You can now sign in.</p>
              <Button asChild className="w-full bg-white text-slate-900 hover:bg-slate-100">
                <Link to="/login">Go to login</Link>
              </Button>
            </div>
          ) : (
            <form className="space-y-4" onSubmit={submit}>
              <div className="space-y-2">
                <Label htmlFor="setup-token" className="text-slate-200">Setup token</Label>
                <Input
                  id="setup-token"
                  value={setupToken}
                  onChange={(event) => setSetupToken(event.target.value)}
                  required
                  className="h-10 border-white/20 bg-white/10 text-slate-100 placeholder:text-slate-400 focus-visible:ring-sky-400/45"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="setup-password" className="text-slate-200">Password</Label>
                <Input
                  id="setup-password"
                  type="password"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  autoComplete="new-password"
                  required
                  className="h-10 border-white/20 bg-white/10 text-slate-100 placeholder:text-slate-400 focus-visible:ring-sky-400/45"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="setup-password-confirm" className="text-slate-200">Confirm password</Label>
                <Input
                  id="setup-password-confirm"
                  type="password"
                  value={confirmPassword}
                  onChange={(event) => setConfirmPassword(event.target.value)}
                  autoComplete="new-password"
                  required
                  className="h-10 border-white/20 bg-white/10 text-slate-100 placeholder:text-slate-400 focus-visible:ring-sky-400/45"
                />
              </div>
              <Button type="submit" className="h-10 w-full bg-white text-slate-900 hover:bg-slate-100" disabled={loading}>
                {loading ? 'Saving...' : 'Set password'}
              </Button>
            </form>
          )}
          {error !== '' && <p className="mt-3 text-sm text-red-300">{error}</p>}
        </CardContent>
      </Card>
    </main>
  )
}
