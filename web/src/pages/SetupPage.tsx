import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  setupComplete,
  setupCreateAdmin,
  verifySetupPassword,
} from '@/lib/api'
import { messageFromError } from '@/lib/errors'
import type { User } from '@/lib/types'

type SetupPageProps = {
  hasAdmin: boolean
  onAdminReady: (user: User) => void
  onSetupComplete: () => void
}

export function SetupPage({
  hasAdmin,
  onAdminReady,
  onSetupComplete,
}: SetupPageProps) {
  const navigate = useNavigate()
  const [step, setStep] = useState(hasAdmin ? 2 : 1)
  const [setupPassword, setSetupPassword] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [serverDomain, setServerDomain] = useState('')
  const [loadingStep, setLoadingStep] = useState(false)
  const [error, setError] = useState('')

  const progress = useMemo(() => {
    if (step <= 1) {
      return 50
    }
    return 100
  }, [step])

  const submitAdmin = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (password !== confirmPassword) {
      setError('password confirmation does not match')
      return
    }

    setError('')
    setLoadingStep(true)
    try {
      const valid = await verifySetupPassword(setupPassword)
      if (!valid) {
        setError('Invalid setup password')
        return
      }
      const user = await setupCreateAdmin(email, password)
      if (user != null) {
        onAdminReady(user)
      }
      setStep(2)
    } catch (e) {
      setError(messageFromError(e))
    } finally {
      setLoadingStep(false)
    }
  }

  const submitServerConfig = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setError('')
    setLoadingStep(true)
    try {
      if (serverDomain.trim() === '') {
        throw new Error('server domain is required')
      }

      await setupComplete(
        email,
        password,
        serverDomain.trim(),
        setupPassword,
      )
      onSetupComplete()
      window.setTimeout(() => {
        void navigate('/', { replace: true })
      }, 350)
    } catch (e) {
      setError(messageFromError(e))
    } finally {
      setLoadingStep(false)
    }
  }

  return (
    <main className="relative min-h-dvh overflow-hidden bg-slate-950 px-6 py-16 text-slate-100">
      <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_20%_20%,_rgba(56,189,248,0.12),_transparent_38%),radial-gradient(circle_at_80%_0%,_rgba(148,163,184,0.2),_transparent_48%)]" />
      <div className="pointer-events-none absolute inset-0 bg-[linear-gradient(rgba(255,255,255,0.04)_1px,transparent_1px),linear-gradient(90deg,rgba(255,255,255,0.04)_1px,transparent_1px)] bg-[size:48px_48px] [mask-image:radial-gradient(ellipse_at_center,black_35%,transparent_75%)]" />

      <section className="relative z-10 mx-auto w-full max-w-3xl space-y-6">
        <header className="rounded-2xl border border-white/10 bg-white/[0.03] p-6 backdrop-blur">
          <p className="text-xs font-medium uppercase tracking-[0.24em] text-slate-400">Arca setup</p>
          <h1 className="mt-2 text-2xl font-semibold text-white">Complete initial configuration</h1>
          <p className="mt-1 text-sm text-slate-300">Admin account and server exposure settings.</p>
          <div className="mt-4 h-2 w-full overflow-hidden rounded-full bg-white/10">
            <div className="h-full rounded-full bg-sky-300 transition-all" style={{ width: `${progress}%` }} />
          </div>
          <p className="mt-2 text-xs text-slate-400">Step {Math.min(step, 2)} of 2</p>
        </header>

        {step === 1 && (
          <Card className="border-white/15 bg-white/[0.04] py-0 shadow-2xl shadow-black/35 backdrop-blur-xl">
            <CardHeader className="space-y-2 p-6 pb-3">
              <CardTitle className="text-xl text-white">1. Create admin account</CardTitle>
              <CardDescription className="text-slate-300">This account will manage your Arca control plane.</CardDescription>
            </CardHeader>
            <CardContent className="p-6 pt-3">
              <form className="space-y-4" onSubmit={submitAdmin}>
                <div className="space-y-2">
                  <Label htmlFor="setup-setup-password" className="text-slate-200">
                    Setup password
                  </Label>
                  <Input
                    id="setup-setup-password"
                    type="password"
                    value={setupPassword}
                    onChange={(event) => setSetupPassword(event.target.value)}
                    required
                    className="h-10 border-white/20 bg-white/10 text-slate-100 placeholder:text-slate-400 focus-visible:ring-sky-400/45"
                    placeholder="shown in the server console output"
                  />
                  <p className="text-xs text-slate-400">Enter the setup password displayed in the server&apos;s standard output.</p>
                </div>
                <div className="space-y-2">
                  <Label htmlFor="setup-email" className="text-slate-200">
                    Email
                  </Label>
                  <Input
                    id="setup-email"
                    type="email"
                    value={email}
                    onChange={(event) => setEmail(event.target.value)}
                    required
                    className="h-10 border-white/20 bg-white/10 text-slate-100 placeholder:text-slate-400 focus-visible:ring-sky-400/45"
                    placeholder="you@company.dev"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="setup-password" className="text-slate-200">
                    Password
                  </Label>
                  <Input
                    id="setup-password"
                    type="password"
                    value={password}
                    onChange={(event) => setPassword(event.target.value)}
                    required
                    className="h-10 border-white/20 bg-white/10 text-slate-100 placeholder:text-slate-400 focus-visible:ring-sky-400/45"
                    placeholder="minimum 8 characters"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="setup-password-confirm" className="text-slate-200">
                    Confirm password
                  </Label>
                  <Input
                    id="setup-password-confirm"
                    type="password"
                    value={confirmPassword}
                    onChange={(event) => setConfirmPassword(event.target.value)}
                    required
                    className="h-10 border-white/20 bg-white/10 text-slate-100 placeholder:text-slate-400 focus-visible:ring-sky-400/45"
                  />
                </div>
                <Button type="submit" disabled={loadingStep} className="h-10 w-full bg-white text-slate-900 hover:bg-slate-100">
                  {loadingStep ? 'Saving...' : 'Continue'}
                </Button>
              </form>
            </CardContent>
          </Card>
        )}

        {step >= 2 && (
          <Card className="border-white/15 bg-white/[0.04] py-0 shadow-2xl shadow-black/35 backdrop-blur-xl">
            <CardHeader className="space-y-2 p-6 pb-3">
              <CardTitle className="text-xl text-white">2. Configure server exposure</CardTitle>
              <CardDescription className="text-slate-300">
                Provide the domain where the Arca console is reachable.
              </CardDescription>
            </CardHeader>
            <CardContent className="p-6 pt-3">
              <form className="space-y-4" onSubmit={submitServerConfig}>
                <div className="space-y-2">
                  <Label htmlFor="setup-server-domain" className="text-slate-200">
                    Server domain
                  </Label>
                  <Input
                    id="setup-server-domain"
                    value={serverDomain}
                    onChange={(event) => setServerDomain(event.target.value)}
                    required
                    className="h-10 border-white/20 bg-white/10 text-slate-100 placeholder:text-slate-400 focus-visible:ring-sky-400/45"
                    placeholder="arca.example.com"
                  />
                  <p className="text-xs text-slate-400">The domain where machines will reach this server.</p>
                </div>

                <Button
                  type="submit"
                  disabled={loadingStep}
                  className="h-10 w-full bg-white text-slate-900 hover:bg-slate-100"
                >
                  {loadingStep ? 'Completing setup...' : 'Finish setup'}
                </Button>
              </form>
            </CardContent>
          </Card>
        )}

        {error !== '' && (
          <p role="alert" className="rounded-md border border-red-400/30 bg-red-500/12 px-3 py-2 text-sm text-red-200">
            {error}
          </p>
        )}
      </section>
    </main>
  )
}
