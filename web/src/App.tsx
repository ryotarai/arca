import { create } from '@bufbuild/protobuf'
import { Code, ConnectError, createClient } from '@connectrpc/connect'
import { createConnectTransport } from '@connectrpc/connect-web'
import { useEffect, useMemo, useState } from 'react'
import { Link, Navigate, Route, Routes, useNavigate, useParams } from 'react-router-dom'
import {
  AuthService,
  LoginRequestSchema,
  LogoutRequestSchema,
  MeRequestSchema,
  RegisterRequestSchema,
} from '@/gen/arca/v1/auth_pb'
import {
  CreateMachineRequestSchema,
  DeleteMachineRequestSchema,
  GetMachineRequestSchema,
  ListMachinesRequestSchema,
  Machine as MachineMessage,
  MachineService,
  StartMachineRequestSchema,
  StopMachineRequestSchema,
  UpdateMachineRequestSchema,
} from '@/gen/arca/v1/machine_pb'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

type User = {
  id: string
  email: string
}

type Machine = MachineMessage

type SetupStatus = {
  isConfigured: boolean
  hasAdmin: boolean
}

type Exposure = {
  id: string
  subdomain: string
  hostname: string
  localPort: number
  isPublic: boolean
}

type ApiErrorPayload = {
  code?: string
  message?: string
}

class ApiError extends Error {
  status: number
  code: string

  constructor(message: string, status: number, code = '') {
    super(message)
    this.status = status
    this.code = code
  }
}

const connectTransport = createConnectTransport({
  baseUrl: window.location.origin,
  fetch: (input, init) => fetch(input, { ...init, credentials: 'include' }),
})

const authClient = createClient(AuthService, connectTransport)
const machineClient = createClient(MachineService, connectTransport)

function toUser(user: { id: string; email: string } | undefined): User | null {
  if (user == null) {
    return null
  }
  return {
    id: user.id,
    email: user.email,
  }
}

function normalizeProcedurePath(path: string): string {
  if (path.startsWith('/')) {
    return path
  }
  return `/${path}`
}

async function connectJSON<Response>(procedurePath: string, body: Record<string, unknown>): Promise<Response> {
  const response = await fetch(normalizeProcedurePath(procedurePath), {
    method: 'POST',
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify(body),
  })

  if (!response.ok) {
    let payload: ApiErrorPayload | null = null
    try {
      payload = (await response.json()) as ApiErrorPayload
    } catch {
      payload = null
    }

    throw new ApiError(
      payload?.message ?? response.statusText ?? 'request failed',
      response.status,
      payload?.code ?? '',
    )
  }

  const contentType = response.headers.get('Content-Type') ?? ''
  if (!contentType.toLowerCase().includes('json')) {
    throw new ApiError('procedure not available', 404, 'unimplemented')
  }

  try {
    return (await response.json()) as Response
  } catch {
    throw new ApiError('invalid response', response.status, '')
  }
}

async function callConnectJSONCandidates<Response>(
  procedurePaths: string[],
  body: Record<string, unknown>,
): Promise<Response> {
  let lastError: unknown = null
  for (const path of procedurePaths) {
    try {
      return await connectJSON<Response>(path, body)
    } catch (error) {
      lastError = error
      if (error instanceof ApiError) {
        const unimplemented =
          error.status === 404 ||
          error.code.toLowerCase().includes('unimplemented') ||
          error.code.toLowerCase().includes('not_found')
        if (unimplemented) {
          continue
        }
      }
      throw error
    }
  }

  throw lastError ?? new Error('request failed')
}

function messageFromError(error: unknown): string {
  if (error instanceof ConnectError) {
    if (error.code === Code.Unavailable) {
      return 'service unavailable'
    }
    return error.rawMessage !== '' ? error.rawMessage : 'request failed'
  }
  if (error instanceof ApiError) {
    return error.message
  }
  return 'request failed'
}

async function listMachines(): Promise<Machine[]> {
  const response = await machineClient.listMachines(create(ListMachinesRequestSchema))
  return response.machines
}

async function createMachine(name: string): Promise<Machine> {
  const response = await machineClient.createMachine(create(CreateMachineRequestSchema, { name }))
  if (response.machine == null) {
    throw new Error('request failed')
  }
  return response.machine
}

async function getMachine(id: string): Promise<Machine> {
  const response = await machineClient.getMachine(create(GetMachineRequestSchema, { machineId: id }))
  if (response.machine == null) {
    throw new Error('request failed')
  }
  return response.machine
}

async function updateMachine(id: string, name: string): Promise<Machine> {
  const response = await machineClient.updateMachine(
    create(UpdateMachineRequestSchema, { machineId: id, name }),
  )
  if (response.machine == null) {
    throw new Error('request failed')
  }
  return response.machine
}

async function startMachine(id: string): Promise<Machine> {
  const response = await machineClient.startMachine(create(StartMachineRequestSchema, { machineId: id }))
  if (response.machine == null) {
    throw new Error('request failed')
  }
  return response.machine
}

async function stopMachine(id: string): Promise<Machine> {
  const response = await machineClient.stopMachine(create(StopMachineRequestSchema, { machineId: id }))
  if (response.machine == null) {
    throw new Error('request failed')
  }
  return response.machine
}

