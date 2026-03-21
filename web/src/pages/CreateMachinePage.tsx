import { useEffect, useState } from 'react'
import { Link, Navigate, useNavigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { createMachine, listAvailableImages, listAvailableMachineTemplates } from '@/lib/api'
import type { CustomImage } from '@/lib/api'
import { messageFromError } from '@/lib/errors'
import type { MachineTemplateSummary, User } from '@/lib/types'

type CreateMachinePageProps = {
  user: User | null
  onLogout: () => Promise<void>
}

export function CreateMachinePage({ user, onLogout }: CreateMachinePageProps) {
  const navigate = useNavigate()
  const [name, setName] = useState('')
  const [selectedTemplateID, setSelectedTemplateID] = useState('')
  const [templates, setTemplates] = useState<MachineTemplateSummary[]>([])
  const [machineType, setMachineType] = useState('')
  const [customImageId, setCustomImageId] = useState('')
  const [availableImages, setAvailableImages] = useState<CustomImage[]>([])
  const [loadingTemplates, setLoadingTemplates] = useState(true)
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (user == null) {
      return
    }

    let cancelled = false

    const run = async () => {
      setLoadingTemplates(true)
      setError('')
      try {
        const items = await listAvailableMachineTemplates()
        if (cancelled) {
          return
        }
        setTemplates(items)
        setSelectedTemplateID((current) => {
          if (current !== '' && items.some((template) => template.id === current)) {
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
          setLoadingTemplates(false)
        }
      }
    }

    void run()

    return () => {
      cancelled = true
    }
  }, [user])

  // Fetch available images when template changes
  useEffect(() => {
    if (selectedTemplateID === '') {
      setAvailableImages([])
      setCustomImageId('')
      return
    }
    let cancelled = false
    const fetchImages = async () => {
      try {
        const imgs = await listAvailableImages(selectedTemplateID)
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
  }, [selectedTemplateID])

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
    if (selectedTemplateID.trim() === '') {
      setError('Template is required.')
      return
    }

    setCreating(true)
    setError('')
    try {
      const options: Record<string, string> = {}
      const selectedTemplate = templates.find((r) => r.id === selectedTemplateID)
      const effectiveMachineType = machineType.trim() ||
        (selectedTemplate?.type === 'gce'
          ? (selectedTemplate.allowedMachineTypes ?? [])[0] ?? ''
          : '')
      if (effectiveMachineType !== '') {
        options.machine_type = effectiveMachineType
      }
      const created = await createMachine(
        trimmedName,
        selectedTemplateID,
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
            <p className="mt-1 text-sm text-muted-foreground">Create a machine from an existing template.</p>
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
            <CardDescription>Choose template and machine name before creation.</CardDescription>
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
                <Label htmlFor="create-machine-template">
                  Template
                </Label>
                <select
                  id="create-machine-template"
                  value={selectedTemplateID}
                  onChange={(event) => setSelectedTemplateID(event.target.value)}
                  className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm text-foreground"
                  disabled={loadingTemplates || templates.length === 0}
                >
                  {templates.length === 0 && <option value="">No template available</option>}
                  {templates.map((template) => (
                    <option key={template.id} value={template.id}>
                      {template.name} ({template.type})
                    </option>
                  ))}
                </select>
              </div>

              {(() => {
                const selectedTemplate = templates.find((r) => r.id === selectedTemplateID)
                if (selectedTemplate == null || selectedTemplate.type !== 'gce') return null
                const allowed = selectedTemplate.allowedMachineTypes ?? []
                if (allowed.length === 0) return null
                return (
                  <div className="space-y-2">
                    <Label htmlFor="create-machine-type">Machine type</Label>
                    <select
                      id="create-machine-type"
                      value={machineType || allowed[0]}
                      onChange={(event) => setMachineType(event.target.value)}
                      className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm text-foreground"
                    >
                      {allowed.map((mt) => (
                        <option key={mt} value={mt}>
                          {mt}
                        </option>
                      ))}
                    </select>
                    <p className="text-xs text-muted-foreground">
                      Select a machine type for this GCE instance.
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
                    Select a custom image or use the template default.
                  </p>
                </div>
              )}

              {templates.length === 0 && !loadingTemplates && (
                <p className="text-sm text-amber-300">Create at least one template before creating machines.</p>
              )}

              <Button
                type="submit"
                className="h-10 px-5"
                disabled={creating || loadingTemplates || templates.length === 0}
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
