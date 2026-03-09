import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  setupComplete,
  setupCreateAdmin,
  setupValidateCloudflare,
} from '@/lib/api'
import {
  normalizeBaseDomainInput,
  normalizeDomainPrefixInput,
  validateBaseDomainInput,
  validateDomainPrefixInput,
} from '@/lib/domainValidation'
import { messageFromError } from '@/lib/errors'
import type { ServerExposureMethod, User } from '@/lib/types'

type SetupPageProps = {
  hasAdmin: boolean
  initialCloudflareZoneID: string
  onAdminReady: (user: User) => void
  onSetupComplete: (
    zoneID: string,
    baseDomain: string,
    domainPrefix: string,
  ) => void
}

export function SetupPage({
  hasAdmin,
  initialCloudflareZoneID,
  onAdminReady,
  onSetupComplete,
}: SetupPageProps) {
  const navigate = useNavigate()
  const [step, setStep] = useState(hasAdmin ? 2 : 1)
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [serverExposureMethod, setServerExposureMethod] = useState<ServerExposureMethod>('cloudflare_tunnel')
  const [serverDomain, setServerDomain] = useState('')
  const [baseDomain, setBaseDomain] = useState('')
  const [domainPrefix, setDomainPrefix] = useState('')
  const [cloudflareAccountID, setCloudflareAccountID] = useState('')
  const [cloudflareToken, setCloudflareToken] = useState('')
  const [cloudflareZoneID, setCloudflareZoneID] = useState(initialCloudflareZoneID)
  const [loadingStep, setLoadingStep] = useState(false)
  const [error, setError] = useState('')

  const progress = useMemo(() => {
    if (step <= 1) {
      return 50
    }
    return 100
  }, [step])

  const baseDomainError = useMemo(() => validateBaseDomainInput(baseDomain), [baseDomain])
  const domainPrefixError = useMemo(() => validateDomainPrefixInput(domainPrefix), [domainPrefix])

  const consoleEndpoint = useMemo(() => {
    if (baseDomainError != null || domainPrefixError != null) {
      return ''
    }
    const normalizedDomain = normalizeBaseDomainInput(baseDomain)
    if (normalizedDomain === '') {
      return ''
    }
    const normalizedPrefix = normalizeDomainPrefixInput(domainPrefix)
    const label = `${normalizedPrefix}app`.replace(/^-+|-+$/g, '') || 'app'
    return `https://${label}.${normalizedDomain}`
  }, [baseDomain, baseDomainError, domainPrefix, domainPrefixError])

  const machineEndpointPreview = useMemo(() => {
    const normalizedDomain = normalizeBaseDomainInput(baseDomain)
    const base = normalizedDomain === '' ? 'your-domain.example.com' : normalizedDomain
    const normalizedPrefix = normalizeDomainPrefixInput(domainPrefix)
    const prefix = normalizedPrefix === '' ? 'your-prefix-' : normalizedPrefix
    return `${prefix}machine-name.${base}`
  }, [baseDomain, domainPrefix])

  const submitAdmin = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (password !== confirmPassword) {
      setError('password confirmation does not match')
      return
    }

    setError('')
    setLoadingStep(true)
    try {
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
      const normalizedBaseDomain = normalizeBaseDomainInput(baseDomain)
      const normalizedDomainPrefix = normalizeDomainPrefixInput(domainPrefix)
      const nextBaseDomainError = validateBaseDomainInput(normalizedBaseDomain)
      if (nextBaseDomainError != null) {
        throw new Error(nextBaseDomainError)
      }
      const nextDomainPrefixError = validateDomainPrefixInput(normalizedDomainPrefix)
      if (nextDomainPrefixError != null) {
        throw new Error(nextDomainPrefixError)
      }

      if (serverExposureMethod === 'cloudflare_tunnel') {
        if (cloudflareZoneID.trim() === '') {
          throw new Error('cloudflare zone id is required')
        }
        if (cloudflareAccountID.trim() === '') {
          throw new Error('cloudflare account id is required')
        }
        await setupValidateCloudflare(cloudflareToken, cloudflareAccountID, normalizedBaseDomain)
      } else {
        if (serverDomain.trim() === '') {
          throw new Error('server domain is required for manual exposure')
        }
      }

      await setupComplete(
        email,
        password,
        normalizedBaseDomain,
        normalizedDomainPrefix,
        cloudflareToken,
        cloudflareZoneID,
        serverExposureMethod,
        serverDomain.trim(),
      )
      onSetupComplete(cloudflareZoneID, normalizedBaseDomain, normalizedDomainPrefix)
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
          <p className="mt-1 text-sm text-slate-300">Admin account and Cloudflare network settings.</p>
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
                Choose how the Arca console is exposed and configure domain settings.
              </CardDescription>
            </CardHeader>
            <CardContent className="p-6 pt-3">
              <form className="space-y-4" onSubmit={submitServerConfig}>
                <div className="space-y-2">
                  <Label htmlFor="setup-server-exposure-method" className="text-slate-200">
                    Server exposure method
                  </Label>
                  <select
                    id="setup-server-exposure-method"
                    value={serverExposureMethod}
                    onChange={(event) => setServerExposureMethod(event.target.value as ServerExposureMethod)}
                    className="h-10 w-full rounded-md border border-white/20 bg-white/10 px-3 text-sm text-slate-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/45"
                  >
                    <option value="cloudflare_tunnel">Cloudflare Tunnel</option>
                    <option value="manual">Manual (own domain / reverse proxy)</option>
                  </select>
                  <p className="text-xs text-slate-400">
                    {serverExposureMethod === 'cloudflare_tunnel'
                      ? 'A Cloudflare Tunnel will be created automatically to expose the console.'
                      : 'You manage DNS and TLS yourself. Provide the domain where the console is reachable.'}
                  </p>
                </div>

                {serverExposureMethod === 'manual' && (
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
                )}

                <div className="space-y-2">
                  <Label htmlFor="setup-base-domain" className="text-slate-200">
                    Base domain
                  </Label>
                  <Input
                    id="setup-base-domain"
                    value={baseDomain}
                    onChange={(event) => setBaseDomain(event.target.value)}
                    required
                    className="h-10 border-white/20 bg-white/10 text-slate-100 placeholder:text-slate-400 focus-visible:ring-sky-400/45"
                    placeholder="example.com"
                  />
                  {baseDomain !== '' && baseDomainError != null && <p className="text-sm text-red-300">{baseDomainError}</p>}
                </div>
                <div className="space-y-2">
                  <Label htmlFor="setup-domain-prefix" className="text-slate-200">
                    Domain prefix
                  </Label>
                  <Input
                    id="setup-domain-prefix"
                    value={domainPrefix}
                    onChange={(event) => setDomainPrefix(event.target.value)}
                    className="h-10 border-white/20 bg-white/10 text-slate-100 placeholder:text-slate-400 focus-visible:ring-sky-400/45"
                    placeholder="arca-"
                  />
                  {domainPrefix !== '' && domainPrefixError != null && <p className="text-sm text-red-300">{domainPrefixError}</p>}
                  <p className="text-xs text-slate-400">Machine endpoint preview: {machineEndpointPreview}</p>
                </div>

                {serverExposureMethod === 'cloudflare_tunnel' && (
                  <div className="space-y-4 rounded-lg border border-white/10 bg-white/[0.02] p-4">
                    <p className="text-sm font-medium text-slate-200">Cloudflare credentials (server)</p>
                    <div className="space-y-2">
                      <Label htmlFor="setup-account-id" className="text-slate-200">
                        Cloudflare account ID
                      </Label>
                      <Input
                        id="setup-account-id"
                        value={cloudflareAccountID}
                        onChange={(event) => setCloudflareAccountID(event.target.value)}
                        required
                        className="h-10 border-white/20 bg-white/10 text-slate-100 placeholder:text-slate-400 focus-visible:ring-sky-400/45"
                        placeholder="account id for your Cloudflare account"
                      />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="setup-zone-id" className="text-slate-200">
                        Cloudflare zone ID
                      </Label>
                      <Input
                        id="setup-zone-id"
                        value={cloudflareZoneID}
                        onChange={(event) => setCloudflareZoneID(event.target.value)}
                        required
                        className="h-10 border-white/20 bg-white/10 text-slate-100 placeholder:text-slate-400 focus-visible:ring-sky-400/45"
                        placeholder="zone id for your base domain"
                      />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="setup-cloudflare-token" className="text-slate-200">
                        Cloudflare API token
                      </Label>
                      <Input
                        id="setup-cloudflare-token"
                        type="password"
                        value={cloudflareToken}
                        onChange={(event) => setCloudflareToken(event.target.value)}
                        required
                        className="h-10 border-white/20 bg-white/10 text-slate-100 placeholder:text-slate-400 focus-visible:ring-sky-400/45"
                        placeholder="token with DNS and tunnel permissions"
                      />
                    </div>
                  </div>
                )}

                {serverExposureMethod === 'cloudflare_tunnel' && consoleEndpoint !== '' && (
                  <div className="rounded-lg border border-sky-400/25 bg-sky-500/10 p-4">
                    <p className="text-sm text-slate-200">Console endpoint after setup</p>
                    <p className="mt-1 break-all text-sm font-medium text-sky-200">{consoleEndpoint}</p>
                  </div>
                )}
                <Button
                  type="submit"
                  disabled={loadingStep || baseDomainError != null || domainPrefixError != null}
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
