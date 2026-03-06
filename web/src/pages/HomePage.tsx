import { Link, Navigate } from 'react-router-dom'
import { Blocks, Cpu, Home, Settings, Users } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarRail,
  SidebarSeparator,
  SidebarTrigger,
} from '@/components/ui/sidebar'
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
    <main className="relative min-h-dvh overflow-hidden bg-slate-950 text-slate-100">
      <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(circle_at_10%_5%,_rgba(56,189,248,0.14),_transparent_30%),radial-gradient(circle_at_90%_10%,_rgba(148,163,184,0.18),_transparent_42%)]" />
      <div className="pointer-events-none absolute inset-0 bg-[linear-gradient(rgba(255,255,255,0.04)_1px,transparent_1px),linear-gradient(90deg,rgba(255,255,255,0.04)_1px,transparent_1px)] bg-[size:44px_44px] [mask-image:radial-gradient(ellipse_at_center,black_40%,transparent_78%)]" />

      <SidebarProvider defaultOpen>
        <Sidebar className="z-20 border-r border-white/10" collapsible="icon">
          <SidebarHeader className="p-4">
            <p className="text-xs font-medium uppercase tracking-[0.24em] text-slate-400">Arca</p>
            <p className="text-sm text-slate-300">{user.email}</p>
          </SidebarHeader>
          <SidebarSeparator />
          <SidebarContent>
            <SidebarGroup>
              <SidebarGroupLabel>Navigation</SidebarGroupLabel>
              <SidebarGroupContent>
                <SidebarMenu>
                  <SidebarMenuItem>
                    <SidebarMenuButton asChild isActive tooltip="Overview">
                      <Link to="/">
                        <Home />
                        <span>Overview</span>
                      </Link>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                  <SidebarMenuItem>
                    <SidebarMenuButton asChild tooltip="Machines">
                      <Link to="/machines">
                        <Cpu />
                        <span>Machines</span>
                      </Link>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                  <SidebarMenuItem>
                    <SidebarMenuButton asChild tooltip="Users">
                      <Link to="/users">
                        <Users />
                        <span>Users</span>
                      </Link>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                  <SidebarMenuItem>
                    <SidebarMenuButton asChild tooltip="Runtimes">
                      <Link to="/runtimes">
                        <Blocks />
                        <span>Runtimes</span>
                      </Link>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                  <SidebarMenuItem>
                    <SidebarMenuButton asChild tooltip="Settings">
                      <Link to="/settings">
                        <Settings />
                        <span>Settings</span>
                      </Link>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                </SidebarMenu>
              </SidebarGroupContent>
            </SidebarGroup>
          </SidebarContent>
          <SidebarFooter className="p-4">
            <Button type="button" variant="secondary" className="w-full" onClick={onLogout}>
              Logout
            </Button>
          </SidebarFooter>
          <SidebarRail />
        </Sidebar>

        <SidebarInset className="relative z-10 bg-transparent">
          <section className="mx-auto flex w-full max-w-6xl flex-1 flex-col gap-6 px-4 py-8 sm:px-6 sm:py-10">
            <header className="rounded-2xl border border-white/10 bg-white/[0.04] p-5 backdrop-blur-xl sm:p-6">
              <div className="mb-4 flex items-center justify-between">
                <div>
                  <p className="text-xs font-medium uppercase tracking-[0.24em] text-slate-400">Overview</p>
                  <h1 className="mt-2 text-2xl font-semibold text-white sm:text-3xl">Control your environment in one place</h1>
                </div>
                <SidebarTrigger className="bg-white/10 text-slate-100 hover:bg-white/20" />
              </div>
              <p className="max-w-2xl text-sm leading-6 text-slate-300 sm:text-base">
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
        </SidebarInset>
      </SidebarProvider>
    </main>
  )
}