async function deleteMachine(id: string): Promise<void> {
  await machineClient.deleteMachine(create(DeleteMachineRequestSchema, { machineId: id }))
}

async function getSetupStatus(): Promise<SetupStatus> {
  try {
    const response = await callConnectJSONCandidates<{
      status?: {
        completed?: boolean
        adminConfigured?: boolean
      }
      isConfigured?: boolean
      configured?: boolean
      setupCompleted?: boolean
      hasAdmin?: boolean
      adminConfigured?: boolean
    }>(
      ['/arca.v1.SetupService/GetSetupStatus', '/arca.v1.SetupService/GetStatus'],
      {},
    )

    const isConfigured =
      response.status?.completed ?? response.isConfigured ?? response.configured ?? response.setupCompleted ?? false
    const hasAdmin = response.status?.adminConfigured ?? response.hasAdmin ?? response.adminConfigured ?? false

    return { isConfigured, hasAdmin }
  } catch (error) {
    if (error instanceof ApiError && (error.status === 404 || error.code.toLowerCase().includes('unimplemented'))) {
      return { isConfigured: true, hasAdmin: true }
    }
    throw error
  }
}

async function setupCreateAdmin(email: string, password: string): Promise<User | null> {
  if (email.trim() === '' || password.trim() === '') {
    throw new Error('email and password are required')
  }
  return null
}

async function setupValidateCloudflare(apiToken: string, baseDomain: string): Promise<void> {
  const response = await callConnectJSONCandidates<{ valid?: boolean; message?: string }>(
    ['/arca.v1.SetupService/ValidateCloudflare', '/arca.v1.SetupService/ValidateCloudflareToken'],
    { apiToken, token: apiToken, baseDomain, domain: baseDomain },
  )
  if (response.valid !== true) {
    throw new Error(response.message ?? 'cloudflare token validation failed')
  }
}

async function setupConfigureProviderDocker(): Promise<void> {
  return
}

async function setupComplete(
  adminEmail: string,
  adminPassword: string,
  baseDomain: string,
  cloudflareApiToken: string,
): Promise<void> {
  try {
    const response = await callConnectJSONCandidates<{
      status?: {
        completed?: boolean
      }
      message?: string
    }>(['/arca.v1.SetupService/CompleteSetup'], {
      adminEmail,
      adminPassword,
      baseDomain,
      cloudflareApiToken,
      dockerProviderEnabled: true,
    })
    if (response.status?.completed !== true) {
      throw new Error(response.message ?? 'setup completion failed')
    }
  } catch (error) {
    if (error instanceof ApiError && (error.status === 404 || error.code.toLowerCase().includes('unimplemented'))) {
      return
    }
    throw error
  }
}

async function listExposures(machineID: string): Promise<Exposure[]> {
  try {
    const response = await callConnectJSONCandidates<{
      exposures?: Array<{
        id?: string
        exposureId?: string
        subdomain?: string
        name?: string
        hostname?: string
        domain?: string
        service?: string
        localPort?: number
        port?: number
        isPublic?: boolean
        public?: boolean
      }>
    }>(
      ['/arca.v1.TunnelService/ListMachineExposures', '/arca.v1.ExposureService/ListExposures'],
      { machineId: machineID },
    )

    return (response.exposures ?? []).map((item) => ({
      id: item.id ?? item.exposureId ?? '',
      subdomain: item.subdomain ?? item.name ?? '',
      hostname: item.hostname ?? item.domain ?? '',
      localPort: item.localPort ?? item.port ?? parseLocalPort(item.service) ?? 80,
      isPublic: item.isPublic ?? item.public ?? false,
    }))
  } catch (error) {
    if (error instanceof ApiError && (error.status === 404 || error.code.toLowerCase().includes('unimplemented'))) {
      return []
    }
    throw error
  }
}

async function createExposure(machineID: string, subdomain: string, localPort: number, isPublic: boolean): Promise<void> {
  await callConnectJSONCandidates(
    ['/arca.v1.TunnelService/UpsertMachineExposure', '/arca.v1.ExposureService/CreateExposure'],
    {
      machineId: machineID,
      name: subdomain,
      zoneId: '',
      subdomain,
      service: `http://localhost:${localPort}`,
      localPort,
      port: localPort,
      isPublic,
      public: isPublic,
    },
  )
}

async function updateExposureVisibility(machineID: string, exposure: Exposure, isPublic: boolean): Promise<void> {
  await callConnectJSONCandidates(
    ['/arca.v1.TunnelService/UpsertMachineExposure', '/arca.v1.ExposureService/UpdateExposureVisibility', '/arca.v1.ExposureService/UpdateExposure'],
    {
      machineId: machineID,
      name: exposure.subdomain,
      zoneId: '',
      service: `http://localhost:${exposure.localPort}`,
      exposureId: exposure.id,
      id: exposure.id,
      isPublic,
      public: isPublic,
    },
  )
}

