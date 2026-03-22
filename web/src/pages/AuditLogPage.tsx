import { useCallback, useEffect, useState } from 'react'
import { Link, Navigate } from 'react-router-dom'
import { listAuditLogs } from '@/lib/api'
import type { AuditLog } from '@/lib/api'
import type { User } from '@/lib/types'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { PageHeader } from '@/components/PageHeader'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'

type AuditLogPageProps = {
  user: User | null
  onLogout: () => Promise<void>
}

const ACTION_PREFIXES = [
  { value: '', label: 'All actions' },
  { value: 'machine.', label: 'machine.*' },
  { value: 'user.', label: 'user.*' },
  { value: 'profile.', label: 'profile.*' },
  { value: 'image.', label: 'image.*' },
  { value: 'group.', label: 'group.*' },
  { value: 'sharing.', label: 'sharing.*' },
  { value: 'auth.', label: 'auth.*' },
  { value: 'admin.', label: 'admin.*' },
]

const PAGE_SIZE = 50

function resourceLink(resourceType: string, resourceId: string): string | null {
  if (!resourceId) return null
  switch (resourceType) {
    case 'machine':
      return `/machines/${resourceId}`
    case 'template':
    case 'profile':
      return `/machine-profiles/${resourceId}`
    default:
      return null
  }
}

