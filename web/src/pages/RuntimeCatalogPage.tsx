import { useEffect, useMemo, useState } from 'react'
import { Link, Navigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { createRuntime, deleteRuntime, listRuntimes, updateRuntime } from '@/lib/api'
import { messageFromError } from '@/lib/errors'
import type { RuntimeCatalogConfig, RuntimeCatalogItem, RuntimeCatalogType, User } from '@/lib/types'

type RuntimeCatalogPageProps = {
  user: User | null
  onLogout: () => Promise<void>
}

type RuntimeFormState = {
  id: string
  name: string
  type: RuntimeCatalogType
  libvirtURI: string
  libvirtNetwork: string
  libvirtStoragePool: string
  gceProject: string
  gceZone: string
  gceNetwork: string
  gceSubnetwork: string
  gceServiceAccountEmail: string
}

const runtimeNamePattern = /^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/

function emptyRuntimeForm(): RuntimeFormState {
  return {
    id: '',
    name: '',
    type: 'libvirt',
    libvirtURI: '',
    libvirtNetwork: '',
    libvirtStoragePool: '',
    gceProject: '',
    gceZone: '',
    gceNetwork: '',
    gceSubnetwork: '',
    gceServiceAccountEmail: '',
  }
}

function validateRuntimeForm(form: RuntimeFormState): string | null {
  const name = form.name.trim().toLowerCase()
  if (name === '') {
    return 'Name is required.'
  }
  if (name.length < 3) {
    return 'Name must be at least 3 characters.'
  }
  if (name.length > 63) {
    return 'Name must be 63 characters or less.'
  }
  if (!runtimeNamePattern.test(name)) {
    return 'Name must use lowercase letters, digits, and hyphens only.'
  }

  if (form.type === 'gce') {
    if (
      form.gceProject.trim() === '' ||
      form.gceZone.trim() === '' ||
      form.gceNetwork.trim() === '' ||
      form.gceSubnetwork.trim() === '' ||
      form.gceServiceAccountEmail.trim() === ''
    ) {
      return 'GCE config requires project, zone, network, subnetwork, and service account email.'
    }
    return null
  }

  if (form.libvirtURI.trim() === '' || form.libvirtNetwork.trim() === '' || form.libvirtStoragePool.trim() === '') {
    return 'Libvirt config requires URI, network, and storage pool.'
  }
  return null
}

function toConfig(form: RuntimeFormState): RuntimeCatalogConfig {
  if (form.type === 'gce') {
    return {
      type: 'gce',
      project: form.gceProject.trim(),
      zone: form.gceZone.trim(),
      network: form.gceNetwork.trim(),
      subnetwork: form.gceSubnetwork.trim(),
      serviceAccountEmail: form.gceServiceAccountEmail.trim(),
    }
  }
  return {
    type: 'libvirt',
    uri: form.libvirtURI.trim(),
    network: form.libvirtNetwork.trim(),
    storagePool: form.libvirtStoragePool.trim(),
  }
}

function fillFormFromRuntime(runtime: RuntimeCatalogItem): RuntimeFormState {
  if (runtime.type === 'gce') {
    return {
      ...emptyRuntimeForm(),
      id: runtime.id,
      name: runtime.name,
      type: 'gce',
      gceProject: runtime.config.project,
      gceZone: runtime.config.zone,
      gceNetwork: runtime.config.network,
      gceSubnetwork: runtime.config.subnetwork,
      gceServiceAccountEmail: runtime.config.serviceAccountEmail,
    }
  }
  return {
    ...emptyRuntimeForm(),
    id: runtime.id,
    name: runtime.name,
    type: 'libvirt',
    libvirtURI: runtime.config.uri,
    libvirtNetwork: runtime.config.network,
    libvirtStoragePool: runtime.config.storagePool,
  }
}

function formatUnix(unix: number): string {
  if (unix <= 0) {
    return 'unknown'
  }
  return new Date(unix * 1000).toLocaleString()
}

export function RuntimeCatalogPage({ user, onLogout }: RuntimeCatalogPageProps) {
  const [runtimes, setRuntimes] = useState<RuntimeCatalogItem[]>([])
  const [form, setForm] = useState<RuntimeFormState>(emptyRuntimeForm())
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [deletingID, setDeletingID] = useState('')
  const [error, setError] = useState('')
  const [success, setSuccess] = useState('')

  const validationError = useMemo(() => validateRuntimeForm(form), [form])

  useEffect(() => {
    const run = async () => {
      setLoading(true)
      setError('')
      try {
        setRuntimes(await listRuntimes())
      } catch (err) {
        setError(messageFromError(err))
      } finally {
        setLoading(false)
      }
    }
    void run()
  }, [])

  if (user == null) {
    return <Navigate to="/login" replace />
  }

  const reloadRuntimes = async () => {
    setRuntimes(await listRuntimes())
  }

  const resetForm = () => {
    setForm(emptyRuntimeForm())
  }

  const submit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (validationError != null) {
      setError(validationError)
      return
    }

    setError('')
    setSuccess('')
    setSaving(true)
    try {
      const runtimeName = form.name.trim().toLowerCase()
      const config = toConfig(form)
      if (form.id === '') {
        await createRuntime(runtimeName, form.type, config)
        setSuccess('Runtime created.')
      } else {
        await updateRuntime(form.id, runtimeName, form.type, config)
        setSuccess('Runtime updated.')
      }
      resetForm()
      await reloadRuntimes()
    } catch (err) {
      setError(messageFromError(err))
    } finally {
      setSaving(false)
    }
  }

  const removeRuntime = async (runtimeID: string) => {
    if (!window.confirm('Delete this runtime?')) {
      return
    }
    setDeletingID(runtimeID)
    setError('')
    setSuccess('')
    try {
      await deleteRuntime(runtimeID)
      if (form.id === runtimeID) {
        resetForm()
      }
      await reloadRuntimes()
      setSuccess('Runtime deleted.')
    } catch (err) {
      setError(messageFromError(err))
    } finally {
      setDeletingID('')
    }
  }

  return (
    <main className="relative min-h-dvh overflow-hidden bg-slate-950 px-6 py-16 text-slate-100">
      <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_20%_20%,_rgba(56,189,248,0.12),_transparent_38%),radial-gradient(circle_at_80%_0%,_rgba(148,163,184,0.2),_transparent_48%)]" />
      <div className="pointer-events-none absolute inset-0 bg-[linear-gradient(rgba(255,255,255,0.04)_1px,transparent_1px),linear-gradient(90deg,rgba(255,255,255,0.04)_1px,transparent_1px)] bg-[size:48px_48px] [mask-image:radial-gradient(ellipse_at_center,black_35%,transparent_75%)]" />

      <section className="relative z-10 mx-auto w-full max-w-4xl space-y-6">
        <header className="flex flex-col items-start justify-between gap-4 rounded-2xl border border-white/10 bg-white/[0.03] p-6 backdrop-blur md:flex-row md:items-center">
          <div>
            <p className="text-xs font-medium uppercase tracking-[0.24em] text-slate-400">Arca</p>
            <h1 className="mt-2 text-2xl font-semibold text-white">Runtimes</h1>
            <p className="mt-1 text-sm text-slate-300">Manage runtime catalog entries and type-specific configuration.</p>
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
            <CardTitle className="text-xl text-white">{form.id === '' ? 'Create runtime' : 'Edit runtime'}</CardTitle>
            <CardDescription className="text-slate-300">
              Runtime IDs are generated automatically. Names must be unique.
            </CardDescription>
          </CardHeader>
          <CardContent className="p-6 pt-3">
            <form className="space-y-4" onSubmit={submit}>
              <div className="space-y-2">
                <Label htmlFor="runtime-name" className="text-slate-200">Name</Label>
                <Input
                  id="runtime-name"
                  value={form.name}
                  onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))}
                  className="h-10 border-white/20 bg-white/10 text-slate-100 placeholder:text-slate-400 focus-visible:ring-sky-400/45"
                  placeholder="edge-libvirt"
                  required
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="runtime-type" className="text-slate-200">Type</Label>
                <select
                  id="runtime-type"
                  value={form.type}
                  onChange={(event) =>
                    setForm((current) => ({ ...current, type: event.target.value === 'gce' ? 'gce' : 'libvirt' }))
                  }
                  className="h-10 w-full rounded-md border border-white/20 bg-white/10 px-3 text-sm text-slate-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/45"
                >
                  <option value="libvirt">Libvirt</option>
                  <option value="gce">Google Compute Engine (GCE)</option>
                </select>
              </div>

              {form.type === 'gce' ? (
                <div className="space-y-4 rounded-md border border-white/10 bg-white/[0.03] p-4">
                  <div className="space-y-2">
                    <Label htmlFor="runtime-gce-project" className="text-slate-200">Project</Label>
                    <Input id="runtime-gce-project" value={form.gceProject} onChange={(event) => setForm((current) => ({ ...current, gceProject: event.target.value }))} className="h-10 border-white/20 bg-white/10 text-slate-100 placeholder:text-slate-400 focus-visible:ring-sky-400/45" />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="runtime-gce-zone" className="text-slate-200">Zone</Label>
                    <Input id="runtime-gce-zone" value={form.gceZone} onChange={(event) => setForm((current) => ({ ...current, gceZone: event.target.value }))} className="h-10 border-white/20 bg-white/10 text-slate-100 placeholder:text-slate-400 focus-visible:ring-sky-400/45" />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="runtime-gce-network" className="text-slate-200">Network</Label>
                    <Input id="runtime-gce-network" value={form.gceNetwork} onChange={(event) => setForm((current) => ({ ...current, gceNetwork: event.target.value }))} className="h-10 border-white/20 bg-white/10 text-slate-100 placeholder:text-slate-400 focus-visible:ring-sky-400/45" />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="runtime-gce-subnetwork" className="text-slate-200">Subnetwork</Label>
                    <Input id="runtime-gce-subnetwork" value={form.gceSubnetwork} onChange={(event) => setForm((current) => ({ ...current, gceSubnetwork: event.target.value }))} className="h-10 border-white/20 bg-white/10 text-slate-100 placeholder:text-slate-400 focus-visible:ring-sky-400/45" />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="runtime-gce-service-account-email" className="text-slate-200">Service account email</Label>
                    <Input id="runtime-gce-service-account-email" value={form.gceServiceAccountEmail} onChange={(event) => setForm((current) => ({ ...current, gceServiceAccountEmail: event.target.value }))} className="h-10 border-white/20 bg-white/10 text-slate-100 placeholder:text-slate-400 focus-visible:ring-sky-400/45" />
                  </div>
                </div>
              ) : (
                <div className="space-y-4 rounded-md border border-white/10 bg-white/[0.03] p-4">
                  <div className="space-y-2">
                    <Label htmlFor="runtime-libvirt-uri" className="text-slate-200">URI</Label>
                    <Input id="runtime-libvirt-uri" value={form.libvirtURI} onChange={(event) => setForm((current) => ({ ...current, libvirtURI: event.target.value }))} className="h-10 border-white/20 bg-white/10 text-slate-100 placeholder:text-slate-400 focus-visible:ring-sky-400/45" placeholder="qemu:///system" />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="runtime-libvirt-network" className="text-slate-200">Network</Label>
                    <Input id="runtime-libvirt-network" value={form.libvirtNetwork} onChange={(event) => setForm((current) => ({ ...current, libvirtNetwork: event.target.value }))} className="h-10 border-white/20 bg-white/10 text-slate-100 placeholder:text-slate-400 focus-visible:ring-sky-400/45" placeholder="default" />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="runtime-libvirt-storage-pool" className="text-slate-200">Storage pool</Label>
                    <Input id="runtime-libvirt-storage-pool" value={form.libvirtStoragePool} onChange={(event) => setForm((current) => ({ ...current, libvirtStoragePool: event.target.value }))} className="h-10 border-white/20 bg-white/10 text-slate-100 placeholder:text-slate-400 focus-visible:ring-sky-400/45" placeholder="default" />
                  </div>
                </div>
              )}

              <div className="flex items-center gap-3">
                <Button type="submit" className="h-10 bg-white px-5 text-slate-900 hover:bg-slate-100" disabled={saving || validationError != null}>
                  {saving ? 'Saving...' : form.id === '' ? 'Create runtime' : 'Save runtime'}
                </Button>
                {form.id !== '' && (
                  <Button type="button" variant="secondary" onClick={resetForm}>
                    Cancel editing
                  </Button>
                )}
              </div>
            </form>
          </CardContent>
        </Card>

        <Card className="border-white/15 bg-white/[0.04] py-0 shadow-2xl shadow-black/35 backdrop-blur-xl">
          <CardHeader className="space-y-2 p-6 pb-3">
            <CardTitle className="text-xl text-white">Runtime catalog</CardTitle>
            <CardDescription className="text-slate-300">Edit or delete existing runtime definitions.</CardDescription>
          </CardHeader>
          <CardContent className="p-6 pt-3">
            {loading ? (
              <p className="text-sm text-slate-300">Loading runtimes...</p>
            ) : runtimes.length === 0 ? (
              <p className="text-sm text-slate-300">No runtimes configured.</p>
            ) : (
              <div className="space-y-3">
                {runtimes.map((runtime) => (
                  <div key={runtime.id} className="rounded-lg border border-white/10 bg-black/20 p-4">
                    <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
                      <div className="space-y-1">
                        <p className="text-sm font-medium text-white">{runtime.name}</p>
                        <p className="text-xs uppercase tracking-wide text-slate-400">{runtime.type}</p>
                        <p className="text-xs text-slate-400">Created {formatUnix(runtime.createdAt)}</p>
                        <p className="text-xs text-slate-400">Updated {formatUnix(runtime.updatedAt)}</p>
                      </div>
                      <div className="flex items-center gap-2">
                        <Button type="button" variant="secondary" onClick={() => setForm(fillFormFromRuntime(runtime))}>Edit</Button>
                        <Button type="button" variant="secondary" onClick={() => void removeRuntime(runtime.id)} disabled={deletingID === runtime.id}>
                          {deletingID === runtime.id ? 'Deleting...' : 'Delete'}
                        </Button>
                      </div>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>

        {validationError != null && <p className="text-sm text-amber-200">{validationError}</p>}
        {success !== '' && <p className="text-sm text-emerald-300">{success}</p>}
        {error !== '' && <p className="text-sm text-red-300">{error}</p>}
      </section>
    </main>
  )
}
