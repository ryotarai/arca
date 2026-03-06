import { useEffect, useState } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import { getSetupStatus, logout, me } from '@/lib/api'
import type { SetupStatus, User } from '@/lib/types'
import { LoginPage } from '@/pages/LoginPage'
import { CreateMachinePage } from '@/pages/CreateMachinePage'
import { MachineDetailPage } from '@/pages/MachineDetailPage'
import { MachineEditPage } from '@/pages/MachineEditPage'
import { MachinesPage } from '@/pages/MachinesPage'
import { OidcCallbackPage } from '@/pages/OidcCallbackPage'
import { RuntimeCatalogPage } from '@/pages/RuntimeCatalogPage'
import { RuntimeDetailPage } from '@/pages/RuntimeDetailPage'
import { SettingsPage } from '@/pages/SettingsPage'
import { SetupPage } from '@/pages/SetupPage'
import { AdminUsersPage } from '@/pages/AdminUsersPage'
import { UserSetupPage } from '@/pages/UserSetupPage'
import { AppLayout } from '@/pages/AppLayout'

export function App() {
  const [loading, setLoading] = useState(true)
  const [user, setUser] = useState<User | null>(null)
  const [setupStatus, setSetupStatus] = useState<SetupStatus>({
    isConfigured: true,
    hasAdmin: true,
    cloudflareZoneID: '',
    baseDomain: '',
    domainPrefix: '',
    internetPublicExposureDisabled: false,
    oidcEnabled: false,
    oidcIssuerURL: '',
    oidcClientID: '',
    oidcClientSecretConfigured: false,
    oidcAllowedEmailDomains: [],
  })

  useEffect(() => {
    const run = async () => {
      try {
        const status = await getSetupStatus()
        setSetupStatus(status)

        if (status.isConfigured) {
          try {
            const meUser = await me()
            if (meUser != null) {
              setUser(meUser)
            }
          } catch {
          }
        }
      } finally {
        setLoading(false)
      }
    }
    void run()
  }, [])

  const handleLogout = async () => {
    try {
      await logout()
    } finally {
      setUser(null)
    }
  }

  if (loading) {
    return (
      <main>
        <p>Loading...</p>
      </main>
    )
  }

  if (!setupStatus.isConfigured) {
    return (
      <Routes>
        <Route
          path="/setup"
          element={
            <SetupPage
              hasAdmin={setupStatus.hasAdmin}
              initialCloudflareZoneID={setupStatus.cloudflareZoneID}
              onAdminReady={setUser}
              onSetupComplete={(zoneID, baseDomain, domainPrefix) =>
                setSetupStatus({
                  isConfigured: true,
                  hasAdmin: true,
                  cloudflareZoneID: zoneID,
                  baseDomain,
                  domainPrefix,
                  internetPublicExposureDisabled: false,
                  oidcEnabled: false,
                  oidcIssuerURL: '',
                  oidcClientID: '',
                  oidcClientSecretConfigured: false,
                  oidcAllowedEmailDomains: [],
                })
              }
            />
          }
        />
        <Route path="*" element={<Navigate to="/setup" replace />} />
      </Routes>
    )
  }

  return (
    <Routes>
      <Route path="/setup" element={<Navigate to="/" replace />} />
      <Route path="/login" element={<LoginPage user={user} onLogin={setUser} oidcEnabled={setupStatus.oidcEnabled} />} />
      <Route path="/login/oidc/callback" element={<OidcCallbackPage user={user} onLogin={setUser} />} />
      <Route path="/users/setup" element={<UserSetupPage user={user} />} />

      <Route element={<AppLayout user={user} onLogout={handleLogout} />}>
        <Route path="/" element={<Navigate to="/machines" replace />} />
        <Route path="/machines" element={<MachinesPage user={user} onLogout={handleLogout} />} />
        <Route path="/machines/create" element={<CreateMachinePage user={user} onLogout={handleLogout} />} />
        <Route path="/machines/:machineID" element={<MachineDetailPage user={user} onLogout={handleLogout} />} />
        <Route path="/machines/:machineID/edit" element={<MachineEditPage user={user} onLogout={handleLogout} />} />
        <Route path="/users" element={<AdminUsersPage user={user} onLogout={handleLogout} />} />
        <Route path="/runtimes" element={<RuntimeCatalogPage user={user} onLogout={handleLogout} />} />
        <Route path="/runtimes/:runtimeID" element={<RuntimeDetailPage user={user} onLogout={handleLogout} />} />
        <Route
          path="/settings"
          element={
            <SettingsPage
              user={user}
              setupStatus={setupStatus}
              onSetupStatusChange={(next) => setSetupStatus(next)}
              onLogout={handleLogout}
            />
          }
        />
      </Route>

      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
