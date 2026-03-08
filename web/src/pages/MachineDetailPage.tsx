import { useEffect, useMemo, useState } from 'react'
import { Link, Navigate, useNavigate, useParams } from 'react-router-dom'
import { EndpointVisibility } from '@/gen/arca/v1/tunnel_pb'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import {
  getMachine,
  deleteMachine,
  listMachineEvents,
  listMachineExposures,
  listRuntimes,
  startMachine,
  stopMachine,
} from '@/lib/api'
import { messageFromError } from '@/lib/errors'
import type { Machine, MachineEvent, MachineExposure, RuntimeCatalogItem, User } from '@/lib/types'

type MachineDetailPageProps = {
  user: User | null
  onLogout: () => Promise<void>
}

const pollingRequestTimeoutMs = 2500
const restartWaitTimeoutMs = 60000
const restartWaitIntervalMs = 1500
const eventLimit = 100

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

function eventLevelTone(level: string): string {
  switch (level) {
    case 'error':
      return 'border-red-400/40 bg-red-500/15 text-red-200'
    case 'warn':
      return 'border-amber-400/40 bg-amber-500/15 text-amber-200'
    default:
      return 'border-sky-400/40 bg-sky-500/15 text-sky-200'
  }
}

function StatusBadge({ status }: { status: string }) {
  return (
    <span
      className={`inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium uppercase tracking-[0.08em] ${statusTone(status)}`}
    >
      {status}
    </span>
  )
}

function formatEventTimestamp(createdAt: number): string {
  if (createdAt <= 0) {
    return 'unknown time'
  }
  return new Date(createdAt * 1000).toLocaleString()
}

function EventLevelBadge({ level }: { level: string }) {
  const normalized = level.trim().toLowerCase() || 'info'
  return (
    <span
      className={`inline-flex items-center rounded-full border px-2 py-0.5 text-[10px] font-medium uppercase tracking-[0.08em] ${eventLevelTone(normalized)}`}
    >
      {normalized}
    </span>
  )
}

