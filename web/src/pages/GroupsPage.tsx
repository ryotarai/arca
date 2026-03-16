import { useEffect, useRef, useState } from 'react'
import { Navigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  addGroupMember,
  createGroup,
  deleteGroup,
  getGroup,
  listGroups,
  removeGroupMember,
  searchUsers,
} from '@/lib/api'
import { messageFromError } from '@/lib/errors'
import type { User } from '@/lib/types'
import type { UserGroup, UserGroupMember } from '@/gen/arca/v1/group_pb'

type GroupsPageProps = {
  user: User | null
  onLogout: () => Promise<void>
}

export function GroupsPage({ user }: GroupsPageProps) {
  const [groups, setGroups] = useState<UserGroup[]>([])
  const [loading, setLoading] = useState(true)
  const [name, setName] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  // Detail view state
  const [selectedGroupID, setSelectedGroupID] = useState<string | null>(null)
  const [selectedGroup, setSelectedGroup] = useState<UserGroup | null>(null)
  const [members, setMembers] = useState<UserGroupMember[]>([])
  const [detailLoading, setDetailLoading] = useState(false)
  const [memberSearch, setMemberSearch] = useState('')
  const [memberSearchResults, setMemberSearchResults] = useState<{ id: string; email: string }[]>([])
  const [showMemberDropdown, setShowMemberDropdown] = useState(false)
  const memberDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const memberWrapperRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const run = async () => {
      setLoading(true)
      setError('')
      try {
        setGroups(await listGroups())
      } catch (err) {
        setError(messageFromError(err))
      } finally {
        setLoading(false)
      }
    }
    void run()
  }, [])

  useEffect(() => {
    if (selectedGroupID == null) return
    let cancelled = false
    const load = async () => {
      setDetailLoading(true)
      try {
        const result = await getGroup(selectedGroupID)
        if (cancelled) return
        setSelectedGroup(result.group)
        setMembers(result.members)
      } catch (err) {
        if (!cancelled) setError(messageFromError(err))
      } finally {
        if (!cancelled) setDetailLoading(false)
      }
    }
    void load()
    return () => { cancelled = true }
  }, [selectedGroupID])

  if (user == null) {
    return <Navigate to="/login" replace />
  }
  if (user.role !== 'admin') {
    return <Navigate to="/machines" replace />
  }

  const reloadGroups = async () => {
    setGroups(await listGroups())
  }

  const handleCreate = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setError('')
    setSaving(true)
    try {
      await createGroup(name.trim())
      setName('')
      await reloadGroups()
    } catch (err) {
      setError(messageFromError(err))
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (groupID: string) => {
    setError('')
    try {
      await deleteGroup(groupID)
      if (selectedGroupID === groupID) {
        setSelectedGroupID(null)
        setSelectedGroup(null)
        setMembers([])
      }
      await reloadGroups()
    } catch (err) {
      setError(messageFromError(err))
    }
  }

  const handleMemberSearchChange = (value: string) => {
    setMemberSearch(value)
    if (memberDebounceRef.current) {
      clearTimeout(memberDebounceRef.current)
    }
    const trimmed = value.trim()
    if (trimmed.length < 2) {
      setMemberSearchResults([])
      setShowMemberDropdown(false)
      return
    }
    memberDebounceRef.current = setTimeout(async () => {
      try {
        const results = await searchUsers(trimmed, 10)
        const memberIDs = new Set(members.map((m) => m.userId))
        const filtered = results.filter((r) => !memberIDs.has(r.id))
        setMemberSearchResults(filtered)
        setShowMemberDropdown(filtered.length > 0)
      } catch {
        setMemberSearchResults([])
        setShowMemberDropdown(false)
      }
    }, 300)
  }

  const handleAddMember = async (userResult: { id: string; email: string }) => {
    if (selectedGroupID == null) return
    setError('')
    setMemberSearch('')
    setMemberSearchResults([])
    setShowMemberDropdown(false)
    try {
      await addGroupMember(selectedGroupID, userResult.id)
      const result = await getGroup(selectedGroupID)
      setSelectedGroup(result.group)
      setMembers(result.members)
      await reloadGroups()
    } catch (err) {
      setError(messageFromError(err))
    }
  }

  const handleRemoveMember = async (userID: string) => {
    if (selectedGroupID == null) return
    setError('')
    try {
      await removeGroupMember(selectedGroupID, userID)
      const result = await getGroup(selectedGroupID)
      setSelectedGroup(result.group)
      setMembers(result.members)
      await reloadGroups()
    } catch (err) {
      setError(messageFromError(err))
    }
  }

  return (
    <main className="min-h-dvh px-6 py-10">
      <section className="mx-auto w-full max-w-4xl space-y-6">
        <header className="flex flex-col items-start justify-between gap-4 rounded-xl border border-border bg-muted/30 p-6 md:flex-row md:items-center">
          <div>
            <p className="text-xs font-medium uppercase tracking-[0.24em] text-muted-foreground">Arca</p>
            <h1 className="mt-2 text-2xl font-semibold text-foreground">Groups</h1>
            <p className="mt-1 text-sm text-muted-foreground">
              Manage user groups for sharing machines.
            </p>
          </div>
        </header>

        <Card className="py-0 shadow-sm">
          <CardHeader className="space-y-2 p-6 pb-3">
            <CardTitle className="text-xl">Create group</CardTitle>
            <CardDescription>Groups can be used to share machines with multiple users.</CardDescription>
          </CardHeader>
          <CardContent className="p-6 pt-3">
            <form className="space-y-4" onSubmit={handleCreate}>
              <div className="space-y-2">
                <Label htmlFor="group-name">Name</Label>
                <Input
                  id="group-name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  className="h-10"
                  placeholder="engineering"
                  required
                />
              </div>
              <Button type="submit" className="h-10 w-full" disabled={saving}>
                {saving ? 'Creating...' : 'Create group'}
              </Button>
            </form>
          </CardContent>
        </Card>

        <Card className="py-0 shadow-sm">
          <CardHeader className="space-y-2 p-6 pb-3">
            <CardTitle className="text-xl">All groups</CardTitle>
            <CardDescription>Click a group to manage its members.</CardDescription>
          </CardHeader>
          <CardContent className="p-6 pt-3">
            {loading ? (
              <p className="text-sm text-muted-foreground">Loading groups...</p>
            ) : groups.length === 0 ? (
              <p className="text-sm text-muted-foreground">No groups found.</p>
            ) : (
              <div className="space-y-3">
                {groups.map((group) => (
                  <div
                    key={group.id}
                    className={`rounded-lg border border-border bg-muted/20 p-4 cursor-pointer transition-colors ${selectedGroupID === group.id ? 'ring-1 ring-primary' : 'hover:bg-muted/40'}`}
                    onClick={() => setSelectedGroupID(selectedGroupID === group.id ? null : group.id)}
                  >
                    <div className="flex flex-col gap-3 md:flex-row md:items-center md:justify-between">
                      <div>
                        <p className="text-sm font-medium text-foreground">{group.name}</p>
                        <p className="text-xs text-muted-foreground">
                          {group.memberCount} {group.memberCount === 1 ? 'member' : 'members'}
                        </p>
                      </div>
                      <Button
                        type="button"
                        variant="secondary"
                        className="h-8 px-3 text-xs"
                        onClick={(e) => {
                          e.stopPropagation()
                          void handleDelete(group.id)
                        }}
                      >
                        Delete
                      </Button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>

        {selectedGroupID != null && (
          <Card className="py-0 shadow-sm">
            <CardHeader className="space-y-2 p-6 pb-3">
              <CardTitle className="text-xl">
                {selectedGroup != null ? selectedGroup.name : 'Group'} members
              </CardTitle>
              <CardDescription>Add or remove members from this group.</CardDescription>
            </CardHeader>
            <CardContent className="p-6 pt-3">
              {detailLoading ? (
                <p className="text-sm text-muted-foreground">Loading members...</p>
              ) : (
                <div className="space-y-4">
                  <div className="relative" ref={memberWrapperRef}>
                    <Input
                      placeholder="Search by email to add member"
                      value={memberSearch}
                      onChange={(e) => handleMemberSearchChange(e.target.value)}
                      onBlur={(e) => {
                        if (!memberWrapperRef.current?.contains(e.relatedTarget as Node)) {
                          setShowMemberDropdown(false)
                        }
                      }}
                      className="h-10"
                      autoComplete="off"
                    />
                    {showMemberDropdown && memberSearchResults.length > 0 && (
                      <div className="absolute left-0 right-0 z-50 mt-1 max-h-48 overflow-y-auto rounded-md border border-border bg-popover p-1 shadow-md">
                        {memberSearchResults.map((u) => (
                          <button
                            key={u.id}
                            type="button"
                            className="w-full rounded-sm px-2 py-1.5 text-sm text-left text-popover-foreground hover:bg-accent hover:text-accent-foreground cursor-pointer"
                            onMouseDown={(e) => {
                              e.preventDefault()
                              void handleAddMember(u)
                            }}
                          >
                            {u.email}
                          </button>
                        ))}
                      </div>
                    )}
                  </div>

                  {members.length === 0 ? (
                    <p className="text-sm text-muted-foreground">No members yet.</p>
                  ) : (
                    <div className="space-y-2">
                      {members.map((member) => (
                        <div
                          key={member.userId}
                          className="flex items-center justify-between gap-2 rounded-lg border border-border bg-muted/30 px-3 py-2"
                        >
                          <span className="text-sm text-foreground truncate">{member.email}</span>
                          <Button
                            type="button"
                            variant="secondary"
                            className="h-8 px-2 text-xs"
                            onClick={() => void handleRemoveMember(member.userId)}
                          >
                            Remove
                          </Button>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              )}
            </CardContent>
          </Card>
        )}

        {error !== '' && <p className="text-sm text-red-300">{error}</p>}
      </section>
    </main>
  )
}
