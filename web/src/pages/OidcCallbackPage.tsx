import { useEffect, useState } from 'react'
import { Navigate, useNavigate, useSearchParams } from 'react-router-dom'
import { completeOidcLogin } from '@/lib/api'
import { messageFromError } from '@/lib/errors'
import type { User } from '@/lib/types'

type OidcCallbackPageProps = {
  user: User | null
  onLogin: (user: User) => void
}

export function OidcCallbackPage({ user, onLogin }: OidcCallbackPageProps) {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const [error, setError] = useState('')

  useEffect(() => {
    if (user != null) {
      void navigate('/', { replace: true })
      return
    }
    const run = async () => {
      const code = searchParams.get('code') ?? ''
      const state = searchParams.get('state') ?? ''
      if (code.trim() === '' || state.trim() === '') {
        setError('oidc callback parameters are missing')
        return
      }
      try {
        const redirectUri = `${window.location.origin}/login/oidc/callback`
        const loggedIn = await completeOidcLogin(code, state, redirectUri)
        if (loggedIn == null) {
          setError('oidc login failed')
          return
        }
        onLogin(loggedIn)
        void navigate('/', { replace: true })
      } catch (e) {
        setError(messageFromError(e))
      }
    }
    void run()
  }, [navigate, onLogin, searchParams, user])

  if (user != null) {
    return <Navigate to="/" replace />
  }

  return (
    <main className="relative flex min-h-dvh items-center justify-center overflow-hidden bg-slate-950 px-6 py-16 text-slate-100">
      <section className="w-full max-w-md rounded-xl border border-white/10 bg-white/[0.03] p-6">
        <h1 className="text-lg font-semibold">Signing in...</h1>
        {error !== '' && <p className="mt-3 text-sm text-red-300">{error}</p>}
      </section>
    </main>
  )
}
