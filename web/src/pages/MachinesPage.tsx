import { useEffect, useState } from 'react'
import { Link, Navigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import {
  getMachine,
  listMachines,
  startMachine,
  stopMachine,
} from '@/lib/api'
import { messageFromError } from '@/lib/errors'
import type { Machine, User } from '@/lib/types'

type MachinesPageProps = {
  user: User | null
  onLogout: () => Promise<void>
}

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

export function MachinesPage({ user, onLogout }: MachinesPageProps) {
  const [machines, setMachines] = useState<Machine[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    if (user == null) {
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
        const items = await listMachines({ timeoutMs: pollingRequestTimeoutMs })
        if (!cancelled) {
          setMachines(items)
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
  }, [user])

  if (user == null) {
    return <Navigate to="/login" replace />
  }

  const submitRestart = async (machineID: string) => {
    if (!window.confirm('Restart this machine?')) {
      return
    }

    setError('')
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
      setError(messageFromError(e))
    }
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
              <p className="text-sm text-muted-foreground">Loading...</p>
            ) : machines.length === 0 ? (
              <p className="text-sm text-muted-foreground">No machines yet.</p>
            ) : (
              <ul className="space-y-3">
                {machines.map((machine) => {
                  return (
                    <li key={machine.id} className="rounded-lg border border-border bg-muted/30 p-4">
                      <div className="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
                        <div className="space-y-2">
                          <p className="font-medium text-foreground">{machine.name}</p>
                          <p className="text-xs text-muted-foreground">runtime: {machine.runtimeId}</p>
                          <div className="mt-1 flex items-center gap-2">
                            <StatusBadge status={machine.status} />
                          </div>
                          {machine.lastError != null && machine.lastError !== '' && (
                            <p className="text-xs text-red-300 break-all">error: {machine.lastError}</p>
                          )}
                        </div>

                        <div className="flex flex-wrap items-center justify-end gap-2 sm:max-w-md">
                          {machine.updateRequired && machine.status !== 'starting' && machine.status !== 'stopping' && machine.status !== 'pending' && machine.status !== 'deleting' && (
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
    </main>
  )
}
