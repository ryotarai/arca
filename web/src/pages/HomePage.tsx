import { Link } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import type { User } from '@/lib/types'

type HomePageProps = {
  user: User
}

export function HomePage({ user }: HomePageProps) {
  return (
    <main className="relative overflow-hidden px-4 py-8 sm:px-6 sm:py-10">
      <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_10%_5%,_rgba(56,189,248,0.14),_transparent_30%),radial-gradient(circle_at_90%_10%,_rgba(148,163,184,0.18),_transparent_42%)]" />
      <div className="pointer-events-none absolute inset-0 bg-[linear-gradient(rgba(255,255,255,0.04)_1px,transparent_1px),linear-gradient(90deg,rgba(255,255,255,0.04)_1px,transparent_1px)] bg-[size:44px_44px] [mask-image:radial-gradient(ellipse_at_center,black_40%,transparent_78%)]" />

      <section className="relative z-10 mx-auto flex w-full max-w-6xl flex-col gap-6">
        <header className="rounded-2xl border border-white/10 bg-white/[0.04] p-5 backdrop-blur-xl sm:p-6">
          <p className="text-xs font-medium uppercase tracking-[0.24em] text-slate-400">Overview</p>
          <h1 className="mt-2 text-2xl font-semibold text-white sm:text-3xl">Control your environment in one place</h1>
          <p className="mt-2 text-sm text-slate-300">Signed in as {user.email}</p>
          <p className="mt-3 max-w-2xl text-sm leading-6 text-slate-300 sm:text-base">
            Use quick actions to manage infrastructure and jump into operations without switching context.
          </p>
          <div className="mt-6 flex flex-wrap gap-3">
            <Button asChild type="button" className="bg-white text-slate-900 hover:bg-slate-100">
              <Link to="/machines">Open machine list</Link>
            </Button>
            <Button asChild type="button" variant="secondary">
              <Link to="/machines/create">Create machine</Link>
            </Button>
            <Button asChild type="button" variant="secondary">
              <Link to="/settings">Open settings</Link>
            </Button>
          </div>
        </header>

        <section className="grid gap-4 md:grid-cols-3">
          <Card className="border-white/15 bg-white/[0.04] py-0 shadow-xl shadow-black/25 backdrop-blur-xl">
            <CardHeader className="p-5 pb-2">
              <CardDescription className="text-slate-300">Machines</CardDescription>
              <CardTitle className="text-2xl text-white">Manage</CardTitle>
            </CardHeader>
            <CardContent className="p-5 pt-2 text-sm text-slate-300">
              Start, stop, and inspect machine status with real-time updates.
            </CardContent>
          </Card>
          <Card className="border-white/15 bg-white/[0.04] py-0 shadow-xl shadow-black/25 backdrop-blur-xl">
            <CardHeader className="p-5 pb-2">
              <CardDescription className="text-slate-300">Team</CardDescription>
              <CardTitle className="text-2xl text-white">Admin</CardTitle>
            </CardHeader>
            <CardContent className="p-5 pt-2 text-sm text-slate-300">
              Review users and maintain access controls for operators.
            </CardContent>
          </Card>
          <Card className="border-white/15 bg-white/[0.04] py-0 shadow-xl shadow-black/25 backdrop-blur-xl">
            <CardHeader className="p-5 pb-2">
              <CardDescription className="text-slate-300">Catalog</CardDescription>
              <CardTitle className="text-2xl text-white">Runtimes</CardTitle>
            </CardHeader>
            <CardContent className="p-5 pt-2 text-sm text-slate-300">
              Track available runtime versions and standardize deployments.
            </CardContent>
          </Card>
        </section>
      </section>
    </main>
  )
}
