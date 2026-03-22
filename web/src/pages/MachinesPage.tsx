import { useCallback, useState } from 'react'
import { Link, Navigate } from 'react-router-dom'
import { Monitor, Terminal, Bot } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import {
  getMachine,
  listAvailableProfiles,
  listMachines,
  startMachine,
  stopMachine,
} from '@/lib/api'
import { messageFromError } from '@/lib/errors'
import { usePolling } from '@/hooks/use-polling'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { EmptyState } from '@/components/EmptyState'
import { ListSkeleton } from '@/components/ListSkeleton'
import type { Machine, MachineProfileSummary, User } from '@/lib/types'

type MachinesPageProps = {
  user: User | null
  onLogout: () => Promise<void>
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
    <span
      className={`inline-flex items-center rounded-full border px-2 py-0.5 text-xs font-medium uppercase tracking-[0.08em] ${statusTone(status)}`}
    >
      {status}
    </span>
  )
}

export function MachinesPage({ user, onLogout, baseDomain = '', domainPrefix = '' }: MachinesPageProps) {
  const [machines, setMachines] = useState<Machine[]>([])
  const [profiles, setProfiles] = useState<MachineProfileSummary[]>([])

  const [actionError, setActionError] = useState('')
  const [confirmAction, setConfirmAction] = useState<{ title: string; description: string; confirmLabel: string; variant: 'default' | 'destructive'; onConfirm: () => void } | null>(null)

  const { loading, error: pollingError } = usePolling(
    useCallback(async () => {
      const [items, profileItems] = await Promise.all([
        listMachines({ timeoutMs: pollingRequestTimeoutMs }),
        listAvailableProfiles(),
      ])
      setMachines(items)
      setProfiles(profileItems)
    }, []),
    { intervalMs: pollingIntervalMs, enabled: user != null },
  )

  const error = actionError || pollingError

  if (user == null) {
    return <Navigate to="/login" replace />
  }

  const submitRestart = (machineID: string) => {
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
            const machine = await getMachine(machineID)
            if (machine.status === 'stopped') {
              break
            }
            await new Promise<void>((resolve) => {
              window.setTimeout(resolve, restartWaitIntervalMs)
            })
          }

          const updated = await startMachine(machineID)
          setMachines((prev) => prev.map((machine) => (machine.id === machineID ? updated : machine)))
        } catch (e) {
          setActionError(messageFromError(e))
        }
      },
    })
  }

  return (
    <main className="min-h-dvh px-6 py-10">
      <section className="mx-auto w-full max-w-4xl space-y-6">
        <header className="flex flex-col items-start justify-between gap-4 rounded-xl border border-border bg-muted/30 p-6 md:flex-row md:items-center">
          <div>
            <p className="text-xs font-medium uppercase tracking-[0.24em] text-muted-foreground">Arca</p>
            <h1 className="mt-2 text-2xl font-semibold text-foreground">Machines</h1>
            <p className="mt-1 text-sm text-muted-foreground">Signed in as {user.email}</p>
          </div>
          <div className="flex items-center gap-3">
            <Button asChild type="button">
              <Link to="/machines/create">Create machine</Link>
            </Button>
            <Button type="button" variant="secondary" onClick={onLogout}>
              Logout
            </Button>
          </div>
        </header>

        <Card className="py-0 shadow-sm">
          <CardHeader className="space-y-2 p-6 pb-3">
            <CardTitle className="text-xl">Machine list</CardTitle>
          </CardHeader>
          <CardContent className="p-6 pt-3">
            {loading ? (
              <ListSkeleton />
            ) : machines.length === 0 ? (
              <EmptyState
                icon={<Monitor className="size-6" />}
                title="No machines yet"
                description="Create your first machine to get started with a cloud development environment."
                action={
                  <Button asChild>
                    <Link to="/machines/new">Create machine</Link>
                  </Button>
                }
              />
            ) : (
              <ul className="space-y-3">
                {machines.map((machine) => {
                  return (
                    <li key={machine.id} className="rounded-lg border border-border bg-muted/30 p-4">
                      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                        <div className="space-y-2">
                          <div className="flex flex-wrap items-center gap-2">
                            <p className="font-medium text-foreground">{machine.name}</p>
                            {machine.userRole !== '' && machine.userRole !== 'admin' && (
                              <span className="inline-flex items-center rounded-full border border-violet-400/40 bg-violet-500/15 px-2 py-0.5 text-[10px] font-medium uppercase tracking-[0.08em] text-violet-200">
                                {machine.userRole}
                              </span>
                            )}
                            {machine.tags.map((tag) => (
                              <span
                                key={tag}
                                className="inline-flex items-center rounded-full border border-cyan-400/30 bg-cyan-500/10 px-2 py-0.5 text-[10px] font-medium text-cyan-200"
                              >
                                {tag}
                              </span>
                            ))}
                          </div>
                          <p className="text-xs text-muted-foreground">profile: {profiles.find((r) => r.id === machine.profileId)?.name ?? machine.profileId}</p>
                          <div className="mt-1 flex items-center gap-2">
                            <StatusBadge status={machine.status} />
                            {machine.restartNeeded && machine.status === 'running' && (
                              <span className="inline-flex items-center rounded-full border border-amber-400/40 bg-amber-500/15 px-2 py-0.5 text-xs font-medium text-amber-200">
                                Restart needed
                              </span>
                            )}
                          </div>
                          {machine.lastError != null && machine.lastError !== '' && (
                            <p className="text-xs text-red-300 break-all">error: {machine.lastError}</p>
                          )}
                        </div>

                        <div className="flex flex-wrap items-center justify-end gap-2 sm:max-w-md">
                          {machine.status === 'running' && baseDomain !== '' && (machine.userRole === 'admin' || machine.userRole === 'editor') && (
                            <>
                              <Button asChild variant="secondary" className="h-9 px-3">
                                <a href={`https://${machineHostname(domainPrefix, machine.name, baseDomain)}/__arca/ttyd`} target="_blank" rel="noreferrer">
                                  <Terminal className="h-4 w-4" /> Terminal
                                </a>
                              </Button>
                              <Button asChild variant="secondary" className="h-9 px-3">
                                <a href={`https://${machineHostname(domainPrefix, machine.name, baseDomain)}/__arca/shelley`} target="_blank" rel="noreferrer">
                                  <Bot className="h-4 w-4" /> Shelley
                                </a>
                              </Button>
                            </>
                          )}
                          {machine.userRole === 'admin' && machine.updateRequired && machine.status !== 'starting' && machine.status !== 'stopping' && machine.status !== 'pending' && machine.status !== 'deleting' && (
                            <Button type="button" variant="secondary" className="h-9 px-3" onClick={() => void submitRestart(machine.id)}>
                              Restart to update
                            </Button>
                          )}
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
