import { useEffect } from 'react'

/**
 * Warns the user when navigating away with unsaved changes.
 *
 * Shows a native `beforeunload` prompt on browser close/reload/navigation.
 */
export function useUnsavedChanges(hasChanges: boolean) {
  useEffect(() => {
    if (!hasChanges) return
    const handler = (e: BeforeUnloadEvent) => {
      e.preventDefault()
    }
    window.addEventListener('beforeunload', handler)
    return () => window.removeEventListener('beforeunload', handler)
  }, [hasChanges])
}
