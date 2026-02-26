import { create } from '@bufbuild/protobuf'
import { Code, ConnectError, createClient } from '@connectrpc/connect'
import { createConnectTransport } from '@connectrpc/connect-web'
import { useEffect, useState } from 'react'
import { Link, Navigate, Route, Routes, useNavigate, useParams } from 'react-router-dom'
import {
  AuthService,
  LoginRequestSchema,
  LogoutRequestSchema,
  MeRequestSchema,
  RegisterRequestSchema,
} from '@/gen/hayai/v1/auth_pb'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

type User = {
  id: string
  email: string
}

type Machine = {
  id: string
  name: string
  status: string
  desiredStatus: string
  lastError?: string
}

const authClient = createClient(
  AuthService,
  createConnectTransport({
    baseUrl: window.location.origin,
    credentials: 'include',
  }),
)

function toUser(user: { id: string; email: string } | undefined): User | null {
  if (user == null) {
    return null
  }
  return {
    id: user.id,
    email: user.email,
  }
}

function messageFromError(error: unknown): string {
  if (error instanceof ConnectError) {
    if (error.code === Code.Unavailable) {
      return 'service unavailable'
    }
    return error.rawMessage !== '' ? error.rawMessage : 'request failed'
  }
  return 'request failed'
}

async function parseMachineResponse(response: Response): Promise<Machine[]> {
  const payload = (await response.json()) as { machines?: Machine[]; error?: string }
  if (!response.ok) {
    throw new Error(payload.error ?? 'request failed')
  }
  return payload.machines ?? []
}

async function listMachines(): Promise<Machine[]> {
  const response = await fetch('/api/machines', {
    credentials: 'include',
  })
  return parseMachineResponse(response)
}

async function createMachine(name: string): Promise<Machine> {
  const response = await fetch('/api/machines', {
    method: 'POST',
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ name }),
  })
  const payload = (await response.json()) as Machine & { error?: string }
  if (!response.ok) {
    throw new Error(payload.error ?? 'request failed')
  }
  return payload
}

async function getMachine(id: string): Promise<Machine> {
  const response = await fetch(`/api/machines/${id}`, {
    credentials: 'include',
  })
  const payload = (await response.json()) as Machine & { error?: string }
  if (!response.ok) {
    throw new Error(payload.error ?? 'request failed')
  }
  return payload
}

async function updateMachine(id: string, name: string): Promise<Machine> {
  const response = await fetch(`/api/machines/${id}`, {
    method: 'PUT',
    credentials: 'include',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ name }),
  })
  const payload = (await response.json()) as { id: string; name: string; error?: string }
  if (!response.ok) {
    throw new Error(payload.error ?? 'request failed')
  }
  return {
    id: payload.id,
    name: payload.name,
    status: 'unknown',
    desiredStatus: 'unknown',
  }
}

async function startMachine(id: string): Promise<void> {
  const response = await fetch(`/api/machines/${id}/start`, {
    method: 'POST',
    credentials: 'include',
  })
  if (response.ok) {
    return
  }
  const payload = (await response.json()) as { error?: string }
  throw new Error(payload.error ?? 'request failed')
}

async function stopMachine(id: string): Promise<void> {
  const response = await fetch(`/api/machines/${id}/stop`, {
    method: 'POST',
    credentials: 'include',
  })
  if (response.ok) {
    return
  }
  const payload = (await response.json()) as { error?: string }
  throw new Error(payload.error ?? 'request failed')
}

async function deleteMachine(id: string): Promise<void> {
  const response = await fetch(`/api/machines/${id}`, {
    method: 'DELETE',
    credentials: 'include',
  })
  if (response.status === 204) {
    return
  }
  const payload = (await response.json()) as { error?: string }
  throw new Error(payload.error ?? 'request failed')
}

