import { useEffect, useRef, useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { getMachineSharing, searchGroups, searchUsers, updateMachineSharing } from '@/lib/api'
import { messageFromError } from '@/lib/errors'

type Member = {
  userId: string
  email: string
  role: string
}

type GroupAccess = {
  groupId: string
  name: string
  role: string
}

type SharingDialogProps = {
  machineID: string
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function SharingDialog({ machineID, open, onOpenChange }: SharingDialogProps) {
  const [members, setMembers] = useState<Member[]>([])
  const [generalAccessScope, setGeneralAccessScope] = useState('none')
  const [generalAccessRole, setGeneralAccessRole] = useState('none')
  const [newEmail, setNewEmail] = useState('')
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [searchResults, setSearchResults] = useState<{ id: string; email: string }[]>([])
  const [showDropdown, setShowDropdown] = useState(false)
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const wrapperRef = useRef<HTMLDivElement>(null)

  // Group sharing state
  const [groupAccessList, setGroupAccessList] = useState<GroupAccess[]>([])
  const [groupSearch, setGroupSearch] = useState('')
  const [groupSearchResults, setGroupSearchResults] = useState<{ id: string; name: string }[]>([])
  const [showGroupDropdown, setShowGroupDropdown] = useState(false)
  const groupDebounceRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const groupWrapperRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!open || machineID === '') return

    let cancelled = false

    const load = async () => {
      setLoading(true)
      setError('')
      try {
        const sharing = await getMachineSharing(machineID)
        if (cancelled) return
        setMembers(
          sharing.members.map((m) => ({ userId: m.userId, email: m.email, role: m.role })),
        )
        setGroupAccessList(
          sharing.groups.map((g) => ({ groupId: g.groupId, name: g.name, role: g.role })),
        )
        setGeneralAccessScope(sharing.generalAccess?.scope ?? 'none')
        setGeneralAccessRole(sharing.generalAccess?.role ?? 'none')
      } catch (e) {
        if (!cancelled) setError(messageFromError(e))
      } finally {
        if (!cancelled) setLoading(false)
      }
    }

    void load()
    return () => { cancelled = true }
  }, [open, machineID])

  useEffect(() => {
    if (!open) {
      setNewEmail('')
      setSearchResults([])
      setShowDropdown(false)
      setGroupSearch('')
      setGroupSearchResults([])
      setShowGroupDropdown(false)
    }
  }, [open])

  const handleEmailChange = (value: string) => {
    setNewEmail(value)

    if (debounceRef.current) {
      clearTimeout(debounceRef.current)
    }

    const trimmed = value.trim()
    if (trimmed.length < 2) {
      setSearchResults([])
      setShowDropdown(false)
      return
    }

    debounceRef.current = setTimeout(async () => {
      try {
        const results = await searchUsers(trimmed, 10)
        const memberEmails = new Set(members.map((m) => m.email))
        const filtered = results.filter((r) => !memberEmails.has(r.email))
        setSearchResults(filtered)
        setShowDropdown(filtered.length > 0)
      } catch {
        setSearchResults([])
        setShowDropdown(false)
      }
    }, 300)
  }

  const handleSelectUser = (user: { id: string; email: string }) => {
    if (members.some((m) => m.email === user.email)) {
      setError('User already added')
      return
    }
    setMembers((prev) => [...prev, { userId: user.id, email: user.email, role: 'viewer' }])
    setNewEmail('')
    setSearchResults([])
    setShowDropdown(false)
    setError('')
  }

  const handleAddMember = () => {
    const email = newEmail.trim()
    if (email === '') return
    if (members.some((m) => m.email === email)) {
      setError('User already added')
      return
    }
    setMembers((prev) => [...prev, { userId: '', email, role: 'viewer' }])
    setNewEmail('')
    setSearchResults([])
    setShowDropdown(false)
    setError('')
  }

  const handleRemoveMember = (email: string) => {
    setMembers((prev) => prev.filter((m) => m.email !== email))
  }

  const handleMemberRoleChange = (email: string, role: string) => {
    setMembers((prev) => prev.map((m) => (m.email === email ? { ...m, role } : m)))
  }

  const handleGroupSearchChange = (value: string) => {
    setGroupSearch(value)
    if (groupDebounceRef.current) {
      clearTimeout(groupDebounceRef.current)
    }
    const trimmed = value.trim()
    if (trimmed.length < 1) {
      setGroupSearchResults([])
      setShowGroupDropdown(false)
      return
    }
    groupDebounceRef.current = setTimeout(async () => {
      try {
        const results = await searchGroups(trimmed, 10)
        const existingIDs = new Set(groupAccessList.map((g) => g.groupId))
        const filtered = results.map((g) => ({ id: g.id, name: g.name })).filter((r) => !existingIDs.has(r.id))
        setGroupSearchResults(filtered)
        setShowGroupDropdown(filtered.length > 0)
      } catch {
        setGroupSearchResults([])
        setShowGroupDropdown(false)
      }
    }, 300)
  }

  const handleSelectGroup = (group: { id: string; name: string }) => {
    setGroupAccessList((prev) => [...prev, { groupId: group.id, name: group.name, role: 'viewer' }])
    setGroupSearch('')
    setGroupSearchResults([])
    setShowGroupDropdown(false)
  }

  const handleRemoveGroup = (groupId: string) => {
    setGroupAccessList((prev) => prev.filter((g) => g.groupId !== groupId))
  }

  const handleGroupRoleChange = (groupId: string, role: string) => {
    setGroupAccessList((prev) => prev.map((g) => (g.groupId === groupId ? { ...g, role } : g)))
  }

  const handleGeneralAccessChange = (value: string) => {
    if (value === 'none') {
      setGeneralAccessScope('none')
      setGeneralAccessRole('none')
    } else if (value === 'arca_users') {
      setGeneralAccessScope('arca_users')
      setGeneralAccessRole('viewer')
    }
  }

  const handleSave = async () => {
    setSaving(true)
    setError('')
    try {
      const result = await updateMachineSharing(
        machineID,
        members,
        { scope: generalAccessScope, role: generalAccessRole },
        groupAccessList,
      )
      setMembers(
        result.members.map((m) => ({ userId: m.userId, email: m.email, role: m.role })),
      )
      setGroupAccessList(
        result.groups.map((g) => ({ groupId: g.groupId, name: g.name, role: g.role })),
      )
      setGeneralAccessScope(result.generalAccess?.scope ?? 'none')
      setGeneralAccessRole(result.generalAccess?.role ?? 'none')
      onOpenChange(false)
    } catch (e) {
      setError(messageFromError(e))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>Share machine</DialogTitle>
          <DialogDescription>Manage who has access to this machine.</DialogDescription>
        </DialogHeader>

        {loading ? (
          <p className="text-sm text-muted-foreground">Loading...</p>
        ) : (
          <div className="space-y-5">
            <div className="space-y-3">
              <div className="relative" ref={wrapperRef}>
                <div className="flex items-center gap-2">
                  <Input
                    placeholder="Add by email"
                    value={newEmail}
                    onChange={(e) => handleEmailChange(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') {
                        e.preventDefault()
                        handleAddMember()
                      }
                      if (e.key === 'Escape') {
                        setShowDropdown(false)
                      }
                    }}
                    onBlur={(e) => {
                      if (!wrapperRef.current?.contains(e.relatedTarget as Node)) {
                        setShowDropdown(false)
                      }
                    }}
                    className="flex-1"
                    autoComplete="off"
                  />
                  <Button type="button" variant="secondary" className="h-9 px-3" onClick={handleAddMember}>
                    Add
                  </Button>
                </div>
                {showDropdown && searchResults.length > 0 && (
                  <div className="absolute left-0 right-0 z-50 mt-1 max-h-48 overflow-y-auto rounded-md border border-border bg-popover p-1 shadow-md">
                    {searchResults.map((user) => (
                      <button
                        key={user.id}
                        type="button"
                        className="w-full rounded-sm px-2 py-1.5 text-sm text-left text-popover-foreground hover:bg-accent hover:text-accent-foreground cursor-pointer"
                        onMouseDown={(e) => {
                          e.preventDefault()
                          handleSelectUser(user)
                        }}
                      >
                        {user.email}
                      </button>
                    ))}
                  </div>
                )}
              </div>

              {members.length > 0 && (
                <div className="space-y-2">
                  <p className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Members</p>
                  {members.map((member) => (
                    <div
                      key={member.email}
                      className="flex items-center justify-between gap-2 rounded-lg border border-border bg-muted/30 px-3 py-2"
                    >
                      <span className="text-sm text-foreground truncate">{member.email}</span>
                      <div className="flex items-center gap-2 shrink-0">
                        <select
                          value={member.role}
                          onChange={(e) => handleMemberRoleChange(member.email, e.target.value)}
                          className="h-8 rounded-md border border-input bg-background px-2 text-xs text-foreground"
                        >
                          <option value="admin">Admin</option>
                          <option value="editor">Editor</option>
                          <option value="viewer">Viewer</option>
                        </select>
                        <Button
                          type="button"
                          variant="secondary"
                          className="h-8 px-2 text-xs"
                          onClick={() => handleRemoveMember(member.email)}
                        >
                          Remove
                        </Button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>

            <div className="space-y-3">
              <div className="relative" ref={groupWrapperRef}>
                <div className="flex items-center gap-2">
                  <Input
                    placeholder="Add group by name"
                    value={groupSearch}
                    onChange={(e) => handleGroupSearchChange(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Escape') {
                        setShowGroupDropdown(false)
                      }
                    }}
                    onBlur={(e) => {
                      if (!groupWrapperRef.current?.contains(e.relatedTarget as Node)) {
                        setShowGroupDropdown(false)
                      }
                    }}
                    className="flex-1"
                    autoComplete="off"
                  />
                </div>
                {showGroupDropdown && groupSearchResults.length > 0 && (
                  <div className="absolute left-0 right-0 z-50 mt-1 max-h-48 overflow-y-auto rounded-md border border-border bg-popover p-1 shadow-md">
                    {groupSearchResults.map((group) => (
                      <button
                        key={group.id}
                        type="button"
                        className="w-full rounded-sm px-2 py-1.5 text-sm text-left text-popover-foreground hover:bg-accent hover:text-accent-foreground cursor-pointer"
                        onMouseDown={(e) => {
                          e.preventDefault()
                          handleSelectGroup(group)
                        }}
                      >
                        {group.name}
                      </button>
                    ))}
                  </div>
                )}
              </div>

              {groupAccessList.length > 0 && (
                <div className="space-y-2">
                  <p className="text-xs font-medium text-muted-foreground uppercase tracking-wider">Groups</p>
                  {groupAccessList.map((group) => (
                    <div
                      key={group.groupId}
                      className="flex items-center justify-between gap-2 rounded-lg border border-border bg-muted/30 px-3 py-2"
                    >
                      <span className="text-sm text-foreground truncate">{group.name}</span>
                      <div className="flex items-center gap-2 shrink-0">
                        <select
                          value={group.role}
                          onChange={(e) => handleGroupRoleChange(group.groupId, e.target.value)}
                          className="h-8 rounded-md border border-input bg-background px-2 text-xs text-foreground"
                        >
                          <option value="admin">Admin</option>
                          <option value="editor">Editor</option>
                          <option value="viewer">Viewer</option>
                        </select>
                        <Button
                          type="button"
                          variant="secondary"
                          className="h-8 px-2 text-xs"
                          onClick={() => handleRemoveGroup(group.groupId)}
                        >
                          Remove
                        </Button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>

            <div className="space-y-2">
              <p className="text-xs font-medium text-muted-foreground uppercase tracking-wider">General access</p>
              <div className="flex items-center gap-2">
                <select
                  value={generalAccessScope === 'none' ? 'none' : 'arca_users'}
                  onChange={(e) => handleGeneralAccessChange(e.target.value)}
                  className="h-9 flex-1 rounded-md border border-input bg-background px-3 text-sm text-foreground"
                >
                  <option value="none">Restricted</option>
                  <option value="arca_users">Anyone with an Arca account</option>
                </select>
                {generalAccessScope !== 'none' && (
                  <span className="text-xs text-muted-foreground shrink-0">Viewer</span>
                )}
              </div>
              <p className="text-xs text-muted-foreground">
                {generalAccessScope === 'none'
                  ? 'Only members listed above can access this machine.'
                  : 'Any authenticated Arca user can view this machine and its endpoints.'}
              </p>
            </div>

            <div className="flex items-center justify-end gap-2">
              <Button type="button" variant="secondary" onClick={() => onOpenChange(false)}>
                Cancel
              </Button>
              <Button type="button" onClick={() => void handleSave()} disabled={saving}>
                {saving ? 'Saving...' : 'Save'}
              </Button>
            </div>

            {error !== '' && (
              <p role="alert" className="rounded-md border border-red-400/30 bg-red-500/12 px-3 py-2 text-sm text-red-200">
                {error}
              </p>
            )}
          </div>
        )}
      </DialogContent>
    </Dialog>
  )
}
