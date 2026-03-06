import { Link } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import type { User } from '@/lib/types'

type HomePageProps = {
  user: User
}

export function HomePage({ user }: HomePageProps) {
  return (
    <main className="px-4 py-8 sm:px-6 sm:py-10">
      <section className="mx-auto flex w-full max-w-6xl flex-col gap-6">
        <header className="rounded-xl border border-border bg-muted/30 p-5 sm:p-6">
          <p className="text-xs font-medium uppercase tracking-[0.24em] text-muted-foreground">Overview</p>
          <h1 className="mt-2 text-2xl font-semibold text-foreground sm:text-3xl">Control your environment in one place</h1>
          <p className="mt-2 text-sm text-muted-foreground">Signed in as {user.email}</p>
          <p className="mt-3 max-w-2xl text-sm leading-6 text-muted-foreground sm:text-base">
            Use quick actions to manage infrastructure and jump into operations without switching context.
          </p>
          <div className="mt-6 flex flex-wrap gap-3">
            <Button asChild type="button">
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
          <Card className="py-0 shadow-sm">
            <CardHeader className="p-5 pb-2">
              <CardDescription>Machines</CardDescription>
              <CardTitle className="text-2xl">Manage</CardTitle>
            </CardHeader>
            <CardContent className="p-5 pt-2 text-sm text-muted-foreground">
              Start, stop, and inspect machine status with real-time updates.
            </CardContent>
          </Card>
          <Card className="py-0 shadow-sm">
            <CardHeader className="p-5 pb-2">
              <CardDescription>Team</CardDescription>
              <CardTitle className="text-2xl">Admin</CardTitle>
            </CardHeader>
            <CardContent className="p-5 pt-2 text-sm text-muted-foreground">
              Review users and maintain access controls for operators.
            </CardContent>
          </Card>
          <Card className="py-0 shadow-sm">
            <CardHeader className="p-5 pb-2">
              <CardDescription>Catalog</CardDescription>
              <CardTitle className="text-2xl">Runtimes</CardTitle>
            </CardHeader>
            <CardContent className="p-5 pt-2 text-sm text-muted-foreground">
              Track available runtime versions and standardize deployments.
            </CardContent>
          </Card>
        </section>
      </section>
    </main>
  )
}
