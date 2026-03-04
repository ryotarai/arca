import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  setupComplete,
  setupConfigureProviderDocker,
  setupCreateAdmin,
  setupValidateCloudflare,
} from '@/lib/api'
import { messageFromError } from '@/lib/errors'
import type { User } from '@/lib/types'

type SetupPageProps = {
  hasAdmin: boolean
  initialCloudflareZoneID: string
  initialMachineRuntime: 'docker' | 'libvirt'
  onAdminReady: (user: User) => void
  onSetupComplete: (
    zoneID: string,
    baseDomain: string,
    domainPrefix: string,
    machineRuntime: 'docker' | 'libvirt',
  ) => void
}

export function SetupPage({
  hasAdmin,
  initialCloudflareZoneID,
  initialMachineRuntime,
  onAdminReady,
  onSetupComplete,
}: SetupPageProps) {
  const navigate = useNavigate()
  const [step, setStep] = useState(hasAdmin ? 2 : 1)
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [baseDomain, setBaseDomain] = useState('')
  const [domainPrefix, setDomainPrefix] = useState('')
  const [cloudflareAccountID, setCloudflareAccountID] = useState('')
  const [cloudflareToken, setCloudflareToken] = useState('')
  const [cloudflareZoneID, setCloudflareZoneID] = useState(initialCloudflareZoneID)
  const [machineRuntime, setMachineRuntime] = useState<'docker' | 'libvirt'>(initialMachineRuntime)
  const [loadingStep, setLoadingStep] = useState(false)
  const [error, setError] = useState('')

  const progress = useMemo(() => {
    if (step <= 1) {
      return 33
    }
    if (step === 2) {
      return 66
    }
    return 100
  }, [step])

  const consoleEndpoint = useMemo(() => {
    const normalizedDomain = baseDomain.trim().toLowerCase()
    if (normalizedDomain === '') {
      return ''
    }
    const normalizedPrefix = domainPrefix
      .trim()
      .toLowerCase()
      .replace(/[^a-z0-9-]/g, '')
    const label = `${normalizedPrefix}app`.replace(/^-+|-+$/g, '') || 'app'
    return `https://${label}.${normalizedDomain}`
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

  const submitCloudflare = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setError('')
    setLoadingStep(true)
    try {
      if (cloudflareZoneID.trim() === '') {
        throw new Error('cloudflare zone id is required')
      }
      if (cloudflareAccountID.trim() === '') {
        throw new Error('cloudflare account id is required')
      }
      await setupValidateCloudflare(cloudflareToken, cloudflareAccountID, baseDomain)
      setStep(3)
    } catch (e) {
      setError(messageFromError(e))
    } finally {
      setLoadingStep(false)
    }
  }

  const submitProvider = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setError('')
    setLoadingStep(true)
    try {
      await setupConfigureProviderDocker()
      await setupComplete(email, password, baseDomain, domainPrefix, cloudflareToken, cloudflareZoneID, machineRuntime)
      onSetupComplete(cloudflareZoneID, baseDomain, domainPrefix, machineRuntime)
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
          <p className="mt-1 text-sm text-slate-300">Admin account, Cloudflare network settings, and Docker provider.</p>
          <div className="mt-4 h-2 w-full overflow-hidden rounded-full bg-white/10">
            <div className="h-full rounded-full bg-sky-300 transition-all" style={{ width: `${progress}%` }} />
          </div>
          <p className="mt-2 text-xs text-slate-400">Step {Math.min(step, 3)} of 3</p>
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
                    minLength={8}
                    required
                    className="h-10 border-white/20 bg-white/10 text-slate-100 placeholder:text-slate-400 focus-visible:ring-sky-400/45"
                    placeholder="minimum 8 characters"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="setup-confirm-password" className="text-slate-200">
                    Confirm password
                  </Label>
                  <Input
                    id="setup-confirm-password"
                    type="password"
                    value={confirmPassword}
                    onChange={(event) => setConfirmPassword(event.target.value)}
                    minLength={8}
                    required
                    className="h-10 border-white/20 bg-white/10 text-slate-100 placeholder:text-slate-400 focus-visible:ring-sky-400/45"
                    placeholder="re-enter password"
                  />
                </div>
                <Button
                  type="submit"
                  disabled={loadingStep}
                  className="h-10 w-full bg-white text-slate-900 hover:bg-slate-100"
                >
                  {loadingStep ? 'Saving...' : 'Continue'}
                </Button>
              </form>
            </CardContent>
          </Card>
        )}

        {step === 2 && (
          <Card className="border-white/15 bg-white/[0.04] py-0 shadow-2xl shadow-black/35 backdrop-blur-xl">
            <CardHeader className="space-y-2 p-6 pb-3">
              <CardTitle className="text-xl text-white">2. Configure Cloudflare exposure</CardTitle>
              <CardDescription className="text-slate-300">Validate token and set the base domain for exposure endpoints.</CardDescription>
            </CardHeader>
            <CardContent className="p-6 pt-3">
              <div className="mb-4 rounded-lg border border-white/10 bg-black/20 p-4 text-sm text-slate-300">
                <p className="font-medium text-slate-100">Required API token permissions</p>
                <ul className="mt-2 list-disc space-y-1 pl-5">
                  <li>`Account` - `Cloudflare Tunnel: Edit`</li>
                  <li>`Zone` - `DNS: Edit` (for the selected zone)</li>
                </ul>
                <p className="mt-3 font-medium text-slate-100">How to create the token</p>
                <ol className="mt-2 list-decimal space-y-1 pl-5">
                  <li>Open Cloudflare Dashboard and select your account</li>
                  <li>`Manage Account` - `API Tokens` - `Create Token`</li>
                  <li>Create an account token and add the permissions above</li>
                  <li>Set account and zone resources for this environment, then create and copy the token value</li>
                </ol>
              </div>
              <form className="space-y-4" onSubmit={submitCloudflare}>
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
                    placeholder="arca.dev"
                  />
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
                    placeholder="arca- (optional)"
                  />
                </div>
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
                <Button
                  type="submit"
                  disabled={loadingStep}
                  className="h-10 w-full bg-white text-slate-900 hover:bg-slate-100"
                >
                  {loadingStep ? 'Validating...' : 'Validate and continue'}
                </Button>
              </form>
            </CardContent>
          </Card>
        )}

        {step >= 3 && (
          <Card className="border-white/15 bg-white/[0.04] py-0 shadow-2xl shadow-black/35 backdrop-blur-xl">
            <CardHeader className="space-y-2 p-6 pb-3">
              <CardTitle className="text-xl text-white">3. Setup provider</CardTitle>
              <CardDescription className="text-slate-300">
                Choose the runtime used for newly created machines. Existing machines keep their assigned runtime.
              </CardDescription>
            </CardHeader>
            <CardContent className="p-6 pt-3">
              <form className="space-y-4" onSubmit={submitProvider}>
                <div className="rounded-lg border border-white/10 bg-white/[0.03] p-4">
                  <p className="text-sm text-slate-300">Provider</p>
                  <div className="mt-2 space-y-2">
                    <label className="flex items-center gap-2 text-sm text-slate-200">
                      <input
                        type="radio"
                        name="setup-machine-runtime"
                        value="docker"
                        checked={machineRuntime === 'docker'}
                        onChange={() => setMachineRuntime('docker')}
                      />
                      <span>Docker</span>
                    </label>
                    <label className="flex items-center gap-2 text-sm text-slate-200">
                      <input
                        type="radio"
                        name="setup-machine-runtime"
                        value="libvirt"
                        checked={machineRuntime === 'libvirt'}
                        onChange={() => setMachineRuntime('libvirt')}
                      />
                      <span>Libvirt (Ubuntu 24.04 VM)</span>
                    </label>
                  </div>
                  <p className="mt-2 text-xs text-slate-400">Expose endpoints in private mode by default.</p>
                </div>
                {consoleEndpoint !== '' && (
                  <div className="rounded-lg border border-sky-400/25 bg-sky-500/10 p-4">
                    <p className="text-sm text-slate-200">Console endpoint after setup</p>
                    <p className="mt-1 break-all text-sm font-medium text-sky-200">{consoleEndpoint}</p>
                  </div>
                )}
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
