import { useState, useEffect, useRef } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { searchUsers, startImpersonation } from '@/lib/api'

export function ImpersonationMenu() {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<{ id: string; email: string }[]>([])
  const [loading, setLoading] = useState(false)
  const debounceRef = useRef<ReturnType<typeof setTimeout>>()

  useEffect(() => {
    if (!open) {
      setQuery('')
      setResults([])
      return
    }
  }, [open])

  useEffect(() => {
    if (debounceRef.current) {
      clearTimeout(debounceRef.current)
    }
    if (query.trim().length < 1) {
      setResults([])
      return
    }
    debounceRef.current = setTimeout(async () => {
      setLoading(true)
      try {
        const users = await searchUsers(query, 10)
        setResults(users)
      } catch {
        setResults([])
      } finally {
        setLoading(false)
      }
    }, 300)
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [query])

  const handleSelect = async (userId: string) => {
    try {
      await startImpersonation(userId)
      window.location.reload()
    } catch (err) {
      alert(err instanceof Error ? err.message : 'Failed to start impersonation')
    }
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogTrigger asChild>
        <Button variant="ghost" size="sm" className="text-xs text-muted-foreground hover:text-foreground">
          Impersonate User
        </Button>
      </DialogTrigger>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Impersonate User</DialogTitle>
        </DialogHeader>
        <div className="space-y-3">
          <Input
            placeholder="Search by email..."
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            autoFocus
          />
          {loading && <p className="text-sm text-muted-foreground">Searching...</p>}
          {results.length > 0 && (
            <ul className="max-h-60 space-y-1 overflow-y-auto">
              {results.map((user) => (
                <li key={user.id}>
                  <button
                    type="button"
                    className="w-full rounded-md px-3 py-2 text-left text-sm hover:bg-muted"
                    onClick={() => handleSelect(user.id)}
                  >
                    {user.email}
                  </button>
                </li>
              ))}
            </ul>
          )}
          {!loading && query.trim().length >= 1 && results.length === 0 && (
            <p className="text-sm text-muted-foreground">No users found</p>
          )}
        </div>
      </DialogContent>
    </Dialog>
  )
}
