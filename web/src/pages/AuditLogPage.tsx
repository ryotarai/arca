import { useEffect, useState } from 'react'
import { Navigate } from 'react-router-dom'
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

type AuditLogPageProps = {
  user: User | null
  onLogout: () => Promise<void>
}

export function AuditLogPage({ user }: AuditLogPageProps) {
  const [logs, setLogs] = useState<AuditLog[]>([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    if (user?.role !== 'admin') return
    const load = async () => {
      try {
        const data = await listAuditLogs(200)
        setLogs(data)
      } catch {
        // ignore
      } finally {
        setLoading(false)
      }
    }
    void load()
  }, [user])

  if (user?.role !== 'admin') {
    return <Navigate to="/" replace />
  }

  const formatTime = (unixStr: string) => {
    const ts = Number(unixStr)
    if (isNaN(ts) || ts === 0) return '-'
    return new Date(ts * 1000).toLocaleString()
  }

  return (
    <main className="mx-auto max-w-6xl p-6 md:p-8">
      <h1 className="mb-6 text-2xl font-semibold">Audit Logs</h1>
      {loading ? (
        <p className="text-muted-foreground">Loading...</p>
      ) : logs.length === 0 ? (
        <p className="text-muted-foreground">No audit logs yet.</p>
      ) : (
        <div className="overflow-x-auto rounded-md border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Time</TableHead>
                <TableHead>Actor</TableHead>
                <TableHead>Acting As</TableHead>
                <TableHead>Action</TableHead>
                <TableHead>Resource</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {logs.map((log) => (
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
                    {log.resourceId ? ` / ${log.resourceId.slice(0, 8)}...` : ''}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </main>
  )
}
