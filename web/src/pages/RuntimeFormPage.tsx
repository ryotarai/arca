import { useEffect, useMemo, useState } from 'react'
import { Link, Navigate, useNavigate, useParams } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { createRuntime, listRuntimes, updateRuntime } from '@/lib/api'
import { messageFromError } from '@/lib/errors'
import type { MachineExposureConfig, MachineExposureMethodType, RuntimeCatalogConfig, RuntimeCatalogItem, RuntimeCatalogType, User } from '@/lib/types'

type RuntimeFormPageProps = {
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
  libvirtStartupScript: string
  gceProject: string
  gceZone: string
  gceNetwork: string
  gceSubnetwork: string
  gceServiceAccountEmail: string
  gceStartupScript: string
  gceDiskSizeGb: string
  gceAllowedMachineTypes: string
  lxdEndpoint: string
  lxdStartupScript: string
  exposureMethod: MachineExposureMethodType
  exposureDomainPrefix: string
  exposureBaseDomain: string
  exposureConnectivity: 'private_ip' | 'public_ip' | ''
  serverApiUrl: string
  autoStopTimeoutHours: string
}

const runtimeNamePattern = /^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/
const maxStartupScriptBytes = 8 * 1024

function emptyRuntimeForm(): RuntimeFormState {
  return {
    id: '',
    name: '',
    type: 'libvirt',
    libvirtURI: '',
    libvirtNetwork: '',
    libvirtStoragePool: '',
    libvirtStartupScript: '',
    gceProject: '',
    gceZone: '',
    gceNetwork: '',
    gceSubnetwork: '',
    gceServiceAccountEmail: '',
    gceStartupScript: '',
    gceDiskSizeGb: '',
    gceAllowedMachineTypes: '',
    lxdEndpoint: '',
    lxdStartupScript: '',
    exposureMethod: 'proxy_via_server',
    exposureDomainPrefix: '',
    exposureBaseDomain: '',
    exposureConnectivity: '',
    serverApiUrl: '',
    autoStopTimeoutHours: '',
  }
}

function utf8ByteLength(value: string): number {
  return new TextEncoder().encode(value).length
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
    if (utf8ByteLength(form.gceStartupScript) > maxStartupScriptBytes) {
      return 'GCE startup script must be 8KB or less.'
    }
    if (
      form.gceProject.trim() === '' ||
      form.gceZone.trim() === '' ||
      form.gceNetwork.trim() === '' ||
      form.gceSubnetwork.trim() === '' ||
      form.gceServiceAccountEmail.trim() === ''
    ) {
      return 'GCE config requires project, zone, network, subnetwork, and service account email.'
    }
    const machineTypes = form.gceAllowedMachineTypes
      .split(/[,\n]/)
      .map((s) => s.trim())
      .filter((s) => s !== '')
    if (machineTypes.length === 0) {
      return 'GCE config requires at least one machine type.'
    }
    return null
  }

  if (form.type === 'lxd') {
    if (utf8ByteLength(form.lxdStartupScript) > maxStartupScriptBytes) {
      return 'LXD startup script must be 8KB or less.'
    }
    if (form.lxdEndpoint.trim() === '') {
      return 'LXD config requires endpoint.'
    }
    return null
  }

  if (utf8ByteLength(form.libvirtStartupScript) > maxStartupScriptBytes) {
    return 'Libvirt startup script must be 8KB or less.'
  }
  if (form.libvirtURI.trim() === '' || form.libvirtNetwork.trim() === '' || form.libvirtStoragePool.trim() === '') {
    return 'Libvirt config requires URI, network, and storage pool.'
  }
  return null
}

