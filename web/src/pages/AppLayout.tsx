import { Blocks, Cpu, Home, Settings, Users } from 'lucide-react'
import { NavLink, Navigate, Outlet, useLocation } from 'react-router-dom'
import { Button } from '@/components/ui/button'
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

type AppLayoutProps = {
  user: User | null
  onLogout: () => Promise<void>
}

const navItems = [
  { to: '/', label: 'Overview', icon: Home },
  { to: '/machines', label: 'Machines', icon: Cpu },
]

const adminNavItems = [
  { to: '/runtimes', label: 'Runtimes', icon: Blocks },
  { to: '/users', label: 'Users', icon: Users },
  { to: '/settings', label: 'Settings', icon: Settings },
]

export function AppLayout({ user, onLogout }: AppLayoutProps) {
  const location = useLocation()

  if (user == null) {
    const next = `${location.pathname}${location.search}${location.hash}`
    return <Navigate to={`/login?next=${encodeURIComponent(next)}`} replace />
  }

  return (
    <SidebarProvider defaultOpen className="dark">
      <Sidebar className="z-40 border-r border-white/10" collapsible="icon">
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
                {navItems.map((item) => (
                  <SidebarMenuItem key={item.to}>
                    <SidebarMenuButton asChild isActive={location.pathname === item.to} tooltip={item.label}>
                      <NavLink to={item.to}>
                        <item.icon />
                        <span>{item.label}</span>
                      </NavLink>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                ))}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
          <SidebarGroup>
            <SidebarGroupLabel>Admin</SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                {adminNavItems.map((item) => (
                  <SidebarMenuItem key={item.to}>
                    <SidebarMenuButton asChild isActive={location.pathname === item.to} tooltip={item.label}>
                      <NavLink to={item.to}>
                        <item.icon />
                        <span>{item.label}</span>
                      </NavLink>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                ))}
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

      <SidebarInset className="bg-slate-950 text-slate-100">
        <header className="sticky top-0 z-30 flex h-14 items-center gap-2 border-b border-white/10 bg-slate-950/80 px-4 backdrop-blur md:px-6">
          <SidebarTrigger className="bg-white/10 text-slate-100 hover:bg-white/20" />
          <p className="text-sm text-slate-300">Arca workspace</p>
        </header>
        <Outlet />
      </SidebarInset>
    </SidebarProvider>
  )
}
