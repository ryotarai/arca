import { useCallback, useMemo, useState } from 'react'
import { Link, Navigate, useNavigate, useParams } from 'react-router-dom'
import { Copy, Check, ExternalLink, Terminal, Bot, Camera } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { SharingDialog } from '@/components/SharingDialog'
import { CreateImageDialog } from '@/components/CreateImageDialog'
import {
  getMachine,
  deleteMachine,
  listMachineEvents,
  listMachineExposures,
  listAvailableProfiles,
  listMachineAccessRequests,
  resolveMachineAccessRequest,
  changeMachineProfile,
  startMachine,
  stopMachine,
  updateMachineOptions,
  updateMachineTags,
} from '@/lib/api'
import type { MachineAccessRequest } from '@/gen/arca/v1/sharing_pb'
import { messageFromError } from '@/lib/errors'
import { usePolling } from '@/hooks/use-polling'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { Skeleton } from '@/components/ui/skeleton'
import { ListSkeleton } from '@/components/ListSkeleton'
import type { Machine, MachineEvent, MachineProfileSummary, User } from '@/lib/types'

type MachineDetailPageProps = {
  user: User | null
  baseDomain?: string
  domainPrefix?: string
}

function machineHostname(prefix: string, machineName: string, baseDomain: string): string {
  return `${prefix}${machineName}.${baseDomain}`
}

