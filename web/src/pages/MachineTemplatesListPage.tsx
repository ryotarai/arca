import { useEffect, useState } from 'react'
import { Link, Navigate } from 'react-router-dom'
import { Plus } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { listMachineTemplates } from '@/lib/api'
import { messageFromError } from '@/lib/errors'
import type { MachineTemplateItem, User } from '@/lib/types'

type MachineTemplatesListPageProps = {
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

export function MachineTemplatesListPage({ user }: MachineTemplatesListPageProps) {
  const [templates, setTemplates] = useState<MachineTemplateItem[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    const run = async () => {
      setLoading(true)
      setError('')
      try {
        setTemplates(await listMachineTemplates())
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
  if (user.role !== 'admin') {
    return <Navigate to="/machines" replace />
  }

  return (
    <main className="min-h-dvh px-6 py-10">
      <section className="mx-auto w-full max-w-4xl space-y-6">
        <div className="flex items-center justify-between">
          <div>
            <h1 className="text-2xl font-semibold text-foreground">Machine Templates</h1>
            <p className="mt-1 text-sm text-muted-foreground">Manage machine template entries and type-specific configuration.</p>
          </div>
          <Button asChild>
            <Link to="/machine-templates/new">
              <Plus className="mr-2 h-4 w-4" />
              New template
            </Link>
          </Button>
        </div>

        <Card className="py-0 shadow-sm">
          <CardHeader className="space-y-2 p-6 pb-3">
            <CardTitle className="text-xl">Template catalog</CardTitle>
            <CardDescription>View, create, or edit template definitions.</CardDescription>
          </CardHeader>
          <CardContent className="p-6 pt-3">
            {loading ? (
              <p className="text-sm text-muted-foreground">Loading templates...</p>
            ) : templates.length === 0 ? (
              <p className="text-sm text-muted-foreground">No templates configured.</p>
            ) : (
              <div className="space-y-3">
                {templates.map((template) => (
                  <Link
                    key={template.id}
                    to={`/machine-templates/${template.id}`}
                    className="block rounded-lg border border-border bg-muted/20 p-4 transition-colors hover:bg-muted/40"
                  >
                    <div className="flex flex-col gap-3 md:flex-row md:items-start md:justify-between">
                      <div className="space-y-1">
                        <p className="text-sm font-medium text-foreground">{template.name}</p>
                        <p className="text-xs uppercase tracking-wide text-muted-foreground">{template.type}</p>
                        <p className="text-xs text-muted-foreground">
                          Exposure: {exposureLabel(template.exposure.method)}
                        </p>
                        <p className="text-xs text-muted-foreground">Created {formatUnix(template.createdAt)}</p>
                      </div>
                    </div>
                  </Link>
                ))}
              </div>
            )}
          </CardContent>
        </Card>

        {error !== '' && <p className="text-sm text-red-300">{error}</p>}
      </section>
    </main>
  )
}
