import { useEffect, useState } from 'react'
import { Link, Navigate, useNavigate, useParams } from 'react-router-dom'
import { EndpointVisibility } from '@/gen/arca/v1/tunnel_pb'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { getSetupStatus, listMachineExposures, updateMachineExposureVisibility } from '@/lib/api'
import { messageFromError } from '@/lib/errors'
import type { MachineExposure, User } from '@/lib/types'

type MachineEditPageProps = {
  user: User | null
  onLogout: () => Promise<void>
}

export function MachineEditPage({ user, onLogout }: MachineEditPageProps) {
  const { machineID } = useParams()
  const navigate = useNavigate()
  const [loading, setLoading] = useState(true)
  const [defaultExposure, setDefaultExposure] = useState<MachineExposure | null>(null)
  const [exposureVisibility, setExposureVisibility] = useState<EndpointVisibility>(EndpointVisibility.OWNER_ONLY)
  const [selectedUserIDsInput, setSelectedUserIDsInput] = useState('')
  const [internetPublicExposureDisabled, setInternetPublicExposureDisabled] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  const internetPublicBlockedByPolicy =
    internetPublicExposureDisabled && exposureVisibility === EndpointVisibility.INTERNET_PUBLIC

  useEffect(() => {
    if (user == null || machineID == null || machineID === '') {
      return
    }

    let cancelled = false

    const run = async () => {
      setLoading(true)
      setError('')
      try {
        const [exposureItems, setupStatus] = await Promise.all([
          listMachineExposures(machineID),
          getSetupStatus(),
        ])
        if (cancelled) {
          return
        }
        const defaultItem = exposureItems.find((item) => item.name === 'default') ?? null
        setDefaultExposure(defaultItem)
        setExposureVisibility(defaultItem?.visibility ?? EndpointVisibility.OWNER_ONLY)
        setSelectedUserIDsInput((defaultItem?.selectedUserIds ?? []).join(', '))
        setInternetPublicExposureDisabled(setupStatus.internetPublicExposureDisabled)
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
  }, [user, machineID])

  if (user == null) {
    return <Navigate to="/login" replace />
  }
  if (machineID == null || machineID === '') {
    return <Navigate to="/machines" replace />
  }

  const handleSave = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    if (defaultExposure == null) {
      setError('Default exposure is not provisioned yet.')
      return
    }

    setSaving(true)
    setError('')
    try {
      const selectedUserIDs =
        exposureVisibility === EndpointVisibility.SELECTED_USERS
          ? selectedUserIDsInput
              .split(',')
              .map((value) => value.trim())
              .filter((value) => value !== '')
          : []
      await updateMachineExposureVisibility(
        machineID,
        defaultExposure.name,
        exposureVisibility,
        selectedUserIDs,
      )
      await navigate(`/machines/${machineID}`)
    } catch (e) {
      setError(messageFromError(e))
    } finally {
      setSaving(false)
    }
  }

  return (
    <main className="min-h-dvh px-6 py-10">
      <section className="mx-auto w-full max-w-3xl space-y-6">
        <header className="flex flex-col items-start justify-between gap-4 rounded-xl border border-border bg-muted/30 p-6 md:flex-row md:items-center">
          <div>
            <p className="text-xs font-medium uppercase tracking-[0.24em] text-muted-foreground">Arca</p>
            <h1 className="mt-2 text-2xl font-semibold text-foreground">Edit machine</h1>
            <p className="mt-1 text-xs text-muted-foreground">{machineID}</p>
          </div>
          <div className="flex items-center gap-3">
            <Button asChild type="button" variant="secondary">
              <Link to={`/machines/${machineID}`}>Back</Link>
            </Button>
            <Button type="button" variant="secondary" onClick={onLogout}>
              Logout
            </Button>
          </div>
        </header>

        <Card className="py-0 shadow-sm">
          <CardHeader className="space-y-2 p-6 pb-3">
            <CardTitle className="text-xl">Endpoint visibility</CardTitle>
            <CardDescription>Control who can access this machine's endpoint.</CardDescription>
          </CardHeader>
          <CardContent className="p-6 pt-3">
            {loading ? (
              <p className="text-sm text-muted-foreground">Loading...</p>
            ) : (
              <form className="space-y-4" onSubmit={(e) => void handleSave(e)}>
                <div className="space-y-2">
                  <label htmlFor="exposure-visibility" className="text-sm text-foreground">
                    Visibility
                  </label>
                  <select
                    id="exposure-visibility"
                    value={exposureVisibility}
                    onChange={(event) => setExposureVisibility(Number(event.target.value) as EndpointVisibility)}
                    className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm text-foreground"
                    disabled={defaultExposure == null}
                  >
                    <option value={EndpointVisibility.OWNER_ONLY}>Owner only</option>
                    <option value={EndpointVisibility.SELECTED_USERS}>Selected Arca users</option>
                    <option value={EndpointVisibility.ALL_ARCA_USERS}>All Arca users</option>
                    <option
                      value={EndpointVisibility.INTERNET_PUBLIC}
                      disabled={internetPublicExposureDisabled}
                    >
                      Internet public
                    </option>
                  </select>
                  {internetPublicExposureDisabled && (
                    <p className="text-xs text-amber-300">
                      Internet public visibility is disabled by admin policy.
                    </p>
                  )}
                </div>

                {exposureVisibility === EndpointVisibility.SELECTED_USERS && (
                  <div className="space-y-2">
                    <label htmlFor="exposure-selected-users" className="text-sm text-foreground">
                      Allowed user IDs
                    </label>
                    <p className="text-xs text-muted-foreground">Comma-separated user IDs allowed to access this endpoint.</p>
                    <input
                      id="exposure-selected-users"
                      value={selectedUserIDsInput}
                      onChange={(event) => setSelectedUserIDsInput(event.target.value)}
                      className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm text-foreground"
                      placeholder="user-id-1, user-id-2"
                    />
                  </div>
                )}

                {defaultExposure == null && !loading && (
                  <p className="text-sm text-amber-300">Default exposure is not provisioned yet.</p>
                )}

                <Button
                  type="submit"
                  className="h-10 px-5"
                  disabled={saving || defaultExposure == null || internetPublicBlockedByPolicy}
                >
                  {saving ? 'Saving...' : 'Save'}
                </Button>
              </form>
            )}

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
