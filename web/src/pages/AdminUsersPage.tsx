import { useEffect, useMemo, useState } from 'react'
import { Link, Navigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { createManagedUser, issueManagedUserSetupToken, listManagedUsers, updateUserRole } from '@/lib/api'
import { messageFromError } from '@/lib/errors'
import type { ManagedUser, User } from '@/lib/types'

type AdminUsersPageProps = {
  user: User | null
  onLogout: () => Promise<void>
}

type TokenResult = {
  email: string
  setupToken: string
  setupTokenExpiresAt: number
}

function formatUnix(unix: number): string {
  if (unix <= 0) {
    return 'not issued'
  }
  return new Date(unix * 1000).toLocaleString()
}

export function AdminUsersPage({ user, onLogout }: AdminUsersPageProps) {
  const [users, setUsers] = useState<ManagedUser[]>([])
  const [loading, setLoading] = useState(true)
  const [email, setEmail] = useState('')
  const [saving, setSaving] = useState(false)
  const [refreshingUserID, setRefreshingUserID] = useState('')
  const [togglingRoleUserID, setTogglingRoleUserID] = useState('')
  const [tokenResult, setTokenResult] = useState<TokenResult | null>(null)
  const [error, setError] = useState('')
  const setupBaseURL = useMemo(() => `${window.location.origin}/users/setup`, [])

  useEffect(() => {
    const run = async () => {
      setLoading(true)
      setError('')
      try {
        setUsers(await listManagedUsers())
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

  const reloadUsers = async () => {
    setUsers(await listManagedUsers())
  }

  const handleCreateUser = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setError('')
    setSaving(true)
    try {
      const created = await createManagedUser(email.trim())
      setTokenResult({
        email: created.user.email,
        setupToken: created.setupToken,
        setupTokenExpiresAt: created.setupTokenExpiresAt,
      })
      setEmail('')
      await reloadUsers()
    } catch (err) {
      setError(messageFromError(err))
    } finally {
      setSaving(false)
    }
  }

  const handleToggleRole = async (target: ManagedUser) => {
    setError('')
    setTogglingRoleUserID(target.id)
    try {
      const newRole = target.role === 'admin' ? 'user' : 'admin'
      const updated = await updateUserRole(target.id, newRole)
      setUsers((prev) => prev.map((u) => (u.id === updated.id ? updated : u)))
    } catch (err) {
      setError(messageFromError(err))
    } finally {
      setTogglingRoleUserID('')
    }
  }

  const handleIssueToken = async (target: ManagedUser) => {
    setError('')
    setRefreshingUserID(target.id)
    try {
      const result = await issueManagedUserSetupToken(target.id)
      setTokenResult({
        email: target.email,
        setupToken: result.setupToken,
        setupTokenExpiresAt: result.setupTokenExpiresAt,
      })
      await reloadUsers()
    } catch (err) {
      setError(messageFromError(err))
    } finally {
      setRefreshingUserID('')
    }
  }

  return (
    <main className="min-h-dvh px-6 py-10">
      <section className="mx-auto w-full max-w-4xl space-y-6">
        <header className="flex flex-col items-start justify-between gap-4 rounded-xl border border-border bg-muted/30 p-6 md:flex-row md:items-center">
          <div>
            <p className="text-xs font-medium uppercase tracking-[0.24em] text-muted-foreground">Arca</p>
            <h1 className="mt-2 text-2xl font-semibold text-foreground">Users</h1>
            <p className="mt-1 text-sm text-muted-foreground">Create users and issue one-time setup tokens.</p>
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
            <CardTitle className="text-xl">Provision user</CardTitle>
            <CardDescription>A setup token is generated immediately and can be used once.</CardDescription>
          </CardHeader>
          <CardContent className="p-6 pt-3">
            <form className="space-y-4" onSubmit={handleCreateUser}>
              <div className="space-y-2">
                <Label htmlFor="user-email">Email</Label>
                <Input
                  id="user-email"
                  value={email}
                  onChange={(event) => setEmail(event.target.value)}
                  className="h-10"
                  placeholder="new-user@example.com"
                  required
                />
              </div>
              <Button type="submit" className="h-10 w-full" disabled={saving}>
                {saving ? 'Creating...' : 'Create user'}
              </Button>
            </form>
          </CardContent>
        </Card>

        {tokenResult != null && (
          <Card className="border-emerald-300/25 bg-emerald-200/10 py-0">
            <CardHeader className="space-y-2 p-6 pb-3">
              <CardTitle className="text-base text-emerald-100">One-time setup token</CardTitle>
              <CardDescription className="text-emerald-200/90">
                Share this with {tokenResult.email}. The token expires at {formatUnix(tokenResult.setupTokenExpiresAt)}.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-2 p-6 pt-3 text-sm text-emerald-50">
              <p className="rounded-md border border-emerald-100/25 bg-black/25 px-3 py-2 font-mono break-all">{tokenResult.setupToken}</p>
              <p className="rounded-md border border-emerald-100/25 bg-black/25 px-3 py-2 font-mono break-all">
                {setupBaseURL}?token={encodeURIComponent(tokenResult.setupToken)}
              </p>
            </CardContent>
          </Card>
        )}

        <Card className="py-0 shadow-sm">
          <CardHeader className="space-y-2 p-6 pb-3">
            <CardTitle className="text-xl">Managed users</CardTitle>
            <CardDescription>Setup-required users cannot sign in until they complete password setup.</CardDescription>
          </CardHeader>
          <CardContent className="p-6 pt-3">
            {loading ? (
              <p className="text-sm text-muted-foreground">Loading users...</p>
            ) : users.length === 0 ? (
              <p className="text-sm text-muted-foreground">No users found.</p>
            ) : (
              <div className="space-y-3">
                {users.map((managedUser) => (
                  <div key={managedUser.id} className="rounded-lg border border-border bg-muted/20 p-4">
                    <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
                      <div>
                        <div className="flex items-center gap-2">
                          <p className="text-sm font-medium text-foreground">{managedUser.email}</p>
                          <span className={`inline-flex items-center rounded px-1.5 py-0.5 text-xs font-medium ${managedUser.role === 'admin' ? 'bg-sky-500/20 text-sky-300' : 'bg-muted text-muted-foreground'}`}>
                            {managedUser.role}
                          </span>
                        </div>
                        <p className="text-xs text-muted-foreground">Created {formatUnix(managedUser.createdAt)}</p>
                        <p className="text-xs text-muted-foreground">
                          {managedUser.setupRequired
                            ? `Setup required, token expires ${formatUnix(managedUser.setupTokenExpiresAt)}`
                            : 'Setup complete'}
                        </p>
                      </div>
                      <div className="flex items-center gap-2">
                        {managedUser.id !== user.id && (
                          <Button
                            type="button"
                            variant="secondary"
                            onClick={() => handleToggleRole(managedUser)}
                            disabled={togglingRoleUserID === managedUser.id}
                          >
                            {togglingRoleUserID === managedUser.id
                              ? 'Updating...'
                              : managedUser.role === 'admin'
                                ? 'Revoke admin'
                                : 'Make admin'}
                          </Button>
                        )}
                        <Button
                          type="button"
                          variant="secondary"
                          onClick={() => handleIssueToken(managedUser)}
                          disabled={refreshingUserID === managedUser.id}
                        >
                          {refreshingUserID === managedUser.id ? 'Issuing...' : 'Issue setup token'}
                        </Button>
                      </div>
                    </div>
                  </div>
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