function parseLocalPort(service: string | undefined): number | null {
  if (service == null || service === '') {
    return null
  }
  const matched = service.match(/localhost:(\d{1,5})/)
  if (matched == null) {
    return null
  }
  const port = Number(matched[1])
  if (!Number.isInteger(port) || port <= 0 || port > 65535) {
    return null
  }
  return port
}

export function App() {
  const [loading, setLoading] = useState(true)
  const [user, setUser] = useState<User | null>(null)
  const [setupStatus, setSetupStatus] = useState<SetupStatus>({ isConfigured: true, hasAdmin: true })

  useEffect(() => {
    const run = async () => {
      try {
        const status = await getSetupStatus()
        setSetupStatus(status)

        if (status.isConfigured) {
          try {
            const response = await authClient.me(create(MeRequestSchema))
            const me = toUser(response.user)
            if (me != null) {
              setUser(me)
            }
          } catch {
          }
        }
      } finally {
        setLoading(false)
      }
    }
    void run()
  }, [])

  const logout = async () => {
    try {
      await authClient.logout(create(LogoutRequestSchema))
    } finally {
      setUser(null)
    }
  }

  if (loading) {
    return (
      <main>
        <p>Loading...</p>
      </main>
    )
  }

  if (!setupStatus.isConfigured) {
    return (
      <Routes>
        <Route
          path="/setup"
          element={
            <SetupPage
              hasAdmin={setupStatus.hasAdmin}
              onAdminReady={setUser}
              onSetupComplete={() => setSetupStatus({ isConfigured: true, hasAdmin: true })}
            />
          }
        />
        <Route path="*" element={<Navigate to="/setup" replace />} />
      </Routes>
    )
  }

  return (
    <Routes>
      <Route path="/" element={<HomePage user={user} onLogout={logout} />} />
      <Route path="/setup" element={<Navigate to="/" replace />} />
      <Route path="/login" element={<LoginPage user={user} onLogin={setUser} />} />
      <Route path="/machines" element={<MachinesPage user={user} onLogout={logout} />} />
      <Route path="/machines/:machineID" element={<MachineDetailPage user={user} onLogout={logout} />} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}

type SetupPageProps = {
  hasAdmin: boolean
  onAdminReady: (user: User) => void
  onSetupComplete: () => void
}

function SetupPage({ hasAdmin, onAdminReady, onSetupComplete }: SetupPageProps) {
  const navigate = useNavigate()
  const [step, setStep] = useState(hasAdmin ? 2 : 1)
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [confirmPassword, setConfirmPassword] = useState('')
  const [baseDomain, setBaseDomain] = useState('')
  const [cloudflareToken, setCloudflareToken] = useState('')
  const [exposureMode, setExposureMode] = useState<'private' | 'public'>('private')
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
      await setupValidateCloudflare(cloudflareToken, baseDomain)
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
      await setupComplete(email, password, baseDomain, cloudflareToken)
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
              <CardTitle className="text-xl text-white">3. Setup provider (Docker)</CardTitle>
              <CardDescription className="text-slate-300">Docker is currently the supported machine provider.</CardDescription>
            </CardHeader>
            <CardContent className="p-6 pt-3">
              <form className="space-y-4" onSubmit={submitProvider}>
                <div className="rounded-lg border border-white/10 bg-white/[0.03] p-4">
                  <p className="text-sm text-slate-300">Provider</p>
                  <p className="mt-1 text-base font-semibold text-white">Local Docker</p>
                  <p className="mt-2 text-xs text-slate-400">Expose endpoints in private mode by default.</p>
                  <div className="mt-3 flex items-center gap-2">
                    <Button
                      type="button"
                      variant={exposureMode === 'private' ? 'default' : 'secondary'}
                      className="h-8 px-3"
                      onClick={() => setExposureMode('private')}
                    >
                      Private default
                    </Button>
                    <Button
                      type="button"
                      variant={exposureMode === 'public' ? 'default' : 'secondary'}
                      className="h-8 px-3"
                      onClick={() => setExposureMode('public')}
                    >
                      Public default
                    </Button>
                  </div>
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

type HomePageProps = {
  user: User | null
  onLogout: () => Promise<void>
}

function HomePage({ user, onLogout }: HomePageProps) {
  if (user == null) {
    return (
      <main className="flex min-h-dvh items-center justify-center bg-[radial-gradient(circle_at_top_left,_#f8fafc_10%,_#e2e8f0_55%,_#cbd5e1_100%)] px-6">
        <div className="w-full max-w-md rounded-2xl border border-slate-300/70 bg-white/90 p-10 shadow-xl shadow-slate-900/10 backdrop-blur">
          <p className="text-xs font-medium uppercase tracking-[0.24em] text-slate-500">Arca</p>
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
        <p className="text-xs font-medium uppercase tracking-[0.24em] text-slate-400">Arca</p>
        <h1 className="mt-2 text-2xl font-semibold">Dashboard</h1>
        <p className="mt-3 text-sm text-slate-300">Signed in as {user.email}</p>
        <div className="mt-6 flex items-center gap-3">
          <Button asChild type="button">
            <Link to="/machines">Machines</Link>
          </Button>
          <Button type="button" variant="secondary" onClick={onLogout}>
            Logout
          </Button>
        </div>
      </div>
    </main>
  )
}

type MachinesPageProps = {
  user: User | null
  onLogout: () => Promise<void>
}

function statusTone(status: string): string {
  switch (status) {
    case 'running':
      return 'border-emerald-400/40 bg-emerald-500/15 text-emerald-200'
    case 'starting':
    case 'pending':
      return 'border-sky-400/40 bg-sky-500/15 text-sky-200'
    case 'stopping':
      return 'border-amber-400/40 bg-amber-500/15 text-amber-200'
    case 'stopped':
      return 'border-slate-400/40 bg-slate-500/15 text-slate-200'
    case 'failed':
      return 'border-red-400/40 bg-red-500/15 text-red-200'
    default:
      return 'border-slate-400/40 bg-slate-500/15 text-slate-200'
  }
}

function exposureTone(isPublic: boolean): string {
  return isPublic
    ? 'border-emerald-400/40 bg-emerald-500/15 text-emerald-100'
    : 'border-slate-400/40 bg-slate-500/15 text-slate-200'
}

function StatusBadge({ status }: { status: string }) {
  return (
    <span className={`inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium uppercase tracking-[0.08em] ${statusTone(status)}`}>
      {status}
    </span>
  )
}

function ExposureBadge({ isPublic }: { isPublic: boolean }) {
  return (
    <span className={`inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium uppercase tracking-[0.08em] ${exposureTone(isPublic)}`}>
      {isPublic ? 'public' : 'private'}
    </span>
  )
}

function MachinesPage({ user, onLogout }: MachinesPageProps) {
  const [machines, setMachines] = useState<Machine[]>([])
  const [loading, setLoading] = useState(true)
  const [name, setName] = useState('')
  const [editingID, setEditingID] = useState<string | null>(null)
  const [editingName, setEditingName] = useState('')
  const [error, setError] = useState('')
  const [exposuresByMachine, setExposuresByMachine] = useState<Record<string, Exposure[]>>({})
  const [newExposureSubdomain, setNewExposureSubdomain] = useState<Record<string, string>>({})
  const [newExposurePort, setNewExposurePort] = useState<Record<string, string>>({})
  const [newExposurePublic, setNewExposurePublic] = useState<Record<string, boolean>>({})

  useEffect(() => {
    if (user == null) {
      return
    }

    const run = async () => {
      try {
        const items = await listMachines()
        setMachines(items)

        const exposureResults = await Promise.all(
          items.map(async (machine) => ({
            machineID: machine.id,
            exposures: await listExposures(machine.id),
          })),
        )

        setExposuresByMachine(
          exposureResults.reduce<Record<string, Exposure[]>>((acc, result) => {
            acc[result.machineID] = result.exposures
            return acc
          }, {}),
        )
      } catch (e) {
        setError(messageFromError(e))
      } finally {
        setLoading(false)
      }
    }

    void run()
    const timer = window.setInterval(() => {
      void run()
    }, 3000)
    return () => window.clearInterval(timer)
  }, [user])

  if (user == null) {
    return <Navigate to="/login" replace />
  }

  const submitCreate = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const trimmed = name.trim()
    if (trimmed === '') {
      setError('name is required')
      return
    }

    setError('')
    try {
      const created = await createMachine(trimmed)
      setMachines((prev) => [created, ...prev])
      setName('')
    } catch (e) {
      setError(messageFromError(e))
    }
  }

  const submitUpdate = async (machineID: string) => {
    const trimmed = editingName.trim()
    if (trimmed === '') {
      setError('name is required')
      return
    }

    setError('')
    try {
      const updated = await updateMachine(machineID, trimmed)
      setMachines((prev) => prev.map((machine) => (machine.id === machineID ? updated : machine)))
      setEditingID(null)
      setEditingName('')
    } catch (e) {
      setError(messageFromError(e))
    }
  }

  const submitStart = async (machineID: string) => {
    setError('')
    try {
      const updated = await startMachine(machineID)
      setMachines((prev) => prev.map((machine) => (machine.id === machineID ? updated : machine)))
    } catch (e) {
      setError(messageFromError(e))
    }
  }

  const submitStop = async (machineID: string) => {
    setError('')
    try {
      const updated = await stopMachine(machineID)
      setMachines((prev) => prev.map((machine) => (machine.id === machineID ? updated : machine)))
    } catch (e) {
      setError(messageFromError(e))
    }
  }

  const submitDelete = async (machineID: string) => {
    setError('')
    try {
      await deleteMachine(machineID)
      setMachines((prev) => prev.filter((machine) => machine.id !== machineID))
      setExposuresByMachine((prev) => {
        const next = { ...prev }
        delete next[machineID]
        return next
      })
      if (editingID === machineID) {
        setEditingID(null)
        setEditingName('')
      }
    } catch (e) {
      setError(messageFromError(e))
    }
  }

  const submitCreateExposure = async (machineID: string, event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const subdomain = (newExposureSubdomain[machineID] ?? '').trim()
    const portText = (newExposurePort[machineID] ?? '').trim()
    if (subdomain === '') {
      setError('subdomain is required')
      return
    }

    const port = Number(portText)
    if (!Number.isInteger(port) || port <= 0 || port > 65535) {
      setError('valid local port is required')
      return
    }

    setError('')
    try {
      await createExposure(machineID, subdomain, port, newExposurePublic[machineID] ?? false)
      const exposures = await listExposures(machineID)
      setExposuresByMachine((prev) => ({ ...prev, [machineID]: exposures }))
      setNewExposureSubdomain((prev) => ({ ...prev, [machineID]: '' }))
      setNewExposurePort((prev) => ({ ...prev, [machineID]: '' }))
      setNewExposurePublic((prev) => ({ ...prev, [machineID]: false }))
    } catch (e) {
      setError(messageFromError(e))
    }
  }

  const submitToggleExposure = async (machineID: string, exposure: Exposure) => {
    setError('')
    try {
      await updateExposureVisibility(machineID, exposure, !exposure.isPublic)
      setExposuresByMachine((prev) => ({
        ...prev,
        [machineID]: (prev[machineID] ?? []).map((item) =>
          item.id === exposure.id ? { ...item, isPublic: !exposure.isPublic } : item,
        ),
      }))
    } catch (e) {
      setError(messageFromError(e))
    }
  }

  return (
    <main className="relative min-h-dvh overflow-hidden bg-slate-950 px-6 py-16 text-slate-100">
      <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_20%_20%,_rgba(56,189,248,0.12),_transparent_38%),radial-gradient(circle_at_80%_0%,_rgba(148,163,184,0.2),_transparent_48%)]" />
      <div className="pointer-events-none absolute inset-0 bg-[linear-gradient(rgba(255,255,255,0.04)_1px,transparent_1px),linear-gradient(90deg,rgba(255,255,255,0.04)_1px,transparent_1px)] bg-[size:48px_48px] [mask-image:radial-gradient(ellipse_at_center,black_35%,transparent_75%)]" />

      <section className="relative z-10 mx-auto w-full max-w-4xl space-y-6">
        <header className="flex flex-col items-start justify-between gap-4 rounded-2xl border border-white/10 bg-white/[0.03] p-6 backdrop-blur md:flex-row md:items-center">
          <div>
            <p className="text-xs font-medium uppercase tracking-[0.24em] text-slate-400">Arca</p>
            <h1 className="mt-2 text-2xl font-semibold text-white">Machines</h1>
            <p className="mt-1 text-sm text-slate-300">Signed in as {user.email}</p>
          </div>
          <div className="flex items-center gap-3">
            <Button asChild type="button" variant="secondary">
              <Link to="/">Back</Link>
            </Button>
            <Button type="button" variant="secondary" onClick={onLogout}>
              Logout
            </Button>
          </div>
        </header>

        <Card className="border-white/15 bg-white/[0.04] py-0 shadow-2xl shadow-black/35 backdrop-blur-xl">
          <CardHeader className="space-y-2 p-6 pb-3">
            <CardTitle className="text-xl text-white">Create machine</CardTitle>
            <CardDescription className="text-slate-300">名前だけ付けられる最小構成です。</CardDescription>
          </CardHeader>
          <CardContent className="p-6 pt-3">
            <form className="flex flex-col gap-3 sm:flex-row" onSubmit={submitCreate}>
              <div className="w-full space-y-2">
                <Label htmlFor="machine-name" className="text-slate-200">
                  Name
                </Label>
                <Input
                  id="machine-name"
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                  className="h-10 border-white/20 bg-white/10 text-slate-100 placeholder:text-slate-400 focus-visible:ring-sky-400/45"
                  placeholder="my-machine"
                  required
                />
              </div>
              <Button type="submit" className="h-10 self-end bg-white text-slate-900 hover:bg-slate-100">
                Create
              </Button>
            </form>
          </CardContent>
        </Card>

        <Card className="border-white/15 bg-white/[0.04] py-0 shadow-2xl shadow-black/35 backdrop-blur-xl">
          <CardHeader className="space-y-2 p-6 pb-3">
            <CardTitle className="text-xl text-white">Machine list</CardTitle>
            <CardDescription className="text-slate-300">一覧から名前変更・起動・停止・削除・詳細確認ができます。</CardDescription>
          </CardHeader>
          <CardContent className="p-6 pt-3">
            {loading ? (
              <p className="text-sm text-slate-300">Loading...</p>
            ) : machines.length === 0 ? (
              <p className="text-sm text-slate-300">No machines yet.</p>
            ) : (
              <ul className="space-y-3">
                {machines.map((machine) => {
                  const editing = editingID === machine.id
                  const exposures = exposuresByMachine[machine.id] ?? []

                  return (
                    <li key={machine.id} className="rounded-lg border border-white/10 bg-white/[0.03] p-4">
                      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                        {editing ? (
                          <Input
                            value={editingName}
                            onChange={(event) => setEditingName(event.target.value)}
                            className="h-10 border-white/20 bg-white/10 text-slate-100 placeholder:text-slate-400 focus-visible:ring-sky-400/45 sm:max-w-sm"
                            aria-label="Edit machine name"
                          />
                        ) : (
                          <div className="space-y-2">
                            <p className="font-medium text-white">{machine.name}</p>
                            <div className="mt-1 flex items-center gap-2">
                              <StatusBadge status={machine.status} />
                              <span className="text-xs text-slate-300">desired: {machine.desiredStatus}</span>
                            </div>
                            <div className="space-y-1">
                              <p className="text-xs uppercase tracking-[0.08em] text-slate-400">Exposures</p>
                              {exposures.length === 0 ? (
                                <p className="text-xs text-slate-400">No exposures.</p>
                              ) : (
                                <ul className="space-y-1">
                                  {exposures.map((exposure) => (
                                    <li key={exposure.id} className="flex items-center gap-2 text-xs text-slate-200">
                                      <ExposureBadge isPublic={exposure.isPublic} />
                                      <span>{exposure.hostname || `${exposure.subdomain} (...)`}</span>
                                      <span className="text-slate-400">-&gt; localhost:{exposure.localPort}</span>
                                      <Button
                                        type="button"
                                        variant="secondary"
                                        className="h-7 px-2 text-xs"
                                        onClick={() => void submitToggleExposure(machine.id, exposure)}
                                      >
                                        Make {exposure.isPublic ? 'private' : 'public'}
                                      </Button>
                                    </li>
                                  ))}
                                </ul>
                              )}
                            </div>
                            {machine.lastError != null && machine.lastError !== '' && (
                              <p className="text-xs text-red-300 break-all">error: {machine.lastError}</p>
                            )}
                          </div>
                        )}

                        <div className="flex flex-wrap items-center justify-end gap-2 sm:max-w-md">
                          {editing ? (
                            <>
                              <Button
                                type="button"
                                className="h-9 bg-white px-3 text-slate-900 hover:bg-slate-100"
                                onClick={() => void submitUpdate(machine.id)}
                              >
                                Save
                              </Button>
                              <Button
                                type="button"
                                variant="secondary"
                                className="h-9 px-3"
                                onClick={() => {
                                  setEditingID(null)
                                  setEditingName('')
                                }}
                              >
                                Cancel
                              </Button>
                            </>
                          ) : (
                            <Button
                              type="button"
                              variant="secondary"
                              className="h-9 px-3"
                              onClick={() => {
                                setEditingID(machine.id)
                                setEditingName(machine.name)
                              }}
                            >
                              Edit
                            </Button>
                          )}
                          <Button
                            type="button"
                            variant="secondary"
                            className="h-9 px-3"
                            onClick={() => void submitStart(machine.id)}
                            disabled={machine.desiredStatus === 'running' && machine.status !== 'failed'}
                          >
                            Start
                          </Button>
                          <Button
                            type="button"
                            variant="secondary"
                            className="h-9 px-3"
                            onClick={() => void submitStop(machine.id)}
                            disabled={machine.desiredStatus === 'stopped' && machine.status !== 'failed'}
                          >
                            Stop
                          </Button>
                          <Button
                            type="button"
                            variant="secondary"
                            className="h-9 px-3"
                            onClick={() => void submitDelete(machine.id)}
                          >
                            Delete
                          </Button>
                          <Button asChild type="button" variant="secondary" className="h-9 px-3">
                            <Link to={`/machines/${machine.id}`}>Details</Link>
                          </Button>
                        </div>
                      </div>

                      <form className="mt-3 flex flex-col gap-2 rounded-lg border border-white/10 bg-black/20 p-3 sm:flex-row sm:items-end" onSubmit={(event) => void submitCreateExposure(machine.id, event)}>
                        <div className="w-full space-y-1 sm:max-w-44">
                          <Label htmlFor={`subdomain-${machine.id}`} className="text-xs text-slate-300">
                            Subdomain
                          </Label>
                          <Input
                            id={`subdomain-${machine.id}`}
                            value={newExposureSubdomain[machine.id] ?? ''}
                            onChange={(event) =>
                              setNewExposureSubdomain((prev) => ({ ...prev, [machine.id]: event.target.value }))
                            }
                            className="h-9 border-white/20 bg-white/10 text-sm text-slate-100"
                            placeholder="app"
                          />
                        </div>
                        <div className="w-full space-y-1 sm:max-w-32">
                          <Label htmlFor={`port-${machine.id}`} className="text-xs text-slate-300">
                            Local port
                          </Label>
                          <Input
                            id={`port-${machine.id}`}
                            type="number"
                            min={1}
                            max={65535}
                            value={newExposurePort[machine.id] ?? ''}
                            onChange={(event) =>
                              setNewExposurePort((prev) => ({ ...prev, [machine.id]: event.target.value }))
                            }
                            className="h-9 border-white/20 bg-white/10 text-sm text-slate-100"
                            placeholder="3000"
                          />
                        </div>
                        <Button
                          type="button"
                          variant={(newExposurePublic[machine.id] ?? false) ? 'default' : 'secondary'}
                          className="h-9 px-3"
                          onClick={() =>
                            setNewExposurePublic((prev) => ({ ...prev, [machine.id]: !(prev[machine.id] ?? false) }))
                          }
                        >
                          {(newExposurePublic[machine.id] ?? false) ? 'Public' : 'Private'}
                        </Button>
                        <Button type="submit" className="h-9 px-3 bg-white text-slate-900 hover:bg-slate-100">
                          Add exposure
                        </Button>
                      </form>
                    </li>
                  )
                })}
              </ul>
            )}

            {error !== '' && (
              <p role="alert" className="mt-4 rounded-md border border-red-400/30 bg-red-500/12 px-3 py-2 text-sm text-red-200">
                {error}
              </p>
            )}
          </CardContent>
        </Card>
      </section>
    </main>
  )
}

