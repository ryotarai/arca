import { useEffect, useMemo, useState } from 'react'
import { Link, Navigate, useNavigate, useParams } from 'react-router-dom'
import { Copy, Check, ExternalLink, Terminal, Bot } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { ListSkeleton } from '@/components/ListSkeleton'
import { Skeleton } from '@/components/ui/skeleton'
import { SharingDialog } from '@/components/SharingDialog'
import {
  getMachine,
  deleteMachine,
  listMachineEvents,
  listMachineExposures,
  listAvailableMachineTemplates,
  listMachineAccessRequests,
  resolveMachineAccessRequest,
  startMachine,
  stopMachine,
  updateMachineOptions,
} from '@/lib/api'
import type { MachineAccessRequest } from '@/gen/arca/v1/sharing_pb'
import { messageFromError } from '@/lib/errors'
import type { Machine, MachineEvent, MachineTemplateSummary, User } from '@/lib/types'

type MachineDetailPageProps = {
  user: User | null
  baseDomain?: string
  domainPrefix?: string
}

function machineHostname(prefix: string, machineName: string, baseDomain: string): string {
  return `${prefix}${machineName}.${baseDomain}`
}

const pollingIntervalMs = 60000
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

function CopyableID({ id }: { id: string }) {
  const [copied, setCopied] = useState(false)

  const handleCopy = async () => {
    await navigator.clipboard.writeText(id)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <button
      type="button"
      onClick={() => void handleCopy()}
      className="group inline-flex items-center gap-1.5 text-xs text-muted-foreground transition hover:text-foreground"
      title="Copy machine ID"
    >
      <span className="font-mono">{id}</span>
      {copied ? (
        <Check className="h-3 w-3 text-emerald-400" />
      ) : (
        <Copy className="h-3 w-3 opacity-0 transition group-hover:opacity-100" />
      )}
    </button>
  )
}

function AccessRequestsPanel({
  machineID,
  requests,
  onResolved,
}: {
  machineID: string
  requests: MachineAccessRequest[]
  onResolved: (id: string) => void
}) {
  const [resolving, setResolving] = useState<string | null>(null)
  const [roles, setRoles] = useState<Record<string, string>>({})
  const [panelError, setPanelError] = useState('')

  const handleResolve = async (requestID: string, action: 'approve' | 'deny') => {
    setResolving(requestID)
    setPanelError('')
    try {
      const role = action === 'approve' ? (roles[requestID] || 'viewer') : ''
      await resolveMachineAccessRequest(requestID, action, role)
      onResolved(requestID)
    } catch (e) {
      setPanelError(messageFromError(e))
    } finally {
      setResolving(null)
    }
  }

  return (
    <Card className="py-0 shadow-sm">
      <CardHeader className="space-y-2 p-6 pb-3">
        <CardTitle className="text-xl">Access requests</CardTitle>
        <CardDescription>Users requesting access to this machine.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3 p-6 pt-3">
        {panelError !== '' && (
          <p className="rounded-md border border-red-400/30 bg-red-500/12 px-3 py-2 text-sm text-red-200">
            {panelError}
          </p>
        )}
        {requests.map((req) => (
          <div key={req.id} className="flex flex-col gap-3 rounded-lg border border-border bg-muted/30 p-4 sm:flex-row sm:items-center sm:justify-between">
            <div className="min-w-0">
              <p className="truncate text-sm font-medium text-foreground">{req.email}</p>
              {req.message !== '' && (
                <p className="mt-1 text-xs text-muted-foreground">{req.message}</p>
              )}
              <p className="mt-1 text-xs text-muted-foreground">
                {new Date(Number(req.createdAt) * 1000).toLocaleString()}
              </p>
            </div>
            <div className="flex items-center gap-2">
              <select
                className="h-9 rounded-md border border-border bg-background px-2 text-sm text-foreground"
                value={roles[req.id] || 'viewer'}
                onChange={(e) => setRoles((prev) => ({ ...prev, [req.id]: e.target.value }))}
                disabled={resolving === req.id}
              >
                <option value="viewer">Viewer — read-only access</option>
                <option value="editor">Editor — terminal access</option>
              </select>
              <Button
                type="button"
                size="sm"
                disabled={resolving === req.id}
                onClick={() => void handleResolve(req.id, 'approve')}
              >
                Approve
              </Button>
              <Button
                type="button"
                variant="secondary"
                size="sm"
                disabled={resolving === req.id}
                onClick={() => void handleResolve(req.id, 'deny')}
              >
                Deny
              </Button>
            </div>
          </div>
        ))}
      </CardContent>
    </Card>
  )
}

