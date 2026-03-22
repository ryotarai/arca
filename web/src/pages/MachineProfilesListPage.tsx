import { useEffect, useState } from 'react'
import { Link, Navigate } from 'react-router-dom'
import { toast } from 'sonner'
import { Plus } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { PageHeader } from '@/components/PageHeader'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { listMachineProfiles } from '@/lib/api'
import { messageFromError } from '@/lib/errors'
import type { MachineProfileItem, User } from '@/lib/types'

type MachineProfilesListPageProps = {
  user: User | null
  onLogout: () => Promise<void>
}

function formatUnix(unix: number): string {
  if (unix <= 0) {
    return 'unknown'
  }
  return new Date(unix * 1000).toLocaleString()
}

function exposureLabel(_method: string): string {
  return 'proxy via server'
}

export function MachineProfilesListPage({ user }: MachineProfilesListPageProps) {
  const [profiles, setProfiles] = useState<MachineProfileItem[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    const run = async () => {
      setLoading(true)
      try {
        setProfiles(await listMachineProfiles())
      } catch (err) {
        toast.error(messageFromError(err))
      } finally {
        setLoading(false)
      }
    }
    void run()
  }, [])

  if (user == null) {
    return <Navigate to="/login" replace />
  }
  if (user.role !== 'admin') {
    return <Navigate to="/machines" replace />
  }

  return (
    <main className="min-h-dvh px-6 py-10">
      <section className="mx-auto w-full max-w-4xl space-y-6">
        <PageHeader
          label="Admin"
          title="Machine Profiles"
          description="Manage machine profile entries and provider-specific configuration."
          actions={
            <Button asChild>
              <Link to="/machine-profiles/new">
                <Plus className="mr-2 h-4 w-4" />
                New profile
              </Link>
            </Button>
          }
        />

        <Card className="py-0 shadow-sm">
          <CardHeader className="space-y-2 p-6 pb-3">
            <CardTitle className="text-xl">Profile catalog</CardTitle>
            <CardDescription>View, create, or edit profile definitions.</CardDescription>
          </CardHeader>
          <CardContent className="p-6 pt-3">
            {loading ? (
              <p className="text-sm text-muted-foreground">Loading profiles...</p>
            ) : profiles.length === 0 ? (
              <p className="text-sm text-muted-foreground">No profiles configured.</p>
            ) : (
              <div className="space-y-3">
                {profiles.map((profile) => (
                  <Link
                    key={profile.id}
                    to={`/machine-profiles/${profile.id}`}
                    className="block rounded-lg border border-border bg-muted/20 p-4 transition-colors hover:bg-muted/40"
                  >
                    <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
                      <div className="space-y-1">
                        <p className="text-sm font-medium text-foreground">{profile.name}</p>
                        <p className="text-xs uppercase tracking-wide text-muted-foreground">{profile.type}</p>
                        <p className="text-xs text-muted-foreground">
                          Exposure: {exposureLabel(profile.exposure.method)}
                        </p>
                        <p className="text-xs text-muted-foreground">Created {formatUnix(profile.createdAt)}</p>
                      </div>
                      <div className="flex items-center gap-2 text-xs text-muted-foreground">
                        <span className="inline-flex items-center rounded-full border border-border bg-muted/30 px-2 py-0.5 font-medium">
                          {profile.machineCount} {profile.machineCount === 1 ? 'machine' : 'machines'}
                        </span>
                        {profile.runningMachineCount > 0 && (
                          <span className="inline-flex items-center rounded-full border border-emerald-400/30 bg-emerald-500/10 px-2 py-0.5 font-medium text-emerald-200">
                            {profile.runningMachineCount} running
                          </span>
                        )}
                      </div>
                    </div>
                  </Link>
                ))}
              </div>
            )}
          </CardContent>
        </Card>

      </section>
    </main>
  )
}