function toConfig(form: RuntimeFormState): RuntimeCatalogConfig {
  if (form.type === 'gce') {
    const allowedMachineTypes = form.gceAllowedMachineTypes
      .split(/[,\n]/)
      .map((s) => s.trim())
      .filter((s) => s !== '')
    return {
      type: 'gce',
      project: form.gceProject.trim(),
      zone: form.gceZone.trim(),
      network: form.gceNetwork.trim(),
      subnetwork: form.gceSubnetwork.trim(),
      serviceAccountEmail: form.gceServiceAccountEmail.trim(),
      startupScript: form.gceStartupScript,
      diskSizeGb: form.gceDiskSizeGb.trim() !== '' ? parseInt(form.gceDiskSizeGb.trim(), 10) || 0 : 0,
      allowedMachineTypes,
    }
  }
  if (form.type === 'lxd') {
    return {
      type: 'lxd',
      endpoint: form.lxdEndpoint.trim(),
      startupScript: form.lxdStartupScript,
    }
  }
  return {
    type: 'libvirt',
    uri: form.libvirtURI.trim(),
    network: form.libvirtNetwork.trim(),
    storagePool: form.libvirtStoragePool.trim(),
    startupScript: form.libvirtStartupScript,
  }
}

function toExposureConfig(form: RuntimeFormState): MachineExposureConfig {
  return {
    method: form.exposureMethod,
    domainPrefix: form.exposureDomainPrefix.trim(),
    baseDomain: form.exposureBaseDomain.trim(),
    connectivity: form.exposureConnectivity,
  }
}

function fillFormFromRuntime(runtime: RuntimeCatalogItem): RuntimeFormState {
  const exposureFields = {
    exposureMethod: runtime.exposure.method,
    exposureDomainPrefix: runtime.exposure.domainPrefix,
    exposureBaseDomain: runtime.exposure.baseDomain,
    exposureConnectivity: runtime.exposure.connectivity,
    serverApiUrl: runtime.serverApiUrl,
    autoStopTimeoutHours: runtime.autoStopTimeoutSeconds > 0 ? String(runtime.autoStopTimeoutSeconds / 3600) : '',
  } as const
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
      gceStartupScript: runtime.config.startupScript,
      gceDiskSizeGb: runtime.config.diskSizeGb ? String(runtime.config.diskSizeGb) : '',
      gceAllowedMachineTypes: (runtime.config.allowedMachineTypes ?? []).join(', '),
      ...exposureFields,
    }
  }
  if (runtime.type === 'lxd') {
    return {
      ...emptyRuntimeForm(),
      id: runtime.id,
      name: runtime.name,
      type: 'lxd',
      lxdEndpoint: runtime.config.endpoint,
      lxdStartupScript: runtime.config.startupScript,
      ...exposureFields,
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
    libvirtStartupScript: runtime.config.startupScript,
    ...exposureFields,
  }
}