type MachineDetailPageProps = {
  user: User | null
  onLogout: () => Promise<void>
}

function MachineDetailPage({ user, onLogout }: MachineDetailPageProps) {
  const { machineID } = useParams()
  const [machine, setMachine] = useState<Machine | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [exposures, setExposures] = useState<Exposure[]>([])
  const [subdomain, setSubdomain] = useState('')
  const [port, setPort] = useState('')
  const [isPublic, setIsPublic] = useState(false)

  useEffect(() => {
    if (user == null || machineID == null || machineID === '') {
      return
    }

    const run = async () => {
      try {
        const [item, exposureItems] = await Promise.all([getMachine(machineID), listExposures(machineID)])
        setMachine(item)
        setExposures(exposureItems)
        setError('')
      } catch (e) {
        setError(messageFromError(e))
      } finally {
        setLoading(false)
      }
    }

    void run()
    const timer = window.setInterval(() => {
      void run()
    }, 3000)
    return () => window.clearInterval(timer)
  }, [user, machineID])

  if (user == null) {
    return <Navigate to="/login" replace />
  }
  if (machineID == null || machineID === '') {
    return <Navigate to="/machines" replace />
  }

  const handleStart = async () => {
    setError('')
    try {
      const updated = await startMachine(machineID)
      setMachine(updated)
    } catch (e) {
      setError(messageFromError(e))
    }
  }

  const handleStop = async () => {
    setError('')
    try {
      const updated = await stopMachine(machineID)
      setMachine(updated)
    } catch (e) {
      setError(messageFromError(e))
    }
  }

  const handleCreateExposure = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const trimmed = subdomain.trim()
    if (trimmed === '') {
      setError('subdomain is required')
      return
    }

    const parsedPort = Number(port)
    if (!Number.isInteger(parsedPort) || parsedPort <= 0 || parsedPort > 65535) {
      setError('valid local port is required')
      return
    }

    setError('')
    try {
      await createExposure(machineID, trimmed, parsedPort, isPublic)
      const items = await listExposures(machineID)
      setExposures(items)
      setSubdomain('')
      setPort('')
      setIsPublic(false)
    } catch (e) {
      setError(messageFromError(e))
    }
  }

  const handleToggleExposure = async (exposure: Exposure) => {
    setError('')
    try {
      await updateExposureVisibility(machineID, exposure, !exposure.isPublic)
      setExposures((prev) =>
        prev.map((item) =>
          item.id === exposure.id
            ? {
                ...item,
                isPublic: !exposure.isPublic,
              }
            : item,
        ),
      )
    } catch (e) {
      setError(messageFromError(e))
    }
  }

  return (
    <main className="relative min-h-dvh overflow-hidden bg-slate-950 px-6 py-16 text-slate-100">
      <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_20%_20%,_rgba(56,189,248,0.12),_transparent_38%),radial-gradient(circle_at_80%_0%,_rgba(148,163,184,0.2),_transparent_48%)]" />
      <div className="pointer-events-none absolute inset-0 bg-[linear-gradient(rgba(255,255,255,0.04)_1px,transparent_1px),linear-gradient(90deg,rgba(255,255,255,0.04)_1px,transparent_1px)] bg-[size:48px_48px] [mask-image:radial-gradient(ellipse_at_center,black_35%,transparent_75%)]" />

      <section className="relative z-10 mx-auto w-full max-w-3xl space-y-6">
        <header className="flex flex-col items-start justify-between gap-4 rounded-2xl border border-white/10 bg-white/[0.03] p-6 backdrop-blur md:flex-row md:items-center">
          <div>
            <p className="text-xs font-medium uppercase tracking-[0.24em] text-slate-400">Arca</p>
            <h1 className="mt-2 text-2xl font-semibold text-white">Machine detail</h1>
            <p className="mt-1 text-xs text-slate-400">{machineID}</p>
          </div>
          <div className="flex items-center gap-3">
            <Button asChild type="button" variant="secondary">
              <Link to="/machines">Back</Link>
            </Button>
            <Button type="button" variant="secondary" onClick={onLogout}>
              Logout
            </Button>
          </div>
        </header>

        <Card className="border-white/15 bg-white/[0.04] py-0 shadow-2xl shadow-black/35 backdrop-blur-xl">
          <CardHeader className="space-y-2 p-6 pb-3">
            <CardTitle className="text-xl text-white">Runtime</CardTitle>
            <CardDescription className="text-slate-300">現在状態と目標状態を表示します。</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4 p-6 pt-3">
            {loading ? (
              <p className="text-sm text-slate-300">Loading...</p>
            ) : machine == null ? (
              <p className="text-sm text-slate-300">Machine not found.</p>
            ) : (
              <>
                <div className="space-y-2 rounded-lg border border-white/10 bg-white/[0.03] p-4">
                  <p className="text-sm text-slate-300">Name</p>
                  <p className="text-lg font-semibold text-white">{machine.name}</p>
                </div>
                <div className="space-y-2 rounded-lg border border-white/10 bg-white/[0.03] p-4">
                  <p className="text-sm text-slate-300">Status</p>
                  <div className="flex items-center gap-2">
                    <StatusBadge status={machine.status} />
                    <span className="text-sm text-slate-300">desired: {machine.desiredStatus}</span>
                  </div>
                </div>
                <div className="space-y-3 rounded-lg border border-white/10 bg-white/[0.03] p-4">
                  <div className="flex items-center justify-between">
                    <p className="text-sm font-medium text-slate-200">Exposure controls</p>
                    <p className="text-xs text-slate-400">Default is private</p>
                  </div>
                  {exposures.length === 0 ? (
                    <p className="text-sm text-slate-400">No exposures configured.</p>
                  ) : (
                    <ul className="space-y-2">
                      {exposures.map((exposure) => (
                        <li
                          key={exposure.id}
                          className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-white/10 bg-black/20 p-3"
                        >
                          <div className="flex items-center gap-2">
                            <ExposureBadge isPublic={exposure.isPublic} />
                            <span className="text-sm text-slate-100">{exposure.hostname || exposure.subdomain}</span>
                            <span className="text-xs text-slate-400">-&gt; localhost:{exposure.localPort}</span>
                          </div>
                          <Button
                            type="button"
                            variant="secondary"
                            className="h-8 px-3"
                            onClick={() => void handleToggleExposure(exposure)}
                          >
                            Make {exposure.isPublic ? 'private' : 'public'}
                          </Button>
                        </li>
                      ))}
                    </ul>
                  )}

                  <form className="flex flex-col gap-2 rounded-md border border-white/10 bg-black/20 p-3 sm:flex-row sm:items-end" onSubmit={handleCreateExposure}>
                    <div className="w-full space-y-1 sm:max-w-44">
                      <Label htmlFor="exposure-subdomain" className="text-xs text-slate-300">
                        Subdomain
                      </Label>
                      <Input
                        id="exposure-subdomain"
                        value={subdomain}
                        onChange={(event) => setSubdomain(event.target.value)}
                        className="h-9 border-white/20 bg-white/10 text-sm text-slate-100"
                        placeholder="app"
                      />
                    </div>
                    <div className="w-full space-y-1 sm:max-w-32">
                      <Label htmlFor="exposure-port" className="text-xs text-slate-300">
                        Local port
                      </Label>
                      <Input
                        id="exposure-port"
                        type="number"
                        min={1}
                        max={65535}
                        value={port}
                        onChange={(event) => setPort(event.target.value)}
                        className="h-9 border-white/20 bg-white/10 text-sm text-slate-100"
                        placeholder="3000"
                      />
                    </div>
                    <Button
                      type="button"
                      variant={isPublic ? 'default' : 'secondary'}
                      className="h-9 px-3"
                      onClick={() => setIsPublic((prev) => !prev)}
                    >
                      {isPublic ? 'Public' : 'Private'}
                    </Button>
                    <Button type="submit" className="h-9 px-3 bg-white text-slate-900 hover:bg-slate-100">
                      Add exposure
                    </Button>
                  </form>
                </div>
                {machine.lastError != null && machine.lastError !== '' && (
                  <div className="rounded-lg border border-red-400/30 bg-red-500/12 p-4">
                    <p className="text-sm text-red-200">last error</p>
                    <p className="mt-1 text-xs text-red-100 break-all">{machine.lastError}</p>
                  </div>
                )}
                <div className="flex items-center gap-2">
                  <Button
                    type="button"
                    variant="secondary"
                    className="h-9 px-3"
                    onClick={() => void handleStart()}
                    disabled={machine.desiredStatus === 'running' && machine.status !== 'failed'}
                  >
                    Start
                  </Button>
                  <Button
                    type="button"
                    variant="secondary"
                    className="h-9 px-3"
                    onClick={() => void handleStop()}
                    disabled={machine.desiredStatus === 'stopped' && machine.status !== 'failed'}
                  >
                    Stop
                  </Button>
                </div>
              </>
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

type LoginPageProps = {
  user: User | null
  onLogin: (user: User) => void
}

function LoginPage({ user, onLogin }: LoginPageProps) {
  const navigate = useNavigate()
  const [mode, setMode] = useState<'login' | 'register'>('login')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')

  if (user != null) {
    return <Navigate to="/" replace />
  }

  const submit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setError('')
    setNotice('')

    try {
      if (mode === 'register') {
        await authClient.register(
          create(RegisterRequestSchema, {
            email,
            password,
          }),
        )
        setNotice('registered. please log in.')
        setMode('login')
        setPassword('')
        return
      }

      const response = await authClient.login(
        create(LoginRequestSchema, {
          email,
          password,
        }),
      )
      const loggedIn = toUser(response.user)
      if (loggedIn == null) {
        setError('request failed')
        return
      }

      onLogin(loggedIn)
      setPassword('')
      void navigate('/', { replace: true })
    } catch (e) {
      setError(messageFromError(e))
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
