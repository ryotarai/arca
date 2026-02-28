import { useEffect, useState } from 'react'
import { Link, Navigate, useParams } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { getMachine, startMachine, stopMachine } from '@/lib/api'
import { messageFromError } from '@/lib/errors'
import type { Machine, User } from '@/lib/types'

type MachineDetailPageProps = {
  user: User | null
  onLogout: () => Promise<void>
}

const pollingRequestTimeoutMs = 2500

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

export function MachineDetailPage({ user, onLogout }: MachineDetailPageProps) {
  const { machineID } = useParams()
  const [machine, setMachine] = useState<Machine | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

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
        const item = await getMachine(machineID, { timeoutMs: pollingRequestTimeoutMs })
        if (!cancelled) {
          setMachine(item)
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
    setError('')
    try {
      const updated = await stopMachine(machineID)
      setMachine(updated)
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
            <CardDescription className="text-slate-300">Shows current and desired states.</CardDescription>
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
