import { Link, Navigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import type { User } from '@/lib/types'

type HomePageProps = {
  user: User | null
  onLogout: () => Promise<void>
}

export function HomePage({ user, onLogout }: HomePageProps) {
  if (user == null) {
    return (
      <main className="flex min-h-dvh items-center justify-center bg-[radial-gradient(circle_at_top_left,_#f8fafc_10%,_#e2e8f0_55%,_#cbd5e1_100%)] px-6">
        <div className="w-full max-w-md rounded-2xl border border-slate-300/70 bg-white/90 p-10 shadow-xl shadow-slate-900/10 backdrop-blur">
          <p className="text-xs font-medium uppercase tracking-[0.24em] text-slate-500">Arca</p>
          <h1 className="mt-2 text-3xl font-semibold text-slate-900">Welcome back</h1>
          <p className="mt-3 text-sm text-slate-600">Sign in to access your workspace.</p>
          <Button asChild className="mt-8 w-full">
            <Link to="/login">Login</Link>
          </Button>
        </div>
      </main>
    )
  }

  return (
    <main className="flex min-h-dvh items-center justify-center bg-slate-950 px-6 py-16">
      <div className="w-full max-w-lg rounded-2xl border border-white/10 bg-white/[0.03] p-8 text-slate-100 shadow-2xl shadow-black/40 backdrop-blur">
        <p className="text-xs font-medium uppercase tracking-[0.24em] text-slate-400">Arca</p>
        <h1 className="mt-2 text-2xl font-semibold">Dashboard</h1>
        <p className="mt-3 text-sm text-slate-300">Signed in as {user.email}</p>
        <div className="mt-6 flex items-center gap-3">
          <Button asChild type="button">
            <Link to="/machines">Machines</Link>
          </Button>
          <Button type="button" variant="secondary" onClick={onLogout}>
            Logout
          </Button>
        </div>
      </div>
    </main>
  )
}
