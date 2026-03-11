import { useEffect, useState } from 'react'
import { Link, Navigate, useParams } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { listAvailableRuntimes, listRuntimes } from '@/lib/api'
import { messageFromError } from '@/lib/errors'
import type { RuntimeCatalogItem, RuntimeSummary, User } from '@/lib/types'

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
  const isAdmin = user?.role === 'admin'
  const [runtime, setRuntime] = useState<RuntimeCatalogItem | null>(null)
  const [summary, setSummary] = useState<RuntimeSummary | null>(null)
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
        if (isAdmin) {
          const items = await listRuntimes()
          if (cancelled) return
          setRuntime(items.find((item) => item.id === runtimeID) ?? null)
        } else {
          const items = await listAvailableRuntimes()
          if (cancelled) return
          setSummary(items.find((item) => item.id === runtimeID) ?? null)
        }
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
  }, [user, runtimeID, isAdmin])

  if (user == null) {
    return <Navigate to="/login" replace />
  }
  if (runtimeID == null || runtimeID === '') {
    return <Navigate to={isAdmin ? '/runtimes' : '/machines'} replace />
  }

  return (
    <main className="min-h-dvh px-6 py-10">
      <section className="mx-auto w-full max-w-3xl space-y-6">
        <header className="flex flex-col items-start justify-between gap-4 rounded-xl border border-border bg-muted/30 p-6 md:flex-row md:items-center">
          <div>
            <p className="text-xs font-medium uppercase tracking-[0.24em] text-muted-foreground">Arca</p>
            <h1 className="mt-2 text-2xl font-semibold text-foreground">Runtime detail</h1>
            <p className="mt-1 text-xs text-muted-foreground">{runtimeID}</p>
          </div>
          <div className="flex items-center gap-3">
            <Button asChild type="button" variant="secondary">
              <Link to={isAdmin ? '/runtimes' : '/machines'}>Back</Link>
            </Button>
            <Button type="button" variant="secondary" onClick={onLogout}>
              Logout
            </Button>
          </div>
        </header>

        <Card className="py-0 shadow-sm">
          <CardHeader className="space-y-2 p-6 pb-3">
            <CardTitle className="text-xl">Runtime metadata</CardTitle>
            <CardDescription>Configuration and timestamps for this runtime entry.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4 p-6 pt-3">
            {loading ? (
              <p className="text-sm text-muted-foreground">Loading...</p>
            ) : isAdmin && runtime != null ? (
              <>
                <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-4">
                  <p className="text-sm text-muted-foreground">Name</p>
                  <p className="text-lg font-semibold text-foreground">{runtime.name}</p>
                </div>
                <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-4">
                  <p className="text-sm text-muted-foreground">Type</p>
                  <p className="text-sm text-foreground">{runtime.type}</p>
                </div>
                <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-4">
                  <p className="text-sm text-muted-foreground">Created</p>
                  <p className="text-sm text-foreground">{formatUnix(runtime.createdAt)}</p>
                </div>
                <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-4">
                  <p className="text-sm text-muted-foreground">Updated</p>
                  <p className="text-sm text-foreground">{formatUnix(runtime.updatedAt)}</p>
                </div>
                <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-4">
                  <p className="text-sm text-muted-foreground">Machine exposure</p>
                  <p className="text-sm text-foreground">Method: {runtime.exposure.method === 'proxy_via_server' ? 'proxy via server' : 'cloudflare tunnel'}</p>
                  {runtime.exposure.domainPrefix !== '' && <p className="text-sm text-foreground">Domain prefix: {runtime.exposure.domainPrefix}</p>}
                  {runtime.exposure.baseDomain !== '' && <p className="text-sm text-foreground">Base domain: {runtime.exposure.baseDomain}</p>}
                  {runtime.exposure.connectivity !== '' && <p className="text-sm text-foreground">Connectivity: {runtime.exposure.connectivity}</p>}
                </div>
                {runtime.config.type === 'libvirt' ? (
                  <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-4">
                    <p className="text-sm text-muted-foreground">Config</p>
                    <p className="text-sm text-foreground">URI: {runtime.config.uri}</p>
                    <p className="text-sm text-foreground">Network: {runtime.config.network}</p>
                    <p className="text-sm text-foreground">Storage pool: {runtime.config.storagePool}</p>
                  </div>
                ) : runtime.config.type === 'lxd' ? (
                  <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-4">
                    <p className="text-sm text-muted-foreground">Config</p>
                    <p className="text-sm text-foreground">Endpoint: {runtime.config.endpoint}</p>
                  </div>
                ) : (
                  <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-4">
                    <p className="text-sm text-muted-foreground">Config</p>
                    <p className="text-sm text-foreground">Project: {runtime.config.project}</p>
                    <p className="text-sm text-foreground">Zone: {runtime.config.zone}</p>
                    <p className="text-sm text-foreground">Network: {runtime.config.network}</p>
                    <p className="text-sm text-foreground">Subnetwork: {runtime.config.subnetwork}</p>
                    <p className="text-sm text-foreground">Service account email: {runtime.config.serviceAccountEmail}</p>
                  </div>
                )}
              </>
            ) : !isAdmin && summary != null ? (
              <>
                <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-4">
                  <p className="text-sm text-muted-foreground">Name</p>
                  <p className="text-lg font-semibold text-foreground">{summary.name}</p>
                </div>
                <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-4">
                  <p className="text-sm text-muted-foreground">Type</p>
                  <p className="text-sm text-foreground">{summary.type}</p>
                </div>
              </>
            ) : (
              <p className="text-sm text-muted-foreground">Runtime not found.</p>
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
