import { useEffect, useState } from 'react'
import { Link, Navigate, useNavigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { createMachine, listRuntimes } from '@/lib/api'
import { messageFromError } from '@/lib/errors'
import type { RuntimeCatalogItem, User } from '@/lib/types'

type CreateMachinePageProps = {
  user: User | null
  onLogout: () => Promise<void>
}

export function CreateMachinePage({ user, onLogout }: CreateMachinePageProps) {
  const navigate = useNavigate()
  const [name, setName] = useState('')
  const [selectedRuntimeID, setSelectedRuntimeID] = useState('')
  const [runtimes, setRuntimes] = useState<RuntimeCatalogItem[]>([])
  const [loadingRuntimes, setLoadingRuntimes] = useState(true)
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (user == null) {
      return
    }

    let cancelled = false

    const run = async () => {
      setLoadingRuntimes(true)
      setError('')
      try {
        const items = await listRuntimes()
        if (cancelled) {
          return
        }
        setRuntimes(items)
        setSelectedRuntimeID((current) => {
          if (current !== '' && items.some((runtime) => runtime.id === current)) {
            return current
          }
          return items[0]?.id ?? ''
        })
      } catch (e) {
        if (!cancelled) {
          setError(messageFromError(e))
        }
      } finally {
        if (!cancelled) {
          setLoadingRuntimes(false)
        }
      }
    }

    void run()

    return () => {
      cancelled = true
    }
  }, [user])

  if (user == null) {
    return <Navigate to="/login" replace />
  }

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const trimmedName = name.trim()
    if (trimmedName === '') {
      setError('Name is required.')
      return
    }
    if (selectedRuntimeID.trim() === '') {
      setError('Runtime is required.')
      return
    }

    setCreating(true)
    setError('')
    try {
      const created = await createMachine(trimmedName, selectedRuntimeID)
      await navigate(`/machines/${created.id}`)
    } catch (e) {
      setError(messageFromError(e))
    } finally {
      setCreating(false)
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
            <h1 className="mt-2 text-2xl font-semibold text-white">Create machine</h1>
            <p className="mt-1 text-sm text-slate-300">Create a machine from an existing runtime catalog entry.</p>
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
            <CardTitle className="text-xl text-white">Machine settings</CardTitle>
            <CardDescription className="text-slate-300">Choose runtime and machine name before creation.</CardDescription>
          </CardHeader>
          <CardContent className="p-6 pt-3">
            <form className="space-y-4" onSubmit={handleSubmit}>
              <div className="space-y-2">
                <Label htmlFor="create-machine-name" className="text-slate-200">
                  Name
                </Label>
                <Input
                  id="create-machine-name"
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                  className="h-10 border-white/20 bg-white/10 text-slate-100 placeholder:text-slate-400 focus-visible:ring-sky-400/45"
                  placeholder="my-machine"
                  required
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="create-machine-runtime" className="text-slate-200">
                  Runtime
                </Label>
                <select
                  id="create-machine-runtime"
                  value={selectedRuntimeID}
                  onChange={(event) => setSelectedRuntimeID(event.target.value)}
                  className="h-10 w-full rounded-md border border-white/20 bg-white/10 px-3 text-sm text-slate-100"
                  disabled={loadingRuntimes || runtimes.length === 0}
                >
                  {runtimes.length === 0 && <option value="">No runtime available</option>}
                  {runtimes.map((runtime) => (
                    <option key={runtime.id} value={runtime.id}>
                      {runtime.name} ({runtime.type})
                    </option>
                  ))}
                </select>
              </div>

              {runtimes.length === 0 && !loadingRuntimes && (
                <p className="text-sm text-amber-300">Create at least one runtime in the runtime catalog before creating machines.</p>
              )}

              <Button
                type="submit"
                className="h-10 bg-white px-5 text-slate-900 hover:bg-slate-100"
                disabled={creating || loadingRuntimes || runtimes.length === 0}
              >
                {creating ? 'Creating...' : 'Create machine'}
              </Button>
            </form>

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