export function RuntimeFormPage({ user }: RuntimeFormPageProps) {
  const { runtimeID } = useParams()
  const navigate = useNavigate()
  const isEdit = runtimeID != null && runtimeID !== ''

  const [form, setForm] = useState<RuntimeFormState>(emptyRuntimeForm())
  const [loading, setLoading] = useState(isEdit)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  const validationError = useMemo(() => validateRuntimeForm(form), [form])

  useEffect(() => {
    if (!isEdit || user == null) {
      return
    }
    let cancelled = false
    const run = async () => {
      setLoading(true)
      setError('')
      try {
        const items = await listRuntimes()
        if (cancelled) return
        const found = items.find((item) => item.id === runtimeID)
        if (found != null) {
          setForm(fillFormFromRuntime(found))
        } else {
          setError('Runtime not found.')
        }
      } catch (err) {
        if (!cancelled) {
          setError(messageFromError(err))
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
  }, [isEdit, runtimeID, user])

  if (user == null) {
    return <Navigate to="/login" replace />
  }
  if (user.role !== 'admin') {
    return <Navigate to="/machines" replace />
  }

  const submit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (validationError != null) {
      setError(validationError)
      return
    }

    setError('')
    setSaving(true)
    try {
      const runtimeName = form.name.trim().toLowerCase()
      const config = toConfig(form)
      const exposure = toExposureConfig(form)
      const serverApiUrl = form.serverApiUrl.trim() || undefined
      const autoStopHours = parseFloat(form.autoStopTimeoutHours.trim())
      const autoStopTimeoutSeconds = autoStopHours > 0 ? Math.round(autoStopHours * 3600) : 0
      if (form.id === '') {
        const created = await createRuntime(runtimeName, form.type, config, exposure, serverApiUrl, autoStopTimeoutSeconds || undefined)
        navigate(`/runtimes/${created.id}`)
      } else {
        await updateRuntime(form.id, runtimeName, form.type, config, exposure, serverApiUrl, autoStopTimeoutSeconds)
        navigate(`/runtimes/${form.id}`)
      }
    } catch (err) {
      setError(messageFromError(err))
    } finally {
      setSaving(false)
    }
  }

  return (
    <main className="min-h-dvh px-6 py-10">
      <section className="mx-auto w-full max-w-4xl space-y-6">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-2xl font-semibold text-foreground">{isEdit ? 'Edit runtime' : 'New runtime'}</h1>
            <p className="mt-1 text-sm text-muted-foreground">
              {isEdit ? 'Update runtime configuration.' : 'Create a new runtime catalog entry.'}
            </p>
          </div>
          <Button asChild variant="secondary">
            <Link to={isEdit ? `/runtimes/${runtimeID}` : '/runtimes'}>Cancel</Link>
          </Button>
        </div>

        {loading ? (
          <p className="text-sm text-muted-foreground">Loading...</p>
        ) : (
          <Card className="py-0 shadow-sm">
            <CardHeader className="space-y-2 p-6 pb-3">
              <CardTitle className="text-xl">{isEdit ? 'Edit runtime' : 'Create runtime'}</CardTitle>
              <CardDescription>
                Runtime IDs are generated automatically. Names must be unique.
              </CardDescription>
            </CardHeader>
            <CardContent className="p-6 pt-3">
              <form className="space-y-4" onSubmit={submit}>
                <div className="space-y-2">
                  <Label htmlFor="runtime-name">Name</Label>
                  <Input
                    id="runtime-name"
                    value={form.name}
                    onChange={(event) => setForm((current) => ({ ...current, name: event.target.value }))}
                    className="h-10"
                    placeholder="main-libvirt"
                    required
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="runtime-type">Type</Label>
                  <select
                    id="runtime-type"
                    value={form.type}
                    onChange={(event) => {
                      const val = event.target.value
                      const t: RuntimeCatalogType = val === 'gce' ? 'gce' : val === 'lxd' ? 'lxd' : 'libvirt'
                      setForm((current) => ({
                        ...current,
                        type: t,
                        exposureConnectivity: (t === 'libvirt' || t === 'lxd') && current.exposureConnectivity === 'public_ip' ? '' : current.exposureConnectivity,
                      }))
                    }}
                    className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    <option value="libvirt">Libvirt</option>
                    <option value="gce">Google Compute Engine (GCE)</option>
                    <option value="lxd">LXD</option>
                  </select>
                </div>

                {form.type === 'gce' ? (
                  <div className="space-y-4 rounded-md border border-border bg-muted/30 p-4">
                    <div className="space-y-2">
                      <Label htmlFor="runtime-gce-project">Project</Label>
                      <Input id="runtime-gce-project" value={form.gceProject} onChange={(event) => setForm((current) => ({ ...current, gceProject: event.target.value }))} className="h-10" />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="runtime-gce-zone">Zone</Label>
                      <Input id="runtime-gce-zone" value={form.gceZone} onChange={(event) => setForm((current) => ({ ...current, gceZone: event.target.value }))} className="h-10" />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="runtime-gce-network">Network</Label>
                      <Input id="runtime-gce-network" value={form.gceNetwork} onChange={(event) => setForm((current) => ({ ...current, gceNetwork: event.target.value }))} className="h-10" />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="runtime-gce-subnetwork">Subnetwork</Label>
                      <Input id="runtime-gce-subnetwork" value={form.gceSubnetwork} onChange={(event) => setForm((current) => ({ ...current, gceSubnetwork: event.target.value }))} className="h-10" />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="runtime-gce-service-account-email">Service account email</Label>
                      <Input id="runtime-gce-service-account-email" value={form.gceServiceAccountEmail} onChange={(event) => setForm((current) => ({ ...current, gceServiceAccountEmail: event.target.value }))} className="h-10" />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="runtime-gce-disk-size-gb">Disk size in GB (optional)</Label>
                      <Input id="runtime-gce-disk-size-gb" type="number" value={form.gceDiskSizeGb} onChange={(event) => setForm((current) => ({ ...current, gceDiskSizeGb: event.target.value }))} className="h-10" placeholder="40" />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="runtime-gce-allowed-machine-types">Machine types</Label>
                      <Input
                        id="runtime-gce-allowed-machine-types"
                        value={form.gceAllowedMachineTypes}
                        onChange={(event) => setForm((current) => ({ ...current, gceAllowedMachineTypes: event.target.value }))}
                        className="h-10"
                        placeholder="e2-medium, e2-standard-2, e2-standard-4"
                      />
                      <p className="text-xs text-muted-foreground">Required. Comma-separated list of machine types users can choose when creating machines.</p>
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="runtime-gce-startup-script">Startup script (Bash, optional)</Label>
                      <textarea
                        id="runtime-gce-startup-script"
                        value={form.gceStartupScript}
                        onChange={(event) => setForm((current) => ({ ...current, gceStartupScript: event.target.value }))}
                        className="min-h-40 w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                        placeholder="#!/usr/bin/env bash"
                      />
                      <p className="text-xs text-muted-foreground">{utf8ByteLength(form.gceStartupScript)} / {maxStartupScriptBytes} bytes</p>
                    </div>
                  </div>
                ) : form.type === 'lxd' ? (
                  <div className="space-y-4 rounded-md border border-border bg-muted/30 p-4">
                    <div className="space-y-2">
                      <Label htmlFor="runtime-lxd-endpoint">Endpoint</Label>
                      <Input id="runtime-lxd-endpoint" value={form.lxdEndpoint} onChange={(event) => setForm((current) => ({ ...current, lxdEndpoint: event.target.value }))} className="h-10" placeholder="https://localhost:8443" />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="runtime-lxd-startup-script">Startup script (Bash, optional)</Label>
                      <textarea
                        id="runtime-lxd-startup-script"
                        value={form.lxdStartupScript}
                        onChange={(event) => setForm((current) => ({ ...current, lxdStartupScript: event.target.value }))}
                        className="min-h-40 w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                        placeholder="#!/usr/bin/env bash"
                      />
                      <p className="text-xs text-muted-foreground">{utf8ByteLength(form.lxdStartupScript)} / {maxStartupScriptBytes} bytes</p>
                    </div>
                  </div>
                ) : (
                  <div className="space-y-4 rounded-md border border-border bg-muted/30 p-4">
                    <div className="space-y-2">
                      <Label htmlFor="runtime-libvirt-uri">URI</Label>
                      <Input id="runtime-libvirt-uri" value={form.libvirtURI} onChange={(event) => setForm((current) => ({ ...current, libvirtURI: event.target.value }))} className="h-10" placeholder="qemu:///system" />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="runtime-libvirt-network">Network</Label>
                      <Input id="runtime-libvirt-network" value={form.libvirtNetwork} onChange={(event) => setForm((current) => ({ ...current, libvirtNetwork: event.target.value }))} className="h-10" placeholder="default" />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="runtime-libvirt-storage-pool">Storage pool</Label>
                      <Input id="runtime-libvirt-storage-pool" value={form.libvirtStoragePool} onChange={(event) => setForm((current) => ({ ...current, libvirtStoragePool: event.target.value }))} className="h-10" placeholder="default" />
                    </div>
                    <div className="space-y-2">
                      <Label htmlFor="runtime-libvirt-startup-script">Startup script (Bash, optional)</Label>
                      <textarea
                        id="runtime-libvirt-startup-script"
                        value={form.libvirtStartupScript}
                        onChange={(event) => setForm((current) => ({ ...current, libvirtStartupScript: event.target.value }))}
                        className="min-h-40 w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                        placeholder="#!/usr/bin/env bash"
                      />
                      <p className="text-xs text-muted-foreground">{utf8ByteLength(form.libvirtStartupScript)} / {maxStartupScriptBytes} bytes</p>
                    </div>
                  </div>
                )}

                <div className="space-y-4 rounded-md border border-border bg-muted/30 p-4">
                  <p className="text-sm font-medium text-foreground">Machine exposure</p>
                  <p className="text-xs text-muted-foreground">
                    Machine traffic is reverse-proxied through the Arca server.
                  </p>
                  <div className="space-y-2">
                    <Label htmlFor="runtime-exposure-domain-prefix">Domain prefix</Label>
                    <Input id="runtime-exposure-domain-prefix" value={form.exposureDomainPrefix} onChange={(event) => setForm((current) => ({ ...current, exposureDomainPrefix: event.target.value }))} className="h-10" placeholder="arca-" />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="runtime-exposure-base-domain">Base domain</Label>
                    <Input id="runtime-exposure-base-domain" value={form.exposureBaseDomain} onChange={(event) => setForm((current) => ({ ...current, exposureBaseDomain: event.target.value }))} className="h-10" placeholder="example.com" />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="runtime-exposure-connectivity">Connectivity</Label>
                    <select
                      id="runtime-exposure-connectivity"
                      value={form.exposureConnectivity}
                      onChange={(event) =>
                        setForm((current) => ({ ...current, exposureConnectivity: event.target.value as 'private_ip' | 'public_ip' | '' }))
                      }
                      className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    >
                      <option value="">Not set</option>
                      <option value="private_ip">Private IP</option>
                      {form.type === 'gce' && <option value="public_ip">Public IP</option>}
                    </select>
                    <p className="text-xs text-muted-foreground">How the server reaches machine IPs for reverse proxying.</p>
                  </div>
                </div>

                <div className="space-y-2">
                  <Label htmlFor="runtime-server-api-url">Server API URL</Label>
                  <Input
                    id="runtime-server-api-url"
                    value={form.serverApiUrl}
                    onChange={(event) => setForm((current) => ({ ...current, serverApiUrl: event.target.value }))}
                    className="h-10"
                    placeholder="https://<server domain>"
                  />
                  <p className="text-xs text-muted-foreground">Override the URL machines use to reach the API server. Leave empty to use the default (https:// + server domain).</p>
                </div>

                <div className="space-y-2">
                  <Label htmlFor="runtime-auto-stop-timeout">Auto-stop timeout (hours)</Label>
                  <Input
                    id="runtime-auto-stop-timeout"
                    type="number"
                    min="0"
                    step="any"
                    value={form.autoStopTimeoutHours}
                    onChange={(event) => setForm((current) => ({ ...current, autoStopTimeoutHours: event.target.value }))}
                    className="h-10"
                    placeholder="0 (disabled)"
                  />
                  <p className="text-xs text-muted-foreground">Automatically stop machines after this many hours of inactivity. Set to 0 or leave empty to disable.</p>
                </div>

                <div className="flex items-center gap-3">
                  <Button type="submit" className="h-10 px-5" disabled={saving || validationError != null}>
                    {saving ? 'Saving...' : isEdit ? 'Save runtime' : 'Create runtime'}
                  </Button>
                  <Button asChild type="button" variant="secondary">
                    <Link to={isEdit ? `/runtimes/${runtimeID}` : '/runtimes'}>Cancel</Link>
                  </Button>
                </div>
              </form>

              {validationError != null && <p className="mt-4 text-sm text-amber-200">{validationError}</p>}
              {error !== '' && <p className="mt-4 text-sm text-red-300">{error}</p>}
            </CardContent>
          </Card>
        )}
      </section>
    </main>
  )
}