export function AuditLogPage({ user }: AuditLogPageProps) {
  const [logs, setLogs] = useState<AuditLog[]>([])
  const [loading, setLoading] = useState(true)
  const [totalCount, setTotalCount] = useState(0)
  const [page, setPage] = useState(0)
  const [actionPrefix, setActionPrefix] = useState('')
  const [actorEmail, setActorEmail] = useState('')
  const [actorEmailInput, setActorEmailInput] = useState('')
  const [expandedDetails, setExpandedDetails] = useState<Set<string>>(new Set())

  const fetchLogs = useCallback(async (p: number, action: string, actor: string) => {
    setLoading(true)
    try {
      const result = await listAuditLogs({
        limit: PAGE_SIZE,
        offset: p * PAGE_SIZE,
        actionPrefix: action,
        actorEmail: actor,
      })
      setLogs(result.auditLogs)
      setTotalCount(result.totalCount)
    } catch {
      // ignore
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (user?.role !== 'admin') return
    void fetchLogs(page, actionPrefix, actorEmail)
  }, [user, page, actionPrefix, actorEmail, fetchLogs])

  if (user?.role !== 'admin') {
    return <Navigate to="/" replace />
  }

  const totalPages = Math.max(1, Math.ceil(totalCount / PAGE_SIZE))

  const formatTime = (unixStr: string) => {
    const ts = Number(unixStr)
    if (isNaN(ts) || ts === 0) return '-'
    return new Date(ts * 1000).toLocaleString()
  }

  const handleActionFilterChange = (value: string) => {
    setActionPrefix(value === 'all' ? '' : value)
    setPage(0)
  }

  const handleActorEmailSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    setActorEmail(actorEmailInput.trim())
    setPage(0)
  }

  const handleClearActorEmail = () => {
    setActorEmailInput('')
    setActorEmail('')
    setPage(0)
  }

  const toggleDetails = (id: string) => {
    setExpandedDetails(prev => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }

  const formatDetails = (json: string) => {
    try {
      return JSON.stringify(JSON.parse(json), null, 2)
    } catch {
      return json
    }
  }

  return (
    <main className="min-h-dvh px-6 py-10">
      <section className="mx-auto w-full max-w-6xl space-y-6">
      <PageHeader
        label="Admin"
        title="Audit Logs"
        description="View user and system activity across the workspace."
      />

      {/* Filters */}
      <div className="mb-4 flex flex-wrap items-end gap-3">
        <div className="w-48">
          <label className="mb-1 block text-xs text-muted-foreground">Action</label>
          <Select value={actionPrefix || 'all'} onValueChange={handleActionFilterChange}>
            <SelectTrigger>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {ACTION_PREFIXES.map(p => (
                <SelectItem key={p.value || 'all'} value={p.value || 'all'}>{p.label}</SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <form onSubmit={handleActorEmailSubmit} className="flex items-end gap-2">
          <div>
            <label className="mb-1 block text-xs text-muted-foreground">Actor email</label>
            <Input
              placeholder="Filter by email..."
              value={actorEmailInput}
              onChange={e => setActorEmailInput(e.target.value)}
              className="w-56"
            />
          </div>
          <Button type="submit" variant="secondary" size="sm">Filter</Button>
          {actorEmail && (
            <Button type="button" variant="ghost" size="sm" onClick={handleClearActorEmail}>Clear</Button>
          )}
        </form>
      </div>

      {loading ? (
        <p className="text-muted-foreground">Loading...</p>
      ) : logs.length === 0 ? (
        <p className="text-muted-foreground">No audit logs found.</p>
      ) : (
        <>
          <div className="overflow-x-auto rounded-md border border-border">
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Time</TableHead>
                  <TableHead>Actor</TableHead>
                  <TableHead>Acting As</TableHead>
                  <TableHead>Action</TableHead>
                  <TableHead>Resource</TableHead>
                  <TableHead>Details</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {logs.map((log) => {
                  const link = resourceLink(log.resourceType, log.resourceId)
                  const isExpanded = expandedDetails.has(log.id)
                  const hasDetails = log.details && log.details !== '{}'
                  return (
                    <TableRow key={log.id}>
                      <TableCell className="whitespace-nowrap text-xs text-muted-foreground">
                        {formatTime(log.createdAt)}
                      </TableCell>
                      <TableCell className="text-sm">{log.actorEmail}</TableCell>
                      <TableCell className="text-sm text-muted-foreground">
                        {log.actingAsEmail || '-'}
                      </TableCell>
                      <TableCell className="text-sm font-mono">{log.action}</TableCell>
                      <TableCell className="text-sm">
                        {log.resourceType}
                        {log.resourceId ? (
                          link ? (
                            <Link to={link} className="ml-1 text-blue-400 hover:underline" title={log.resourceId}>
                              {log.resourceId.slice(0, 8)}...
                            </Link>
                          ) : (
                            <span className="ml-1 text-muted-foreground" title={log.resourceId}>
                              {log.resourceId.slice(0, 8)}...
                            </span>
                          )
                        ) : ''}
                      </TableCell>
                      <TableCell className="text-sm">
                        {hasDetails ? (
                          <button
                            onClick={() => toggleDetails(log.id)}
                            className="text-xs text-blue-400 hover:underline cursor-pointer"
                          >
                            {isExpanded ? 'Hide' : 'Show'}
                          </button>
                        ) : (
                          <span className="text-xs text-muted-foreground">-</span>
                        )}
                        {isExpanded && hasDetails && (
                          <pre className="mt-1 max-w-xs overflow-x-auto rounded bg-muted/50 p-2 text-xs whitespace-pre-wrap">
                            {formatDetails(log.details)}
                          </pre>
                        )}
                      </TableCell>
                    </TableRow>
                  )
                })}
              </TableBody>
            </Table>
          </div>

          {/* Pagination */}
          <div className="mt-4 flex items-center justify-between text-sm text-muted-foreground">
            <span>
              Showing {page * PAGE_SIZE + 1}–{Math.min((page + 1) * PAGE_SIZE, totalCount)} of {totalCount}
            </span>
            <div className="flex gap-2">
              <Button
                variant="outline"
                size="sm"
                disabled={page === 0}
                onClick={() => setPage(p => p - 1)}
              >
                Previous
              </Button>
              <Button
                variant="outline"
                size="sm"
                disabled={page >= totalPages - 1}
                onClick={() => setPage(p => p + 1)}
              >
                Next
              </Button>
            </div>
          </div>
        </>
      )}
      </section>
    </main>
  )
}
