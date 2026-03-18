import { useEffect, useState } from 'react'
import { Link, Navigate, useNavigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { createMachine, listAvailableImages, listAvailableRuntimes, listRuntimes } from '@/lib/api'
import type { CustomImage } from '@/lib/api'
import { messageFromError } from '@/lib/errors'
import type { RuntimeCatalogItem, RuntimeSummary, User } from '@/lib/types'

type CreateMachinePageProps = {
  user: User | null
  onLogout: () => Promise<void>
}

export function CreateMachinePage({ user, onLogout }: CreateMachinePageProps) {
  const navigate = useNavigate()
  const [name, setName] = useState('')
  const [selectedRuntimeID, setSelectedRuntimeID] = useState('')
  const [runtimes, setRuntimes] = useState<RuntimeSummary[]>([])
  const [runtimeDetails, setRuntimeDetails] = useState<RuntimeCatalogItem[]>([])
  const [machineType, setMachineType] = useState('')
  const [customImageId, setCustomImageId] = useState('')
  const [availableImages, setAvailableImages] = useState<CustomImage[]>([])
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
        const [items, details] = await Promise.all([listAvailableRuntimes(), listRuntimes()])
        if (cancelled) {
          return
        }
        setRuntimes(items)
        setRuntimeDetails(details)
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

  // Fetch available images when runtime changes
  useEffect(() => {
    if (selectedRuntimeID === '') {
      setAvailableImages([])
      setCustomImageId('')
      return
    }
    let cancelled = false
    const fetchImages = async () => {
      try {
        const imgs = await listAvailableImages(selectedRuntimeID)
        if (!cancelled) {
          setAvailableImages(imgs)
          setCustomImageId('')
        }
      } catch {
        if (!cancelled) {
          setAvailableImages([])
        }
      }
    }
    void fetchImages()
    return () => { cancelled = true }
  }, [selectedRuntimeID])

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
      const options: Record<string, string> = {}
      if (machineType.trim() !== '') {
        options.machine_type = machineType.trim()
      }
      const created = await createMachine(
        trimmedName,
        selectedRuntimeID,
        Object.keys(options).length > 0 ? options : undefined,
        customImageId || undefined,
      )
      await navigate(`/machines/${created.id}`)
    } catch (e) {
      setError(messageFromError(e))
    } finally {
      setCreating(false)
    }
  }

  return (
    <main className="min-h-dvh px-6 py-10">
      <section className="mx-auto w-full max-w-3xl space-y-6">
        <header className="flex flex-col items-start justify-between gap-4 rounded-xl border border-border bg-muted/30 p-6 md:flex-row md:items-center">
          <div>
            <p className="text-xs font-medium uppercase tracking-[0.24em] text-muted-foreground">Arca</p>
            <h1 className="mt-2 text-2xl font-semibold text-foreground">Create machine</h1>
            <p className="mt-1 text-sm text-muted-foreground">Create a machine from an existing runtime catalog entry.</p>
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

        <Card className="py-0 shadow-sm">
          <CardHeader className="space-y-2 p-6 pb-3">
            <CardTitle className="text-xl">Machine settings</CardTitle>
            <CardDescription>Choose runtime and machine name before creation.</CardDescription>
          </CardHeader>
          <CardContent className="p-6 pt-3">
            <form className="space-y-4" onSubmit={handleSubmit}>
              <div className="space-y-2">
                <Label htmlFor="create-machine-name">
                  Name
                </Label>
                <Input
                  id="create-machine-name"
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                  className="h-10"
                  placeholder="my-machine"
                  required
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="create-machine-runtime">
                  Runtime
                </Label>
                <select
                  id="create-machine-runtime"
                  value={selectedRuntimeID}
                  onChange={(event) => setSelectedRuntimeID(event.target.value)}
                  className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm text-foreground"
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

              {(() => {
                const selectedRuntime = runtimeDetails.find((r) => r.id === selectedRuntimeID)
                if (selectedRuntime == null || selectedRuntime.type !== 'gce') return null
                const gceConfig = selectedRuntime.config
                if (gceConfig.type !== 'gce') return null
                const allowed = gceConfig.allowedMachineTypes ?? []
                const defaultMT = gceConfig.machineType || ''
                return (
                  <div className="space-y-2">
                    <Label htmlFor="create-machine-type">Machine type</Label>
                    {allowed.length > 0 ? (
                      <select
                        id="create-machine-type"
                        value={machineType || defaultMT}
                        onChange={(event) => setMachineType(event.target.value)}
                        className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm text-foreground"
                      >
                        {allowed.map((mt) => (
                          <option key={mt} value={mt}>
                            {mt}{mt === defaultMT ? ' (default)' : ''}
                          </option>
                        ))}
                      </select>
                    ) : (
                      <Input
                        id="create-machine-type"
                        value={machineType}
                        onChange={(event) => setMachineType(event.target.value)}
                        className="h-10"
                        placeholder={defaultMT || 'e2-standard-2'}
                      />
                    )}
                    <p className="text-xs text-muted-foreground">
                      {allowed.length > 0 ? 'Select a machine type for this GCE instance.' : 'Leave empty to use the runtime default.'}
                    </p>
                  </div>
                )
              })()}

              {availableImages.length > 0 && (
                <div className="space-y-2">
                  <Label htmlFor="create-machine-image">Image</Label>
                  <select
                    id="create-machine-image"
                    value={customImageId}
                    onChange={(event) => setCustomImageId(event.target.value)}
                    className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm text-foreground"
                  >
                    <option value="">Default</option>
                    {availableImages.map((img) => (
                      <option key={img.id} value={img.id}>
                        {img.name}{img.description ? ` - ${img.description}` : ''}
                      </option>
                    ))}
                  </select>
                  <p className="text-xs text-muted-foreground">
                    Select a custom image or use the runtime default.
                  </p>
                </div>
              )}

              {runtimes.length === 0 && !loadingRuntimes && (
                <p className="text-sm text-amber-300">Create at least one runtime in the runtime catalog before creating machines.</p>
              )}

              <Button
                type="submit"
                className="h-10 px-5"
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
