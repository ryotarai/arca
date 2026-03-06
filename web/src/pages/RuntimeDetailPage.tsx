import { useEffect, useState } from 'react'
import { Link, Navigate, useParams } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { listRuntimes } from '@/lib/api'
import { messageFromError } from '@/lib/errors'
import type { RuntimeCatalogItem, User } from '@/lib/types'

type RuntimeDetailPageProps = {
  user: User | null
  onLogout: () => Promise<void>
}

function formatUnix(unix: number): string {
  if (unix <= 0) {
    return 'unknown'
  }
  return new Date(unix * 1000).toLocaleString()
}

export function RuntimeDetailPage({ user, onLogout }: RuntimeDetailPageProps) {
  const { runtimeID } = useParams()
  const [runtime, setRuntime] = useState<RuntimeCatalogItem | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    if (user == null || runtimeID == null || runtimeID === '') {
      return
    }

    let cancelled = false

    const run = async () => {
      setLoading(true)
      setError('')
      try {
        const items = await listRuntimes()
        if (cancelled) {
          return
        }
        setRuntime(items.find((item) => item.id === runtimeID) ?? null)
      } catch (e) {
        if (!cancelled) {
          setError(messageFromError(e))
        }
      } finally {
        if (!cancelled) {
          setLoading(false)
        }
      }
    }

    void run()

    return () => {
      cancelled = true
    }
  }, [user, runtimeID])

  if (user == null) {
    return <Navigate to="/login" replace />
  }
  if (runtimeID == null || runtimeID === '') {
    return <Navigate to="/runtimes" replace />
  }

  return (
    <main className="relative min-h-dvh overflow-hidden bg-slate-950 px-6 py-16 text-slate-100">
      <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_20%_20%,_rgba(56,189,248,0.12),_transparent_38%),radial-gradient(circle_at_80%_0%,_rgba(148,163,184,0.2),_transparent_48%)]" />
      <div className="pointer-events-none absolute inset-0 bg-[linear-gradient(rgba(255,255,255,0.04)_1px,transparent_1px),linear-gradient(90deg,rgba(255,255,255,0.04)_1px,transparent_1px)] bg-[size:48px_48px] [mask-image:radial-gradient(ellipse_at_center,black_35%,transparent_75%)]" />

      <section className="relative z-10 mx-auto w-full max-w-3xl space-y-6">
        <header className="flex flex-col items-start justify-between gap-4 rounded-2xl border border-white/10 bg-white/[0.03] p-6 backdrop-blur md:flex-row md:items-center">
          <div>
            <p className="text-xs font-medium uppercase tracking-[0.24em] text-slate-400">Arca</p>
            <h1 className="mt-2 text-2xl font-semibold text-white">Runtime detail</h1>
            <p className="mt-1 text-xs text-slate-400">{runtimeID}</p>
          </div>
          <div className="flex items-center gap-3">
            <Button asChild type="button" variant="secondary">
              <Link to="/runtimes">Back</Link>
            </Button>
            <Button type="button" variant="secondary" onClick={onLogout}>
              Logout
            </Button>
          </div>
        </header>

        <Card className="border-white/15 bg-white/[0.04] py-0 shadow-2xl shadow-black/35 backdrop-blur-xl">
          <CardHeader className="space-y-2 p-6 pb-3">
            <CardTitle className="text-xl text-white">Runtime metadata</CardTitle>
            <CardDescription className="text-slate-300">Configuration and timestamps for this runtime entry.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4 p-6 pt-3">
            {loading ? (
              <p className="text-sm text-slate-300">Loading...</p>
            ) : runtime == null ? (
              <p className="text-sm text-slate-300">Runtime not found.</p>
            ) : (
              <>
                <div className="space-y-2 rounded-lg border border-white/10 bg-white/[0.03] p-4">
                  <p className="text-sm text-slate-300">Name</p>
                  <p className="text-lg font-semibold text-white">{runtime.name}</p>
                </div>
                <div className="space-y-2 rounded-lg border border-white/10 bg-white/[0.03] p-4">
                  <p className="text-sm text-slate-300">Type</p>
                  <p className="text-sm text-slate-100">{runtime.type}</p>
                </div>
                <div className="space-y-2 rounded-lg border border-white/10 bg-white/[0.03] p-4">
                  <p className="text-sm text-slate-300">Created</p>
                  <p className="text-sm text-slate-100">{formatUnix(runtime.createdAt)}</p>
                </div>
                <div className="space-y-2 rounded-lg border border-white/10 bg-white/[0.03] p-4">
                  <p className="text-sm text-slate-300">Updated</p>
                  <p className="text-sm text-slate-100">{formatUnix(runtime.updatedAt)}</p>
                </div>
                {runtime.config.type === 'libvirt' ? (
                  <div className="space-y-2 rounded-lg border border-white/10 bg-white/[0.03] p-4">
                    <p className="text-sm text-slate-300">Config</p>
                    <p className="text-sm text-slate-100">URI: {runtime.config.uri}</p>
                    <p className="text-sm text-slate-100">Network: {runtime.config.network}</p>
                    <p className="text-sm text-slate-100">Storage pool: {runtime.config.storagePool}</p>
                  </div>
                ) : (
                  <div className="space-y-2 rounded-lg border border-white/10 bg-white/[0.03] p-4">
                    <p className="text-sm text-slate-300">Config</p>
                    <p className="text-sm text-slate-100">Project: {runtime.config.project}</p>
                    <p className="text-sm text-slate-100">Zone: {runtime.config.zone}</p>
                    <p className="text-sm text-slate-100">Network: {runtime.config.network}</p>
                    <p className="text-sm text-slate-100">Subnetwork: {runtime.config.subnetwork}</p>
                    <p className="text-sm text-slate-100">Service account email: {runtime.config.serviceAccountEmail}</p>
                  </div>
                )}
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