export function MachineDetailPage({ user, onLogout }: MachineDetailPageProps) {
  const { machineID } = useParams()
  const navigate = useNavigate()
  const [machine, setMachine] = useState<Machine | null>(null)
  const [events, setEvents] = useState<MachineEvent[]>([])
  const [runtimes, setRuntimes] = useState<RuntimeCatalogItem[]>([])
  const [loading, setLoading] = useState(true)
  const [deleting, setDeleting] = useState(false)
  const [error, setError] = useState('')
  const [defaultExposure, setDefaultExposure] = useState<MachineExposure | null>(null)
  const [exposureVisibility, setExposureVisibility] = useState<EndpointVisibility>(EndpointVisibility.OWNER_ONLY)
  const endpointURL = machine == null || machine.endpoint === '' ? null : `https://${machine.endpoint}`
  const ttydURL = endpointURL != null ? `${endpointURL}/__arca/ttyd` : null

  const sortedEvents = useMemo(() => {
    return [...events].sort((a, b) => Number(b.createdAt) - Number(a.createdAt))
  }, [events])

  useEffect(() => {
    if (user == null || machineID == null || machineID === '') {
      return
    }

    let cancelled = false
    let timer: number | null = null
    let running = false

    const run = async () => {
      if (cancelled || running) {
        return
      }
      running = true
      try {
        const [item, eventItems, exposureItems, runtimeItems] = await Promise.all([
          getMachine(machineID, { timeoutMs: pollingRequestTimeoutMs }),
          listMachineEvents(machineID, eventLimit, { timeoutMs: pollingRequestTimeoutMs }),
          listMachineExposures(machineID),
          listRuntimes(),
        ])
        if (!cancelled) {
          setMachine(item)
          setEvents(eventItems)
          setRuntimes(runtimeItems)
          const defaultItem = exposureItems.find((item) => item.name === 'default') ?? null
          setDefaultExposure(defaultItem)
          setExposureVisibility(defaultItem?.visibility ?? EndpointVisibility.OWNER_ONLY)
          setError('')
        }
      } catch (e) {
        if (!cancelled) {
          setError(messageFromError(e))
        }
      } finally {
        running = false
        if (!cancelled) {
          setLoading(false)
          timer = window.setTimeout(() => {
            void run()
          }, 3000)
        }
      }
    }

    void run()

    return () => {
      cancelled = true
      if (timer != null) {
        window.clearTimeout(timer)
      }
    }
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
    if (!window.confirm('Stop this machine?')) {
      return
    }

    setError('')
    try {
      const updated = await stopMachine(machineID)
      setMachine(updated)
    } catch (e) {
      setError(messageFromError(e))
    }
  }

  const handleRestart = async () => {
    if (!window.confirm('Restart this machine?')) {
      return
    }

    setError('')
    try {
      await stopMachine(machineID)
      const startedAt = Date.now()
      while (Date.now() < startedAt + restartWaitTimeoutMs) {
        const current = await getMachine(machineID)
        setMachine(current)
        if (current.status === 'stopped') {
          break
        }
        await new Promise<void>((resolve) => {
          window.setTimeout(resolve, restartWaitIntervalMs)
        })
      }
      const updated = await startMachine(machineID)
      setMachine(updated)
    } catch (e) {
      setError(messageFromError(e))
    }
  }

  const handleDelete = async () => {
    if (!window.confirm('Delete this machine? This action cannot be undone.')) {
      return
    }

    setError('')
    setDeleting(true)
    try {
      await deleteMachine(machineID)
      await navigate('/machines')
    } catch (e) {
      setError(messageFromError(e))
      setDeleting(false)
    }
  }

  return (
    <main className="min-h-dvh px-6 py-10">
      <section className="mx-auto w-full max-w-3xl space-y-6">
        <header className="flex flex-col items-start justify-between gap-4 rounded-xl border border-border bg-muted/30 p-6 md:flex-row md:items-center">
          <div>
            <p className="text-xs font-medium uppercase tracking-[0.24em] text-muted-foreground">Arca</p>
            <h1 className="mt-2 text-2xl font-semibold text-foreground">Machine detail</h1>
            <p className="mt-1 text-xs text-muted-foreground">{machineID}</p>
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

        <Card className="py-0 shadow-sm">
          <CardHeader className="space-y-2 p-6 pb-3">
            <CardTitle className="text-xl">Machine overview</CardTitle>
            <CardDescription>Runtime, state, endpoint, and lifecycle controls.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4 p-6 pt-3">
            {loading ? (
              <p className="text-sm text-muted-foreground">Loading...</p>
            ) : machine == null ? (
              <p className="text-sm text-muted-foreground">Machine not found.</p>
            ) : (
              <>
                <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-4">
                  <p className="text-sm text-muted-foreground">Name</p>
                  <p className="text-lg font-semibold text-foreground">{machine.name}</p>
                </div>
                <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-4">
                  <p className="text-sm text-muted-foreground">Runtime</p>
                  {machine.runtimeId === '' ? (
                    <p className="text-sm text-foreground">Unassigned</p>
                  ) : (
                    <Link
                      to={`/runtimes/${machine.runtimeId}`}
                      className="text-sm text-sky-300 underline decoration-sky-300/50 underline-offset-2 transition hover:text-sky-200"
                    >
                      {runtimes.find((r) => r.id === machine.runtimeId)?.name ?? machine.runtimeId}
                    </Link>
                  )}
                </div>
                <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-4">
                  <p className="text-sm text-muted-foreground">Status</p>
                  <div className="flex items-center gap-2">
                    <StatusBadge status={machine.status} />
                  </div>
                </div>
                {endpointURL != null && (
                  <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-4">
                    <p className="text-sm text-muted-foreground">Endpoint</p>
                    <a
                      href={endpointURL}
                      target="_blank"
                      rel="noreferrer"
                      className="text-sm text-sky-300 underline decoration-sky-300/50 underline-offset-2 transition hover:text-sky-200"
                    >
                      {endpointURL}
                    </a>
                    <p className="text-xs text-muted-foreground">Proxied to localhost:8080 inside the machine</p>
                  </div>
                )}
                {ttydURL != null && (
                  <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-4">
                    <p className="text-sm text-muted-foreground">Terminal (ttyd)</p>
                    <a
                      href={ttydURL}
                      target="_blank"
                      rel="noreferrer"
                      className="text-sm text-sky-300 underline decoration-sky-300/50 underline-offset-2 transition hover:text-sky-200"
                    >
                      {ttydURL}
                    </a>
                  </div>
                )}
                <div className="space-y-3 rounded-lg border border-border bg-muted/30 p-4">
                  <p className="text-sm text-muted-foreground">Endpoint visibility</p>
                  <p className="text-sm text-foreground">
                    {exposureVisibility === EndpointVisibility.OWNER_ONLY && 'Owner only'}
                    {exposureVisibility === EndpointVisibility.SELECTED_USERS && 'Selected Arca users'}
                    {exposureVisibility === EndpointVisibility.ALL_ARCA_USERS && 'All Arca users'}
                    {exposureVisibility === EndpointVisibility.INTERNET_PUBLIC && 'Internet public'}
                  </p>
                  <Button asChild type="button" variant="secondary" className="h-9 px-3">
                    <Link to={`/machines/${machineID}/edit`}>Edit visibility</Link>
                  </Button>
                </div>
                {machine.lastError != null && machine.lastError !== '' && (
                  <div className="rounded-lg border border-red-400/30 bg-red-500/12 p-4">
                    <p className="text-sm text-red-200">last error</p>
                    <p className="mt-1 break-all text-xs text-red-100">{machine.lastError}</p>
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
                  <Button
                    type="button"
                    variant="secondary"
                    className="h-9 px-3"
                    onClick={() => void handleRestart()}
                    disabled={machine.status === 'starting' || machine.status === 'stopping' || machine.status === 'pending' || machine.status === 'deleting'}
                  >
                    {machine.updateRequired ? 'Restart to update' : 'Restart'}
                  </Button>
                  <Button
                    type="button"
                    variant="secondary"
                    className="h-9 px-3"
                    onClick={() => void handleDelete()}
                    disabled={deleting}
                  >
                    {deleting ? 'Deleting...' : 'Delete'}
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

        <Card className="py-0 shadow-sm">
          <CardHeader className="space-y-2 p-6 pb-3">
            <CardTitle className="text-xl">Machine events</CardTitle>
            <CardDescription>Recent state transitions and worker activities.</CardDescription>
          </CardHeader>
          <CardContent className="p-6 pt-3">
            {loading ? (
              <p className="text-sm text-muted-foreground">Loading events...</p>
            ) : sortedEvents.length === 0 ? (
              <p className="text-sm text-muted-foreground">No events yet.</p>
            ) : (
              <div className="space-y-2">
                {sortedEvents.map((event) => (
                  <div key={event.id} className="rounded-lg border border-border bg-muted/30 p-3">
                    <div className="flex flex-wrap items-center justify-between gap-2">
                      <div className="flex items-center gap-2">
                        <EventLevelBadge level={event.level} />
                        <span className="text-xs font-medium uppercase tracking-[0.08em] text-muted-foreground">{event.eventType}</span>
                      </div>
                      <span className="text-xs text-muted-foreground">{formatEventTimestamp(Number(event.createdAt))}</span>
                    </div>
                    <p className="mt-2 text-sm text-foreground">{event.message}</p>
                    {event.jobId !== '' && <p className="mt-1 text-xs text-muted-foreground">job: {event.jobId}</p>}
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </section>
    </main>
  )
}
