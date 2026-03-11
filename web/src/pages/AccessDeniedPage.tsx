import { useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { requestMachineAccess } from '@/lib/api'
import { messageFromError } from '@/lib/errors'

export function AccessDeniedPage() {
  const [searchParams] = useSearchParams()
  const machineID = searchParams.get('machine_id') ?? ''
  const [requested, setRequested] = useState(false)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const handleRequest = async () => {
    if (machineID === '') return
    setLoading(true)
    setError('')
    try {
      await requestMachineAccess(machineID)
      setRequested(true)
    } catch (e) {
      const msg = messageFromError(e)
      if (msg.toLowerCase().includes('already')) {
        setRequested(true)
      } else {
        setError(msg)
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <main className="flex min-h-dvh items-center justify-center px-4">
      <Card className="w-full max-w-md py-0 shadow-sm">
        <CardHeader className="space-y-2 p-6 pb-3">
          <CardTitle className="text-xl">Access required</CardTitle>
          <CardDescription>
            You do not have permission to access this machine.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4 p-6 pt-3">
          {requested ? (
            <div className="rounded-lg border border-sky-400/40 bg-sky-500/15 p-4 text-sm text-sky-200">
              Your request is pending. An admin will review it shortly.
            </div>
          ) : (
            <>
              {error !== '' && (
                <div className="rounded-lg border border-red-400/40 bg-red-500/15 p-3 text-sm text-red-200">
                  {error}
                </div>
              )}
              {machineID !== '' && (
                <Button
                  type="button"
                  className="w-full"
                  disabled={loading}
                  onClick={() => void handleRequest()}
                >
                  {loading ? 'Requesting...' : 'Request access'}
                </Button>
              )}
            </>
          )}
          <Button asChild type="button" variant="secondary" className="w-full">
            <Link to="/machines">Back to machines</Link>
          </Button>
        </CardContent>
      </Card>
    </main>
  )
}
