import { useEffect, useState } from 'react'
import { Link, Navigate, useNavigate, useParams } from 'react-router-dom'
import { Pencil, Trash2 } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { deleteMachineTemplate, listAvailableMachineTemplates, listMachineTemplates } from '@/lib/api'
import { messageFromError } from '@/lib/errors'
import type { MachineTemplateItem, MachineTemplateSummary, User } from '@/lib/types'

type MachineTemplateDetailPageProps = {
  user: User | null
  onLogout: () => Promise<void>
}

function formatUnix(unix: number): string {
  if (unix <= 0) {
    return 'unknown'
  }
  return new Date(unix * 1000).toLocaleString()
}

export function MachineTemplateDetailPage({ user, onLogout }: MachineTemplateDetailPageProps) {
  const { templateID } = useParams()
  const navigate = useNavigate()
  const isAdmin = user?.role === 'admin'
  const [template, setTemplate] = useState<MachineTemplateItem | null>(null)
  const [summary, setSummary] = useState<MachineTemplateSummary | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [deleting, setDeleting] = useState(false)
  const [confirmAction, setConfirmAction] = useState<{ title: string; description: string; confirmLabel: string; variant: 'default' | 'destructive'; onConfirm: () => void } | null>(null)

  useEffect(() => {
    if (user == null || templateID == null || templateID === '') {
      return
    }

    let cancelled = false

    const run = async () => {
      setLoading(true)
      setError('')
      try {
        if (isAdmin) {
          const items = await listMachineTemplates()
          if (cancelled) return
          setTemplate(items.find((item) => item.id === templateID) ?? null)
        } else {
          const items = await listAvailableMachineTemplates()
          if (cancelled) return
          setSummary(items.find((item) => item.id === templateID) ?? null)
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
  }, [user, templateID, isAdmin])

  if (user == null) {
    return <Navigate to="/login" replace />
  }
  if (templateID == null || templateID === '') {
    return <Navigate to={isAdmin ? '/machine-templates' : '/machines'} replace />
  }

  const handleDelete = () => {
    setConfirmAction({
      title: 'Delete template',
      description: 'Are you sure you want to delete this template?',
      confirmLabel: 'Delete',
      variant: 'destructive',
      onConfirm: () => {
        void (async () => {
          setDeleting(true)
          setError('')
          try {
            await deleteMachineTemplate(templateID)
            navigate('/machine-templates')
          } catch (err) {
            setError(messageFromError(err))
          } finally {
            setDeleting(false)
          }
        })()
      },
    })
  }

  return (
    <main className="min-h-dvh px-6 py-10">
      <section className="mx-auto w-full max-w-3xl space-y-6">
        <header className="flex flex-col items-start justify-between gap-4 rounded-xl border border-border bg-muted/30 p-6 md:flex-row md:items-center">
          <div>
            <p className="text-xs font-medium uppercase tracking-[0.24em] text-muted-foreground">Arca</p>
            <h1 className="mt-2 text-2xl font-semibold text-foreground">Template detail</h1>
            <p className="mt-1 text-xs text-muted-foreground">{templateID}</p>
          </div>
          <div className="flex items-center gap-3">
            {isAdmin && (
              <>
                <Button asChild variant="secondary">
                  <Link to={`/machine-templates/${templateID}/edit`}>
                    <Pencil className="mr-2 h-4 w-4" />
                    Edit
                  </Link>
                </Button>
                <Button variant="destructive" onClick={() => handleDelete()} disabled={deleting}>
                  <Trash2 className="mr-2 h-4 w-4" />
                  {deleting ? 'Deleting...' : 'Delete'}
                </Button>
              </>
            )}
            <Button asChild type="button" variant="secondary">
              <Link to={isAdmin ? '/machine-templates' : '/machines'}>Back</Link>
            </Button>
          </div>
        </header>

        <Card className="py-0 shadow-sm">
          <CardHeader className="space-y-2 p-6 pb-3">
            <CardTitle className="text-xl">Template metadata</CardTitle>
            <CardDescription>Configuration and timestamps for this template entry.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-4 p-6 pt-3">
            {loading ? (
              <p className="text-sm text-muted-foreground">Loading...</p>
            ) : isAdmin && template != null ? (
              <>
                <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-4">
                  <p className="text-sm text-muted-foreground">Name</p>
                  <p className="text-lg font-semibold text-foreground">{template.name}</p>
                </div>
                <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-4">
                  <p className="text-sm text-muted-foreground">Type</p>
                  <p className="text-sm text-foreground">{template.type}</p>
                </div>
                <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-4">
                  <p className="text-sm text-muted-foreground">Created</p>
                  <p className="text-sm text-foreground">{formatUnix(template.createdAt)}</p>
                </div>
                <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-4">
                  <p className="text-sm text-muted-foreground">Updated</p>
                  <p className="text-sm text-foreground">{formatUnix(template.updatedAt)}</p>
                </div>
                <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-4">
                  <p className="text-sm text-muted-foreground">Machine exposure</p>
                  <p className="text-sm text-foreground">Method: Proxy via Server</p>
                  {template.exposure.connectivity !== '' && <p className="text-sm text-foreground">Connectivity: {template.exposure.connectivity}</p>}
                </div>
                {template.config.type === 'libvirt' ? (
                  <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-4">
                    <p className="text-sm text-muted-foreground">Config</p>
                    <p className="text-sm text-foreground">URI: {template.config.uri}</p>
                    <p className="text-sm text-foreground">Network: {template.config.network}</p>
                    <p className="text-sm text-foreground">Storage pool: {template.config.storagePool}</p>
                  </div>
                ) : template.config.type === 'lxd' ? (
                  <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-4">
                    <p className="text-sm text-muted-foreground">Config</p>
                    <p className="text-sm text-foreground">Endpoint: {template.config.endpoint}</p>
                  </div>
                ) : (
                  <div className="space-y-2 rounded-lg border border-border bg-muted/30 p-4">
                    <p className="text-sm text-muted-foreground">Config</p>
                    <p className="text-sm text-foreground">Project: {template.config.project}</p>
                    <p className="text-sm text-foreground">Zone: {template.config.zone}</p>
                    <p className="text-sm text-foreground">Network: {template.config.network}</p>
                    <p className="text-sm text-foreground">Subnetwork: {template.config.subnetwork}</p>
                    <p className="text-sm text-foreground">Service account email: {template.config.serviceAccountEmail}</p>
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
              <p className="text-sm text-muted-foreground">Template not found.</p>
            )}

            {error !== '' && (
              <p role="alert" className="rounded-md border border-red-400/30 bg-red-500/12 px-3 py-2 text-sm text-red-200">
                {error}
              </p>
            )}
          </CardContent>
        </Card>
      </section>

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