export function MachineDetailPage({ user, baseDomain = '', domainPrefix = '' }: MachineDetailPageProps) {
  const { machineID } = useParams()
  const navigate = useNavigate()
  const [machine, setMachine] = useState<Machine | null>(null)
  const [events, setEvents] = useState<MachineEvent[]>([])
  const [templates, setTemplates] = useState<MachineTemplateSummary[]>([])
  const [editingMachineType, setEditingMachineType] = useState(false)
  const [editMachineType, setEditMachineType] = useState('')
  const [savingMachineType, setSavingMachineType] = useState(false)
  const [loading, setLoading] = useState(true)
  const [deleting, setDeleting] = useState(false)
  const [error, setError] = useState('')
  const [sharingOpen, setSharingOpen] = useState(false)
  const [accessRequests, setAccessRequests] = useState<MachineAccessRequest[]>([])
  const [confirmAction, setConfirmAction] = useState<{ title: string; description: string; confirmLabel: string; variant: 'default' | 'destructive'; onConfirm: () => void } | null>(null)
  const endpointURL = machine == null || machine.name === '' || baseDomain === '' ? null : `https://${machineHostname(domainPrefix, machine.name, baseDomain)}`
  const ttydURL = endpointURL != null ? `${endpointURL}/__arca/ttyd` : null
  const shelleyURL = endpointURL != null ? `${endpointURL}/__arca/shelley` : null
  const isRunning = machine?.status === 'running'
  const isAdmin = machine?.userRole === 'admin'
  const isEditor = machine?.userRole === 'editor'
  const isTransitioning = machine?.status === 'starting' || machine?.status === 'stopping' || machine?.status === 'pending' || machine?.status === 'deleting'

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
        const [item, eventItems, exposureItems, templateItems] = await Promise.all([
          getMachine(machineID, { timeoutMs: pollingRequestTimeoutMs }),
          listMachineEvents(machineID, eventLimit, { timeoutMs: pollingRequestTimeoutMs }),
          listMachineExposures(machineID),
          listAvailableMachineTemplates(),
        ])
        if (!cancelled) {
          setMachine(item)
          setEvents(eventItems)
          setTemplates(templateItems)
          setError('')
          // Fetch access requests for admins
          if (item.userRole === 'admin') {
            try {
              const reqs = await listMachineAccessRequests(machineID)
              if (!cancelled) setAccessRequests(reqs)
            } catch {
              // ignore errors for access requests polling
            }
          }
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
          }, pollingIntervalMs)
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

  const handleStop = () => {
    setConfirmAction({
      title: 'Stop machine',
      description: 'Are you sure you want to stop this machine?',
      confirmLabel: 'Stop',
      variant: 'destructive',
      onConfirm: () => {
        void (async () => {
          setError('')
          try {
            const updated = await stopMachine(machineID)
            setMachine(updated)
          } catch (e) {
            setError(messageFromError(e))
          }
        })()
      },
    })
  }

  const handleRestart = () => {
    setConfirmAction({
      title: 'Restart machine',
      description: 'Are you sure you want to restart this machine?',
      confirmLabel: 'Restart',
      variant: 'default',
      onConfirm: () => {
        void (async () => {
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
        })()
      },
    })
  }

  const handleDelete = () => {
    setConfirmAction({
      title: 'Delete machine',
      description: 'Are you sure you want to delete this machine? This action cannot be undone.',
      confirmLabel: 'Delete',
      variant: 'destructive',
      onConfirm: () => {
        void (async () => {
          setError('')
          setDeleting(true)
          try {
            await deleteMachine(machineID)
            await navigate('/machines')
          } catch (e) {
            setError(messageFromError(e))
            setDeleting(false)
          }
        })()
      },
    })
  }

  return (
    <main className="px-6 py-10">
      <section className="mx-auto w-full max-w-3xl space-y-6">
        {/* Page header: machine name + status + share action */}
        <header className="flex flex-col items-start justify-between gap-4 md:flex-row md:items-center">
          <div className="min-w-0">
            <h1 className="truncate text-2xl font-semibold text-foreground">
              {loading ? <Skeleton className="h-7 w-48 inline-block" /> : machine?.name ?? 'Machine not found'}
            </h1>
            {machine != null && (
              <div className="mt-2 flex items-center gap-3">
                <StatusBadge status={machine.status} />
                <CopyableID id={machineID} />
              </div>
            )}
          </div>
          <div className="flex items-center gap-2">
            {isRunning && endpointURL != null && (
              <>
                <Button asChild variant="secondary" className="h-9 px-3">
                  <a href={endpointURL} target="_blank" rel="noreferrer">
                    <ExternalLink className="h-4 w-4" /> Endpoint
                  </a>
                </Button>
                {(isAdmin || isEditor) && ttydURL != null && (
                  <Button asChild variant="secondary" className="h-9 px-3">
                    <a href={ttydURL} target="_blank" rel="noreferrer">
                      <Terminal className="h-4 w-4" /> Terminal
                    </a>
                  </Button>
                )}
                {(isAdmin || isEditor) && shelleyURL != null && (
                  <Button asChild variant="secondary" className="h-9 px-3">
                    <a href={shelleyURL} target="_blank" rel="noreferrer">
                      <Bot className="h-4 w-4" /> Shelley
                    </a>
                  </Button>
                )}
              </>
            )}
            {isAdmin && (
              <Button type="button" variant="secondary" onClick={() => setSharingOpen(true)}>
                Share
              </Button>
            )}
          </div>
        </header>

        {error !== '' && (
          <p role="alert" className="rounded-md border border-red-400/30 bg-red-500/12 px-3 py-2 text-sm text-red-200">
            {error}
          </p>
        )}

        {!loading && machine != null && (
          <>
            {/* Machine information card */}
            <Card className="py-0 shadow-sm">
              <CardHeader className="space-y-2 p-6 pb-3">
                <CardTitle className="text-xl">Information</CardTitle>
              </CardHeader>
              <CardContent className="space-y-4 p-6 pt-3">
                {isAdmin && (
                  <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-4">
                    <p className="text-sm text-muted-foreground">Template</p>
                    {machine.templateId === '' ? (
                      <p className="text-sm text-foreground">Unassigned</p>
                    ) : (
                      <Link
                        to={`/machine-templates/${machine.templateId}`}
                        className="text-sm text-sky-300 underline decoration-sky-300/50 underline-offset-2 transition hover:text-sky-200"
                      >
                        {templates.find((r) => r.id === machine.templateId)?.name ?? machine.templateId}
                      </Link>
                    )}
                    {machine.templateType && (
                      <p className="text-xs text-muted-foreground">Type: {machine.templateType}</p>
                    )}
                  </div>
                )}
                {(() => {
                  const rt = templates.find((r) => r.id === machine.templateId)
                  if (rt == null || rt.type !== 'gce') return null
                  const currentMT = machine.options?.machine_type || (rt.allowedMachineTypes ?? [])[0] || 'e2-standard-2'
                  const isStopped = machine.status === 'stopped'
                  const allowed = rt.allowedMachineTypes ?? []

                  const handleSaveMachineType = async () => {
                    setSavingMachineType(true)
                    setError('')
                    try {
                      const updated = await updateMachineOptions(machineID, { machine_type: editMachineType.trim() })
                      setMachine(updated)
                      setEditingMachineType(false)
                    } catch (e) {
                      setError(messageFromError(e))
                    } finally {
                      setSavingMachineType(false)
                    }
                  }

                  return (
                    <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-4">
                      <p className="text-sm text-muted-foreground">Machine type</p>
                      {editingMachineType ? (
                        <div className="flex items-center gap-2">
                          {allowed.length > 0 ? (
                            <select
                              value={editMachineType}
                              onChange={(e) => setEditMachineType(e.target.value)}
                              className="h-9 flex-1 rounded-md border border-input bg-background px-3 text-sm text-foreground"
                              disabled={savingMachineType}
                            >
                              {allowed.map((mt) => (
                                <option key={mt} value={mt}>{mt}</option>
                              ))}
                            </select>
                          ) : (
                            <input
                              type="text"
                              value={editMachineType}
                              onChange={(e) => setEditMachineType(e.target.value)}
                              className="h-9 flex-1 rounded-md border border-input bg-background px-3 text-sm text-foreground"
                              disabled={savingMachineType}
                              placeholder={currentMT}
                            />
                          )}
                          <Button type="button" size="sm" disabled={savingMachineType || editMachineType.trim() === ''} onClick={() => void handleSaveMachineType()}>
                            {savingMachineType ? 'Saving...' : 'Save'}
                          </Button>
                          <Button type="button" variant="secondary" size="sm" disabled={savingMachineType} onClick={() => setEditingMachineType(false)}>
                            Cancel
                          </Button>
                        </div>
                      ) : (
                        <div className="flex items-center gap-2">
                          <p className="text-sm font-medium text-foreground">{currentMT}</p>
                          {isAdmin && isStopped && (
                            <Button
                              type="button"
                              variant="secondary"
                              size="sm"
                              className="h-7 px-2 text-xs"
                              onClick={() => {
                                setEditMachineType(currentMT)
                                setEditingMachineType(true)
                              }}
                            >
                              Edit
                            </Button>
                          )}
                        </div>
                      )}
                    </div>
                  )
                })()}
                {isRunning && endpointURL != null && (
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
                    <p className="text-xs text-muted-foreground">Proxied to localhost:11030 inside the machine</p>
                  </div>
                )}
                {machine.lastError != null && machine.lastError !== '' && (
                  <div className="rounded-lg border border-red-400/30 bg-red-500/12 p-4">
                    <p className="text-sm text-red-200">Last error</p>
                    <p className="mt-1 break-all text-xs text-red-100">{machine.lastError}</p>
                  </div>
                )}
              </CardContent>
            </Card>

            {/* Lifecycle controls card (admin only) */}
            {isAdmin && (
              <Card className="py-0 shadow-sm">
                <CardHeader className="space-y-2 p-6 pb-3">
                  <CardTitle className="text-xl">Controls</CardTitle>
                  <CardDescription>Manage machine lifecycle.</CardDescription>
                </CardHeader>
                <CardContent className="space-y-4 p-6 pt-3">
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
                      onClick={() => handleStop()}
                      disabled={machine.desiredStatus === 'stopped' && machine.status !== 'failed'}
                    >
                      Stop
                    </Button>
                    <Button
                      type="button"
                      variant="secondary"
                      className="h-9 px-3"
                      onClick={() => handleRestart()}
                      disabled={isTransitioning}
                    >
                      {machine.updateRequired && !isTransitioning ? 'Restart to update' : 'Restart'}
                    </Button>
                  </div>
                  <div className="border-t border-border pt-4">
                    <Button
                      type="button"
                      variant="destructive"
                      className="h-9 px-3"
                      onClick={() => handleDelete()}
                      disabled={deleting}
                    >
                      {deleting ? 'Deleting...' : 'Delete machine'}
                    </Button>
                  </div>
                </CardContent>
              </Card>
            )}

            {/* Access requests (admin only) */}
            {isAdmin && accessRequests.length > 0 && (
              <AccessRequestsPanel
                machineID={machineID}
                requests={accessRequests}
                onResolved={(id) => setAccessRequests((prev) => prev.filter((r) => r.id !== id))}
              />
            )}

            {/* Machine events (admin only) */}
            {isAdmin && (
              <Card className="py-0 shadow-sm">
                <CardHeader className="space-y-2 p-6 pb-3">
                  <CardTitle className="text-xl">Events</CardTitle>
                  <CardDescription>Recent state transitions and worker activities.</CardDescription>
                </CardHeader>
                <CardContent className="p-6 pt-3">
                  {sortedEvents.length === 0 ? (
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
            )}
          </>
        )}

        {loading && (
          <ListSkeleton count={2} />
        )}

        {!loading && machine == null && (
          <p className="text-sm text-muted-foreground">Machine not found.</p>
        )}
      </section>

      {isAdmin && (
        <SharingDialog
          machineID={machineID}
          open={sharingOpen}
          onOpenChange={setSharingOpen}
        />
      )}

      <ConfirmDialog
        open={confirmAction != null}
        onOpenChange={(open) => { if (!open) setConfirmAction(null) }}
        title={confirmAction?.title ?? ''}
        description={confirmAction?.description ?? ''}
        confirmLabel={confirmAction?.confirmLabel ?? 'Confirm'}
        variant={confirmAction?.variant ?? 'default'}
        onConfirm={() => {
          confirmAction?.onConfirm()
          setConfirmAction(null)
        }}
      />
    </main>
  )
}
