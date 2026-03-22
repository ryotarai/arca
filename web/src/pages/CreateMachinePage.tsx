import { useEffect, useState } from 'react'
import { Link, Navigate, useNavigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { createMachine, listAvailableImages, listAvailableProfiles } from '@/lib/api'
import type { CustomImage } from '@/lib/api'
import { messageFromError } from '@/lib/errors'
import type { MachineProfileSummary, User } from '@/lib/types'

type CreateMachinePageProps = {
  user: User | null
  onLogout: () => Promise<void>
}

const machineNamePattern = /^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$/

function validateMachineName(value: string): string {
  const trimmed = value.trim()
  if (trimmed === '') {
    return 'Name is required.'
  }
  if (trimmed.length < 3) {
    return 'Name must be at least 3 characters.'
  }
  if (trimmed.length > 63) {
    return 'Name must be 63 characters or less.'
  }
  if (trimmed.startsWith('arca-')) {
    return 'Name cannot start with arca-.'
  }
  if (!machineNamePattern.test(trimmed)) {
    return 'Name must use lowercase letters, digits, and hyphens only, and cannot start or end with a hyphen.'
  }
  return ''
}

export function CreateMachinePage({ user, onLogout }: CreateMachinePageProps) {
  const navigate = useNavigate()
  const [name, setName] = useState('')
  const [nameError, setNameError] = useState('')
  const [nameTouched, setNameTouched] = useState(false)
  const [selectedProfileID, setSelectedProfileID] = useState('')
  const [profiles, setProfiles] = useState<MachineProfileSummary[]>([])
  const [machineType, setMachineType] = useState('')
  const [customImageId, setCustomImageId] = useState('')
  const [availableImages, setAvailableImages] = useState<CustomImage[]>([])
  const [loadingProfiles, setLoadingProfiles] = useState(true)
  const [tagsInput, setTagsInput] = useState('')
  const [creating, setCreating] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (user == null) {
      return
    }

    let cancelled = false

    const run = async () => {
      setLoadingProfiles(true)
      setError('')
      try {
        const items = await listAvailableProfiles()
        if (cancelled) {
          return
        }
        setProfiles(items)
        setSelectedProfileID((current) => {
          if (current !== '' && items.some((profile) => profile.id === current)) {
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
          setLoadingProfiles(false)
        }
      }
    }

    void run()

    return () => {
      cancelled = true
    }
  }, [user])

  // Fetch available images when profile changes
  useEffect(() => {
    if (selectedProfileID === '') {
      setAvailableImages([])
      setCustomImageId('')
      return
    }
    let cancelled = false
    const fetchImages = async () => {
      try {
        const imgs = await listAvailableImages(selectedProfileID)
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
  }, [selectedProfileID])

  if (user == null) {
    return <Navigate to="/login" replace />
  }

  const handleSubmit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    const trimmedName = name.trim()
    const nameValidationError = validateMachineName(trimmedName)
    if (nameValidationError !== '') {
      setNameTouched(true)
      setNameError(nameValidationError)
      return
    }
    if (selectedProfileID.trim() === '') {
      setError('Profile is required.')
      return
    }

    setCreating(true)
    setError('')
    try {
      const options: Record<string, string> = {}
      const selectedProfile = profiles.find((r) => r.id === selectedProfileID)
      const effectiveMachineType = machineType.trim() ||
        (selectedProfile?.type === 'gce'
          ? (selectedProfile.allowedMachineTypes ?? [])[0] ?? ''
          : '')
      if (effectiveMachineType !== '') {
        options.machine_type = effectiveMachineType
      }
      const tags = tagsInput.split(',').map((t) => t.trim()).filter((t) => t !== '')
      const created = await createMachine(
        trimmedName,
        selectedProfileID,
        Object.keys(options).length > 0 ? options : undefined,
        customImageId || undefined,
        tags.length > 0 ? tags : undefined,
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
            <p className="mt-1 text-sm text-muted-foreground">Create a machine from an existing profile.</p>
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
            <CardDescription>Choose profile and machine name before creation.</CardDescription>
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
                  onChange={(event) => {
                    setName(event.target.value)
                    if (nameTouched) {
                      setNameError(validateMachineName(event.target.value))
                    }
                  }}
                  onBlur={() => {
                    setNameTouched(true)
                    setNameError(validateMachineName(name))
                  }}
                  className={`h-10${nameError !== '' ? ' border-red-400' : ''}`}
                  placeholder="my-machine"
                  required
                />
                {nameError !== '' && (
                  <p className="text-xs text-red-300">{nameError}</p>
                )}
              </div>

              <div className="space-y-2">
                <Label htmlFor="create-machine-profile">
                  Profile
                </Label>
                <select
                  id="create-machine-profile"
                  value={selectedProfileID}
                  onChange={(event) => setSelectedProfileID(event.target.value)}
                  className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm text-foreground"
                  disabled={loadingProfiles || profiles.length === 0}
                >
                  {profiles.length === 0 && <option value="">No profile available</option>}
                  {profiles.map((profile) => (
                    <option key={profile.id} value={profile.id}>
                      {profile.name} ({profile.type})
                    </option>
                  ))}
                </select>
              </div>

              {(() => {
                const selectedProfile = profiles.find((r) => r.id === selectedProfileID)
                if (selectedProfile == null || selectedProfile.type !== 'gce') return null
                const allowed = selectedProfile.allowedMachineTypes ?? []
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
                    Select a custom image or use the profile default.
                  </p>
                </div>
              )}

              <div className="space-y-2">
                <Label htmlFor="create-machine-tags">Tags</Label>
                <Input
                  id="create-machine-tags"
                  value={tagsInput}
                  onChange={(event) => setTagsInput(event.target.value)}
                  className="h-10"
                  placeholder="web, production, team-a"
                />
                <p className="text-xs text-muted-foreground">
                  Comma-separated. Lowercase alphanumeric and hyphens only. Max 10 tags.
                </p>
              </div>

              {profiles.length === 0 && !loadingProfiles && (
                <p className="text-sm text-amber-300">Create at least one profile before creating machines.</p>
              )}

              <Button
                type="submit"
                className="h-10 px-5"
                disabled={creating || loadingProfiles || profiles.length === 0}
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
