import { Button } from '@/components/ui/button'
import { stopImpersonation } from '@/lib/api'
import { useState } from 'react'

type ImpersonationBannerProps = {
  impersonatedUserEmail: string
  originalUserEmail: string
}

export function ImpersonationBanner({ impersonatedUserEmail, originalUserEmail }: ImpersonationBannerProps) {
  const [exiting, setExiting] = useState(false)

  const handleExit = async () => {
    setExiting(true)
    try {
      await stopImpersonation()
      window.location.reload()
    } catch {
      setExiting(false)
    }
  }

  return (
    <div className="sticky top-0 z-50 flex items-center justify-center gap-3 bg-amber-600 px-4 py-2 text-sm font-medium text-white">
      <span>
        You are impersonating <strong>{impersonatedUserEmail}</strong>
        <span className="ml-1 text-amber-100">(signed in as {originalUserEmail})</span>
      </span>
      <Button
        size="sm"
        variant="secondary"
        className="h-7 bg-white/20 text-white hover:bg-white/30"
        onClick={handleExit}
        disabled={exiting}
      >
        {exiting ? 'Exiting...' : 'Exit Impersonation'}
      </Button>
    </div>
  )
}
