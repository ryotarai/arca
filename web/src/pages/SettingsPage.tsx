import { useEffect, useState } from 'react'
import { Link, Navigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Label } from '@/components/ui/label'
import { getUserSettings, updateUserSettings } from '@/lib/api'
import { messageFromError } from '@/lib/errors'
import type { User } from '@/lib/types'

type SettingsPageProps = {
  user: User | null
  onLogout: () => Promise<void>
}

export function SettingsPage({ user, onLogout }: SettingsPageProps) {
  const [sshPublicKeysText, setSshPublicKeysText] = useState('')
  const [sshLoading, setSshLoading] = useState(false)
  const [sshSaving, setSshSaving] = useState(false)
  const [sshError, setSshError] = useState('')
  const [sshSaved, setSshSaved] = useState(false)

  useEffect(() => {
    if (user == null) {
      return
    }
    let cancelled = false
    const load = async () => {
      setSshLoading(true)
      setSshError('')
      try {
        const settings = await getUserSettings()
        if (cancelled) {
          return
        }
        setSshPublicKeysText(settings.sshPublicKeys.join('\n'))
      } catch (e) {
        if (!cancelled) {
          setSshError(messageFromError(e))
        }
      } finally {
        if (!cancelled) {
          setSshLoading(false)
        }
      }
    }
    void load()
    return () => {
      cancelled = true
    }
  }, [user?.id])

  if (user == null) {
    return <Navigate to="/login" replace />
  }

  const submitSSHSettings = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setSshError('')
    setSshSaved(false)
    setSshSaving(true)
    try {
      const keys = sshPublicKeysText
        .split(/\r?\n/)
        .map((value) => value.trim())
        .filter((value) => value !== '')
      const settings = await updateUserSettings(keys)
      setSshPublicKeysText(settings.sshPublicKeys.join('\n'))
      setSshSaved(true)
    } catch (e) {
      setSshError(messageFromError(e))
    } finally {
      setSshSaving(false)
    }
  }

  return (
    <main className="min-h-dvh px-6 py-10">
      <section className="mx-auto w-full max-w-3xl space-y-6">
        <header className="flex flex-col items-start justify-between gap-4 rounded-xl border border-border bg-muted/30 p-6 md:flex-row md:items-center">
          <div>
            <p className="text-xs font-medium uppercase tracking-[0.24em] text-muted-foreground">Arca</p>
            <h1 className="mt-2 text-2xl font-semibold text-foreground">User settings</h1>
            <p className="mt-1 text-sm text-muted-foreground">Manage SSH public keys for your interactive machine user.</p>
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
            <CardTitle className="text-xl">SSH public keys</CardTitle>
            <CardDescription>
              Configure keys added to your machine interactive user&apos;s <code>~/.ssh/authorized_keys</code>.
            </CardDescription>
          </CardHeader>
          <CardContent className="p-6 pt-3">
            <form className="space-y-4" onSubmit={submitSSHSettings}>
              <div className="space-y-2">
                <Label htmlFor="settings-ssh-public-keys">
                  Public keys (one per line)
                </Label>
                <textarea
                  id="settings-ssh-public-keys"
                  value={sshPublicKeysText}
                  onChange={(event) => setSshPublicKeysText(event.target.value)}
                  rows={8}
                  className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  placeholder={'ssh-ed25519 AAAA... you@example.com'}
                  disabled={sshLoading || sshSaving}
                />
              </div>
              <Button type="submit" className="h-10 w-full" disabled={sshLoading || sshSaving}>
                {sshSaving ? 'Saving...' : 'Save SSH keys'}
              </Button>
            </form>
            {sshSaved && <p className="mt-3 text-sm text-emerald-300">SSH keys updated.</p>}
            {sshError !== '' && <p className="mt-3 text-sm text-red-300">{sshError}</p>}
          </CardContent>
        </Card>
      </section>
    </main>
  )
}
