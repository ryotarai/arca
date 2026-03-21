import { useEffect } from 'react'
import { useBlocker } from 'react-router-dom'

/**
 * Warns the user when navigating away with unsaved changes.
 *
 * - Shows a native `beforeunload` prompt on browser close/reload.
 * - Blocks in-app navigation via react-router's `useBlocker` and returns the
 *   blocker object so callers can render a confirmation dialog.
 */
export function useUnsavedChanges(hasChanges: boolean) {
  // Warn on browser close / reload
  useEffect(() => {
    if (!hasChanges) return
    const handler = (e: BeforeUnloadEvent) => {
      e.preventDefault()
    }
    window.addEventListener('beforeunload', handler)
    return () => window.removeEventListener('beforeunload', handler)
  }, [hasChanges])

  // Block in-app navigation
  const blocker = useBlocker(hasChanges)

  return blocker
}