export function App() {
  const [loading, setLoading] = useState(true)
  const [user, setUser] = useState<User | null>(null)

  useEffect(() => {
    const run = async () => {
      try {
        const response = await authClient.me(create(MeRequestSchema))
        const me = toUser(response.user)
        if (me != null) {
          setUser(me)
        }
      } catch {
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

  return (
    <Routes>
      <Route path="/" element={<HomePage user={user} onLogout={logout} />} />
      <Route path="/login" element={<LoginPage user={user} onLogin={setUser} />} />
      <Route path="/machines" element={<MachinesPage user={user} onLogout={logout} />} />
      <Route path="/machines/:machineID" element={<MachineDetailPage user={user} onLogout={logout} />} />
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
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

function StatusBadge({ status }: { status: string }) {
  return (
    <span className={`inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium uppercase tracking-[0.08em] ${statusTone(status)}`}>
      {status}
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

  useEffect(() => {
    if (user == null) {
      return
    }

    const run = async () => {
      try {
        const items = await listMachines()
        setMachines(items)
      } catch (e) {
        setError(e instanceof Error ? e.message : 'request failed')
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
      setError(e instanceof Error ? e.message : 'request failed')
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
      setMachines((prev) =>
        prev.map((machine) =>
          machine.id === machineID
            ? { ...machine, name: updated.name }
            : machine,
        ),
      )
      setEditingID(null)
      setEditingName('')
    } catch (e) {
      setError(e instanceof Error ? e.message : 'request failed')
    }
  }

  const submitStart = async (machineID: string) => {
    setError('')
    try {
      await startMachine(machineID)
      setMachines((prev) =>
        prev.map((machine) =>
          machine.id === machineID
            ? { ...machine, status: 'pending', desiredStatus: 'running', lastError: '' }
            : machine,
        ),
      )
    } catch (e) {
      setError(e instanceof Error ? e.message : 'request failed')
    }
  }

  const submitStop = async (machineID: string) => {
    setError('')
    try {
      await stopMachine(machineID)
      setMachines((prev) =>
        prev.map((machine) =>
          machine.id === machineID
            ? { ...machine, status: 'stopping', desiredStatus: 'stopped', lastError: '' }
            : machine,
        ),
      )
    } catch (e) {
      setError(e instanceof Error ? e.message : 'request failed')
    }
  }

  const submitDelete = async (machineID: string) => {
    setError('')
    try {
      await deleteMachine(machineID)
      setMachines((prev) => prev.filter((machine) => machine.id !== machineID))
      if (editingID === machineID) {
        setEditingID(null)
        setEditingName('')
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : 'request failed')
    }
  }

  return (
    <main className="relative min-h-dvh overflow-hidden bg-slate-950 px-6 py-16 text-slate-100">
      <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_20%_20%,_rgba(56,189,248,0.12),_transparent_38%),radial-gradient(circle_at_80%_0%,_rgba(148,163,184,0.2),_transparent_48%)]" />
      <div className="pointer-events-none absolute inset-0 bg-[linear-gradient(rgba(255,255,255,0.04)_1px,transparent_1px),linear-gradient(90deg,rgba(255,255,255,0.04)_1px,transparent_1px)] bg-[size:48px_48px] [mask-image:radial-gradient(ellipse_at_center,black_35%,transparent_75%)]" />

      <section className="relative z-10 mx-auto w-full max-w-4xl space-y-6">
        <header className="flex flex-col items-start justify-between gap-4 rounded-2xl border border-white/10 bg-white/[0.03] p-6 backdrop-blur md:flex-row md:items-center">
          <div>
            <p className="text-xs font-medium uppercase tracking-[0.24em] text-slate-400">Hayai</p>
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
                  return (
                    <li
                      key={machine.id}
                      className="rounded-lg border border-white/10 bg-white/[0.03] p-4"
                    >
                      <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
                        {editing ? (
                          <Input
                            value={editingName}
                            onChange={(event) => setEditingName(event.target.value)}
                            className="h-10 border-white/20 bg-white/10 text-slate-100 placeholder:text-slate-400 focus-visible:ring-sky-400/45 sm:max-w-sm"
                            aria-label="Edit machine name"
                          />
                        ) : (
                          <div>
                            <p className="font-medium text-white">{machine.name}</p>
                            <div className="mt-1 flex items-center gap-2">
                              <StatusBadge status={machine.status} />
                              <span className="text-xs text-slate-300">desired: {machine.desiredStatus}</span>
                            </div>
                            {machine.lastError != null && machine.lastError !== '' && (
                              <p className="text-xs text-red-300">error: {machine.lastError}</p>
                            )}
                          </div>
                        )}

                        <div className="flex items-center gap-2">
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

  useEffect(() => {
    if (user == null || machineID == null || machineID === '') {
      return
    }

    const run = async () => {
      try {
        const item = await getMachine(machineID)
        setMachine(item)
        setError('')
      } catch (e) {
        setError(e instanceof Error ? e.message : 'request failed')
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
      await startMachine(machineID)
      setMachine((prev) => (prev == null ? prev : { ...prev, status: 'pending', desiredStatus: 'running', lastError: '' }))
    } catch (e) {
      setError(e instanceof Error ? e.message : 'request failed')
    }
  }

  const handleStop = async () => {
    setError('')
    try {
      await stopMachine(machineID)
      setMachine((prev) => (prev == null ? prev : { ...prev, status: 'stopping', desiredStatus: 'stopped', lastError: '' }))
    } catch (e) {
      setError(e instanceof Error ? e.message : 'request failed')
    }
  }

  return (
    <main className="relative min-h-dvh overflow-hidden bg-slate-950 px-6 py-16 text-slate-100">
      <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_20%_20%,_rgba(56,189,248,0.12),_transparent_38%),radial-gradient(circle_at_80%_0%,_rgba(148,163,184,0.2),_transparent_48%)]" />
      <div className="pointer-events-none absolute inset-0 bg-[linear-gradient(rgba(255,255,255,0.04)_1px,transparent_1px),linear-gradient(90deg,rgba(255,255,255,0.04)_1px,transparent_1px)] bg-[size:48px_48px] [mask-image:radial-gradient(ellipse_at_center,black_35%,transparent_75%)]" />

      <section className="relative z-10 mx-auto w-full max-w-3xl space-y-6">
        <header className="flex flex-col items-start justify-between gap-4 rounded-2xl border border-white/10 bg-white/[0.03] p-6 backdrop-blur md:flex-row md:items-center">
          <div>
            <p className="text-xs font-medium uppercase tracking-[0.24em] text-slate-400">Hayai</p>
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
                {machine.lastError != null && machine.lastError !== '' && (
                  <div className="rounded-lg border border-red-400/30 bg-red-500/12 p-4">
                    <p className="text-sm text-red-200">last error</p>
                    <p className="mt-1 text-xs text-red-100">{machine.lastError}</p>
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
