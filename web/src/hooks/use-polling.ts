import { useEffect, useRef, useState } from 'react'
import { messageFromError } from '@/lib/errors'

type UsePollingOptions = {
  intervalMs?: number
  enabled?: boolean
}

type UsePollingResult<T> = {
  data: T | null
  loading: boolean
  error: string
}

/**
 * Polls a fetcher function at a configurable interval.
 *
 * The fetcher is called immediately, then re-scheduled after each completion.
 * Overlapping calls are prevented via a running guard. The loop is torn down
 * on unmount or when `enabled` becomes false.
 *
 * Pass a stable `fetcher` reference (e.g. via useCallback) so the effect
 * does not restart on every render.
 */
export function usePolling<T>(
  fetcher: () => Promise<T>,
  options: UsePollingOptions = {},
): UsePollingResult<T> {
  const { intervalMs = 60000, enabled = true } = options
  const [data, setData] = useState<T | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const fetcherRef = useRef(fetcher)
  fetcherRef.current = fetcher

  useEffect(() => {
    if (!enabled) {
      setLoading(false)
      return
    }

    let cancelled = false
    let timer: number | null = null
    let running = false

    const run = async () => {
      if (cancelled || running) {
        return
      }
      running = true
      try {
        const result = await fetcherRef.current()
        if (!cancelled) {
          setData(result)
          setError('')
        }
      } catch (e) {
        if (!cancelled) {
          setError(messageFromError(e))
        }
      } finally {
        running = false
        if (!cancelled) {
          setLoading(false)
          timer = window.setTimeout(() => {
            void run()
          }, intervalMs)
        }
      }
    }

    void run()

    return () => {
      cancelled = true
      if (timer != null) {
        window.clearTimeout(timer)
      }
    }
  }, [intervalMs, enabled])

  return { data, loading, error }
}