const pollingIntervalMs = 60000
const activePollingIntervalMs = 5000
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
  const [profiles, setProfiles] = useState<MachineProfileSummary[]>([])
  const [editingMachineType, setEditingMachineType] = useState(false)
  const [editMachineType, setEditMachineType] = useState('')
  const [savingMachineType, setSavingMachineType] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [actionError, setActionError] = useState('')
  const [confirmAction, setConfirmAction] = useState<{ title: string; description: string; confirmLabel: string; variant: 'default' | 'destructive'; onConfirm: () => void } | null>(null)
  const [sharingOpen, setSharingOpen] = useState(false)
  const [createImageOpen, setCreateImageOpen] = useState(false)
  const [accessRequests, setAccessRequests] = useState<MachineAccessRequest[]>([])
  const [editingTags, setEditingTags] = useState(false)
  const [editTagsInput, setEditTagsInput] = useState('')
  const [savingTags, setSavingTags] = useState(false)
  const [changingProfile, setChangingProfile] = useState(false)
  const [selectedProfileId, setSelectedProfileId] = useState('')
  const [savingProfile, setSavingProfile] = useState(false)
  const endpointURL = machine == null || machine.name === '' || baseDomain === '' ? null : `https://${machineHostname(domainPrefix, machine.name, baseDomain)}`
  const ttydURL = endpointURL != null ? `${endpointURL}/__arca/ttyd` : null
  const shelleyURL = endpointURL != null ? `${endpointURL}/__arca/shelley` : null
  const isRunning = machine?.status === 'running'
  const isAdmin = machine?.userRole === 'admin'
  const isEditor = machine?.userRole === 'editor'
  const isOwner = machine?.userRole === 'owner'
  const isTransitioning = machine?.status === 'starting' || machine?.status === 'stopping' || machine?.status === 'pending' || machine?.status === 'deleting'
  const hasLockedOperation = machine?.lockedOperation != null && machine.lockedOperation !== ''
  const isImaging = machine?.lockedOperation === 'create_image'

  const sortedEvents = useMemo(() => {
    return [...events].sort((a, b) => Number(b.createdAt) - Number(a.createdAt))
  }, [events])

  const pollingEnabled = user != null && machineID != null && machineID !== ''
  const currentPollingInterval = hasLockedOperation || isTransitioning ? activePollingIntervalMs : pollingIntervalMs

  const { loading, error: pollingError } = usePolling(
    useCallback(async () => {
      if (machineID == null || machineID === '') return
      const [item, eventItems, , profileItems] = await Promise.all([
        getMachine(machineID, { timeoutMs: pollingRequestTimeoutMs }),
        listMachineEvents(machineID, eventLimit, { timeoutMs: pollingRequestTimeoutMs }),
        listMachineExposures(machineID),
        listAvailableProfiles(),
      ])
      setMachine(item)
      setEvents(eventItems)
      setProfiles(profileItems)
      // Fetch access requests for admins
      if (item.userRole === 'admin') {
        try {
          const reqs = await listMachineAccessRequests(machineID)
          setAccessRequests(reqs)
        } catch {
          // ignore errors for access requests polling
        }
      }
    }, [machineID]),
    { intervalMs: currentPollingInterval, enabled: pollingEnabled },
  )

  const error = actionError || pollingError

  if (user == null) {
    return <Navigate to="/login" replace />
  }
  if (machineID == null || machineID === '') {
    return <Navigate to="/machines" replace />
  }

  const handleStart = async () => {
    setActionError('')
    try {
      const updated = await startMachine(machineID)
      setMachine(updated)
    } catch (e) {
      setActionError(messageFromError(e))
    }
  }

  const handleStop = () => {
    setConfirmAction({
      title: 'Stop machine',
      description: 'Are you sure you want to stop this machine?',
      confirmLabel: 'Stop',
      variant: 'default',
      onConfirm: async () => {
        setConfirmAction(null)
        setActionError('')
        try {
          const updated = await stopMachine(machineID)
          setMachine(updated)
        } catch (e) {
          setActionError(messageFromError(e))
        }
      },
    })
  }

  const handleRestart = () => {
    setConfirmAction({
      title: 'Restart machine',
      description: 'Are you sure you want to restart this machine?',
      confirmLabel: 'Restart',
      variant: 'default',
      onConfirm: async () => {
        setConfirmAction(null)
        setActionError('')
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
          setActionError(messageFromError(e))
        }
      },
    })
  }

  const handleDelete = () => {
    setConfirmAction({
      title: 'Delete machine',
      description: 'Delete this machine? This action cannot be undone.',
      confirmLabel: 'Delete',
      variant: 'destructive',
      onConfirm: async () => {
        setConfirmAction(null)
        setActionError('')
        setDeleting(true)
        try {
          await deleteMachine(machineID)
          await navigate('/machines')
        } catch (e) {
          setActionError(messageFromError(e))
          setDeleting(false)
        }
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
              {loading ? 'Loading...' : machine?.name ?? 'Machine not found'}
            </h1>
            {machine != null && (
              <div className="mt-2 flex items-center gap-3">
                <StatusBadge status={machine.status} />
                {isImaging && (
                  <span className="inline-flex items-center gap-1.5 rounded-full border border-violet-400/40 bg-violet-500/15 px-2 py-0.5 text-xs font-medium uppercase tracking-[0.08em] text-violet-200">
                    <Camera className="h-3 w-3 animate-pulse" />
                    Creating Image...
                  </span>
                )}
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
            {/* Restart-needed banner */}
            {machine.restartNeeded && isAdmin && (
              <div className="flex items-center justify-between rounded-lg border border-amber-400/30 bg-amber-500/12 px-4 py-3">
                <p className="text-sm text-amber-200">Profile updated. Changes will apply on next restart.</p>
                <Button
                  type="button"
                  variant="secondary"
                  size="sm"
                  onClick={() => void handleRestart()}
                  disabled={isTransitioning}
                >
                  Restart
                </Button>
              </div>
            )}

            {/* Machine information card */}
            <Card className="py-0 shadow-sm">
              <CardHeader className="space-y-2 p-6 pb-3">
                <CardTitle className="text-xl">Information</CardTitle>
              </CardHeader>
              <CardContent className="space-y-4 p-6 pt-3">
                {isAdmin && (
                  <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-4">
                    <p className="text-sm text-muted-foreground">Profile</p>
                    {changingProfile ? (
                      <div className="space-y-2">
                        <select
                          value={selectedProfileId}
                          onChange={(e) => setSelectedProfileId(e.target.value)}
                          className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm text-foreground"
                          disabled={savingProfile}
                        >
                          <option value="">Select a profile...</option>
                          {profiles
                            .filter((p) => !machine.providerType || p.type === machine.providerType)
                            .map((p) => (
                              <option key={p.id} value={p.id}>{p.name}</option>
                            ))}
                        </select>
                        <div className="flex items-center gap-2">
                          <Button
                            type="button"
                            size="sm"
                            disabled={savingProfile || selectedProfileId === '' || selectedProfileId === machine.profileId}
                            onClick={() => {
                              const doChange = async () => {
                                setSavingProfile(true)
                                setActionError('')
                                try {
                                  const updated = await changeMachineProfile(machineID, selectedProfileId)
                                  setMachine(updated)
                                  setChangingProfile(false)
                                } catch (e) {
                                  setActionError(messageFromError(e))
                                } finally {
                                  setSavingProfile(false)
                                }
                              }
                              void doChange()
                            }}
                          >
                            {savingProfile ? 'Saving...' : 'Save'}
                          </Button>
                          <Button type="button" variant="secondary" size="sm" disabled={savingProfile} onClick={() => setChangingProfile(false)}>
                            Cancel
                          </Button>
                        </div>
                      </div>
                    ) : (
                      <div className="flex items-center gap-2">
                        {machine.profileId === '' ? (
                          <p className="text-sm text-foreground">Unassigned</p>
                        ) : (
                          <Link
                            to={`/machine-profiles/${machine.profileId}`}
                            className="text-sm text-sky-300 underline decoration-sky-300/50 underline-offset-2 transition hover:text-sky-200"
                          >
                            {profiles.find((r) => r.id === machine.profileId)?.name ?? machine.profileId}
                          </Link>
                        )}
                        {machine.status === 'stopped' && (
                          <Button
                            type="button"
                            variant="secondary"
                            size="sm"
                            className="h-7 px-2 text-xs"
                            onClick={() => {
                              setSelectedProfileId(machine.profileId)
                              setChangingProfile(true)
                            }}
                          >
                            Change
                          </Button>
                        )}
                      </div>
                    )}
                    {machine.providerType && (
                      <p className="text-xs text-muted-foreground">Provider: {machine.providerType}</p>
                    )}
                  </div>
                )}
                <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-4">
                  <p className="text-sm text-muted-foreground">Tags</p>
                  {editingTags ? (
                    <div className="space-y-2">
                      <input
                        type="text"
                        value={editTagsInput}
                        onChange={(e) => setEditTagsInput(e.target.value)}
                        className="h-9 w-full rounded-md border border-input bg-background px-3 text-sm text-foreground"
                        disabled={savingTags}
                        placeholder="tag1, tag2, tag3"
                      />
                      <p className="text-xs text-muted-foreground">Comma-separated. Lowercase alphanumeric and hyphens only. Max 10 tags.</p>
                      <div className="flex items-center gap-2">
                        <Button
                          type="button"
                          size="sm"
                          disabled={savingTags}
                          onClick={() => {
                            const doSave = async () => {
                              setSavingTags(true)
                              setActionError('')
                              try {
                                const tags = editTagsInput.split(',').map((t) => t.trim()).filter((t) => t !== '')
                                const updated = await updateMachineTags(machineID, tags)
                                setMachine(updated)
                                setEditingTags(false)
                              } catch (e) {
                                setActionError(messageFromError(e))
                              } finally {
                                setSavingTags(false)
                              }
                            }
                            void doSave()
                          }}
                        >
                          {savingTags ? 'Saving...' : 'Save'}
                        </Button>
                        <Button type="button" variant="secondary" size="sm" disabled={savingTags} onClick={() => setEditingTags(false)}>
                          Cancel
                        </Button>
                      </div>
                    </div>
                  ) : (
                    <div className="flex flex-wrap items-center gap-2">
                      {machine.tags.length === 0 ? (
                        <p className="text-sm text-muted-foreground">No tags</p>
                      ) : (
                        machine.tags.map((tag) => (
                          <span key={tag} className="inline-flex items-center rounded-full border border-cyan-400/30 bg-cyan-500/10 px-2 py-0.5 text-xs font-medium text-cyan-200">{tag}</span>
                        ))
                      )}
                      {isAdmin && (
                        <Button type="button" variant="secondary" size="sm" className="h-7 px-2 text-xs" onClick={() => { setEditTagsInput(machine.tags.join(', ')); setEditingTags(true) }}>
                          Edit
                        </Button>
                      )}
                    </div>
                  )}
                </div>
                {(() => {
                  const rt = profiles.find((r) => r.id === machine.profileId)
                  if (rt == null || rt.type !== 'gce') return null
                  const currentMT = machine.options?.machine_type || (rt.allowedMachineTypes ?? [])[0] || 'e2-standard-2'
                  const isStopped = machine.status === 'stopped'
                  const allowed = rt.allowedMachineTypes ?? []

                  const handleSaveMachineType = async () => {
                    setSavingMachineType(true)
                    setActionError('')
                    try {
                      const updated = await updateMachineOptions(machineID, { machine_type: editMachineType.trim() })
                      setMachine(updated)
                      setEditingMachineType(false)
                    } catch (e) {
                      setActionError(messageFromError(e))
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
                {/* Configuration with source labels (admin only) */}
                {isAdmin && machine.profileId !== '' && (() => {
                  let infraConfig: Record<string, unknown> = {}
                  try {
                    infraConfig = machine.infrastructureConfigJson ? JSON.parse(machine.infrastructureConfigJson) : {}
                  } catch { /* ignore */ }
                  const providerConfig = (infraConfig.libvirt ?? infraConfig.gce ?? infraConfig.lxd ?? {}) as Record<string, unknown>

                  type ConfigEntry = { label: string; value: string; source: 'profile' | 'machine' | 'fixed' }
                  const entries: ConfigEntry[] = []

                  // Machine-level (per-machine)
                  if (machine.providerType === 'gce' && machine.options?.['machine_type']) {
                    entries.push({ label: 'Machine type', value: machine.options['machine_type'], source: 'machine' })
                  }

                  // Fixed (frozen at creation from infrastructure config snapshot)
                  if (machine.providerType === 'gce') {
                    if (providerConfig.project) entries.push({ label: 'Project', value: String(providerConfig.project), source: 'fixed' })
                    if (providerConfig.zone) entries.push({ label: 'Zone', value: String(providerConfig.zone), source: 'fixed' })
                    if (providerConfig.network) entries.push({ label: 'Network', value: String(providerConfig.network), source: 'fixed' })
                  } else if (machine.providerType === 'lxd') {
                    if (providerConfig.endpoint) entries.push({ label: 'Endpoint', value: String(providerConfig.endpoint), source: 'fixed' })
                  } else if (machine.providerType === 'libvirt') {
                    if (providerConfig.uri) entries.push({ label: 'URI', value: String(providerConfig.uri), source: 'fixed' })
                    if (providerConfig.network) entries.push({ label: 'Network', value: String(providerConfig.network), source: 'fixed' })
                  }

                  // Profile-sourced dynamic settings (read live from profile on each start)
                  const profileName = profiles.find((r) => r.id === machine.profileId)?.name
                  const profileLabel = profileName ? `From profile "${profileName}"` : 'From profile'
                  entries.push({ label: 'Startup script', value: profileLabel, source: 'profile' })
                  entries.push({ label: 'Auto-stop timeout', value: profileLabel, source: 'profile' })
                  entries.push({ label: 'Server API URL', value: profileLabel, source: 'profile' })

                  if (entries.length === 0) return null

                  const sourceBadge = (source: 'profile' | 'machine' | 'fixed') => {
                    switch (source) {
                      case 'profile':
                        return <span className="inline-flex items-center rounded-full border border-sky-400/30 bg-sky-500/10 px-1.5 py-0.5 text-[10px] font-medium text-sky-300">Profile</span>
                      case 'machine':
                        return <span className="inline-flex items-center rounded-full border border-violet-400/30 bg-violet-500/10 px-1.5 py-0.5 text-[10px] font-medium text-violet-300">Machine</span>
                      case 'fixed':
                        return <span className="inline-flex items-center rounded-full border border-slate-400/30 bg-slate-500/10 px-1.5 py-0.5 text-[10px] font-medium text-slate-300">Fixed</span>
                    }
                  }

                  return (
                    <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-4">
                      <p className="text-sm text-muted-foreground">Configuration</p>
                      <div className="space-y-1.5">
                        {entries.map((entry) => (
                          <div key={entry.label} className="flex items-center justify-between gap-2">
                            <div className="flex items-center gap-2 min-w-0">
                              <span className="text-xs text-muted-foreground shrink-0">{entry.label}</span>
                              {sourceBadge(entry.source)}
                            </div>
                            <span className="text-xs font-medium text-foreground truncate text-right">{entry.value}</span>
                          </div>
                        ))}
                      </div>
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

            {/* Lifecycle controls card (admin/owner) */}
            {(isAdmin || isOwner) && (
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
                      disabled={hasLockedOperation || (machine.desiredStatus === 'running' && machine.status !== 'failed')}
                    >
                      Start
                    </Button>
                    <Button
                      type="button"
                      variant="secondary"
                      className="h-9 px-3"
                      onClick={() => void handleStop()}
                      disabled={hasLockedOperation || (machine.desiredStatus === 'stopped' && machine.status !== 'failed')}
                    >
                      Stop
                    </Button>
                    <Button
                      type="button"
                      variant="secondary"
                      className="h-9 px-3"
                      onClick={() => void handleRestart()}
                      disabled={hasLockedOperation || isTransitioning}
                    >
                      {machine.updateRequired && !isTransitioning ? 'Restart to update' : 'Restart'}
                    </Button>
                  </div>
                  <div className="flex items-center gap-2 border-t border-border pt-4">
                    <Button
                      type="button"
                      variant="secondary"
                      className="h-9 px-3"
                      onClick={() => setCreateImageOpen(true)}
                      disabled={!isRunning || hasLockedOperation}
                    >
                      <Camera className="mr-1.5 h-4 w-4" />
                      Create image
                    </Button>
                  </div>
                  <div className="border-t border-border pt-4">
                    <Button
                      type="button"
                      variant="destructive"
                      className="h-9 px-3"
                      onClick={() => void handleDelete()}
                      disabled={hasLockedOperation || deleting}
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
          <div className="space-y-4">
            <Skeleton className="h-8 w-48" />
            <ListSkeleton count={2} />
          </div>
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

      {(isAdmin || isOwner) && machine != null && (
        <CreateImageDialog
          machineId={machineID}
          machineName={machine.name}
          open={createImageOpen}
          onOpenChange={setCreateImageOpen}
          onSuccess={() => {
            // Trigger a refresh by re-fetching machine state
            void getMachine(machineID).then(setMachine)
          }}
        />
      )}

      <ConfirmDialog
        open={confirmAction !== null}
        onOpenChange={(open) => { if (!open) setConfirmAction(null) }}
        title={confirmAction?.title ?? ''}
        description={confirmAction?.description ?? ''}
        confirmLabel={confirmAction?.confirmLabel}
        variant={confirmAction?.variant}
        onConfirm={() => { if (confirmAction) void confirmAction.onConfirm() }}
      />
    </main>
  )
}
