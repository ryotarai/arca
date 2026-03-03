import { useState } from 'react'
import { Link, Navigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { updateDomainSettings } from '@/lib/api'
import { messageFromError } from '@/lib/errors'
import type { SetupStatus, User } from '@/lib/types'

type SettingsPageProps = {
  user: User | null
  setupStatus: SetupStatus
  onSetupStatusChange: (status: SetupStatus) => void
  onLogout: () => Promise<void>
}

export function SettingsPage({ user, setupStatus, onSetupStatusChange, onLogout }: SettingsPageProps) {
  const [baseDomain, setBaseDomain] = useState(setupStatus.baseDomain)
  const [domainPrefix, setDomainPrefix] = useState(setupStatus.domainPrefix)
  const [machineRuntime, setMachineRuntime] = useState<'docker' | 'libvirt'>(setupStatus.machineRuntime)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [saved, setSaved] = useState(false)

  if (user == null) {
    return <Navigate to="/login" replace />
  }

  const submit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setError('')
    setSaved(false)
    setLoading(true)
    try {
      await updateDomainSettings(baseDomain, domainPrefix, machineRuntime)
      onSetupStatusChange({
        ...setupStatus,
        baseDomain: baseDomain.trim(),
        domainPrefix: domainPrefix.trim(),
        machineRuntime,
      })
      setSaved(true)
    } catch (e) {
      setError(messageFromError(e))
    } finally {
      setLoading(false)
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
            <h1 className="mt-2 text-2xl font-semibold text-white">Settings</h1>
            <p className="mt-1 text-sm text-slate-300">Update domain settings used for newly created machine exposures.</p>
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
            <CardTitle className="text-xl text-white">Domain settings</CardTitle>
            <CardDescription className="text-slate-300">
              Existing machine hostnames stay unchanged. New machines use this configuration.
            </CardDescription>
          </CardHeader>
          <CardContent className="p-6 pt-3">
            <form className="space-y-4" onSubmit={submit}>
              <div className="space-y-2">
                <Label htmlFor="settings-base-domain" className="text-slate-200">
                  Base domain
                </Label>
                <Input
                  id="settings-base-domain"
                  value={baseDomain}
                  onChange={(event) => setBaseDomain(event.target.value)}
                  required
                  className="h-10 border-white/20 bg-white/10 text-slate-100 placeholder:text-slate-400 focus-visible:ring-sky-400/45"
                  placeholder="ryotarai.info"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="settings-domain-prefix" className="text-slate-200">
                  Domain prefix
                </Label>
                <Input
                  id="settings-domain-prefix"
                  value={domainPrefix}
                  onChange={(event) => setDomainPrefix(event.target.value)}
                  className="h-10 border-white/20 bg-white/10 text-slate-100 placeholder:text-slate-400 focus-visible:ring-sky-400/45"
                  placeholder="arca-"
                />
              </div>
              <div className="space-y-2">
                <Label htmlFor="settings-machine-runtime" className="text-slate-200">
                  Machine runtime
                </Label>
                <select
                  id="settings-machine-runtime"
                  value={machineRuntime}
                  onChange={(event) => setMachineRuntime(event.target.value === 'libvirt' ? 'libvirt' : 'docker')}
                  className="h-10 w-full rounded-md border border-white/20 bg-white/10 px-3 text-sm text-slate-100 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-sky-400/45"
                >
                  <option value="docker">Docker</option>
                  <option value="libvirt">Libvirt (Ubuntu 24.04 VM)</option>
                </select>
              </div>
              <Button type="submit" className="h-10 w-full bg-white text-slate-900 hover:bg-slate-100" disabled={loading}>
                {loading ? 'Saving...' : 'Save settings'}
              </Button>
            </form>
            {saved && <p className="mt-3 text-sm text-emerald-300">Settings updated.</p>}
            {error !== '' && <p className="mt-3 text-sm text-red-300">{error}</p>}
          </CardContent>
        </Card>
      </section>
    </main>
  )
}
