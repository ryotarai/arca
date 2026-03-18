import { useEffect, useState } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import { getImpersonationStatus, getSetupStatus, logout, me } from '@/lib/api'
import type { ImpersonationStatus, SetupStatus, User } from '@/lib/types'
import { LoginPage } from '@/pages/LoginPage'
import { CreateMachinePage } from '@/pages/CreateMachinePage'
import { MachineDetailPage } from '@/pages/MachineDetailPage'
import { MachinesPage } from '@/pages/MachinesPage'
import { OidcCallbackPage } from '@/pages/OidcCallbackPage'
import { RuntimesListPage } from '@/pages/RuntimesListPage'
import { RuntimeFormPage } from '@/pages/RuntimeFormPage'
import { RuntimeDetailPage } from '@/pages/RuntimeDetailPage'
import { SettingsPage } from '@/pages/SettingsPage'
import { AdminSettingsPage } from '@/pages/AdminSettingsPage'
import { SetupPage } from '@/pages/SetupPage'
import { AdminUsersPage } from '@/pages/AdminUsersPage'
import { UserSetupPage } from '@/pages/UserSetupPage'
import { AccessDeniedPage } from '@/pages/AccessDeniedPage'
import { GroupsPage } from '@/pages/GroupsPage'
import { AuditLogPage } from '@/pages/AuditLogPage'
import { CustomImagesPage } from '@/pages/CustomImagesPage'
import { ServerLLMModelsPage } from '@/pages/ServerLLMModelsPage'
import { AppLayout } from '@/pages/AppLayout'
import { ImpersonationBanner } from '@/components/ImpersonationBanner'

export function App() {
  const [loading, setLoading] = useState(true)
  const [user, setUser] = useState<User | null>(null)
  const [impersonation, setImpersonation] = useState<ImpersonationStatus | null>(null)
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
    passwordLoginDisabled: false,
    iapEnabled: false,
    iapAudience: '',
    iapAutoProvisioning: false,
    oidcAutoProvisioning: false,
    serverExposureMethod: 'cloudflare_tunnel',
    serverDomain: '',
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
              try {
                const impStatus = await getImpersonationStatus()
                if (impStatus.isImpersonating) {
                  setImpersonation(impStatus)
                }
              } catch {
                // ignore - endpoint may not exist on older servers
              }
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
              onSetupComplete={(zoneID) =>
                setSetupStatus({
                  isConfigured: true,
                  hasAdmin: true,
                  cloudflareZoneID: zoneID,
                  baseDomain: '',
                  domainPrefix: '',
                  internetPublicExposureDisabled: false,
                  oidcEnabled: false,
                  oidcIssuerURL: '',
                  oidcClientID: '',
                  oidcClientSecretConfigured: false,
                  oidcAllowedEmailDomains: [],
                  passwordLoginDisabled: false,
                  iapEnabled: false,
                  iapAudience: '',
                  iapAutoProvisioning: false,
                  oidcAutoProvisioning: false,
                  serverExposureMethod: 'cloudflare_tunnel',
                  serverDomain: '',
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
    <>
      {impersonation?.isImpersonating && (
        <ImpersonationBanner
          impersonatedUserEmail={impersonation.impersonatedUserEmail}
          originalUserEmail={impersonation.originalUserEmail}
        />
      )}
      <Routes>
        <Route path="/setup" element={<Navigate to="/" replace />} />
        <Route path="/login" element={<LoginPage user={user} onLogin={setUser} oidcEnabled={setupStatus.oidcEnabled} passwordLoginDisabled={setupStatus.passwordLoginDisabled} iapEnabled={setupStatus.iapEnabled} />} />
        <Route path="/login/oidc/callback" element={<OidcCallbackPage user={user} onLogin={setUser} />} />
        <Route path="/users/setup" element={<UserSetupPage user={user} />} />
        <Route path="/access-denied" element={<AccessDeniedPage />} />

        <Route element={<AppLayout user={user} onLogout={handleLogout} impersonation={impersonation} />}>
          <Route path="/" element={<Navigate to="/machines" replace />} />
          <Route path="/machines" element={<MachinesPage user={user} onLogout={handleLogout} />} />
          <Route path="/machines/create" element={<CreateMachinePage user={user} onLogout={handleLogout} />} />
          <Route path="/machines/:machineID" element={<MachineDetailPage user={user} onLogout={handleLogout} />} />
          <Route path="/users" element={<AdminUsersPage user={user} onLogout={handleLogout} />} />
          <Route path="/groups" element={<GroupsPage user={user} onLogout={handleLogout} />} />
          <Route path="/runtimes" element={<RuntimesListPage user={user} onLogout={handleLogout} />} />
          <Route path="/runtimes/new" element={<RuntimeFormPage user={user} onLogout={handleLogout} />} />
          <Route path="/runtimes/:runtimeID" element={<RuntimeDetailPage user={user} onLogout={handleLogout} />} />
          <Route path="/runtimes/:runtimeID/edit" element={<RuntimeFormPage user={user} onLogout={handleLogout} />} />
          <Route
            path="/settings"
            element={
              <SettingsPage
                user={user}
                onLogout={handleLogout}
              />
            }
          />
          <Route
            path="/admin/settings"
            element={
              <AdminSettingsPage
                user={user}
                setupStatus={setupStatus}
                onSetupStatusChange={(next) => setSetupStatus(next)}
                onLogout={handleLogout}
              />
            }
          />
          <Route path="/admin/images" element={<CustomImagesPage user={user} onLogout={handleLogout} />} />
          <Route path="/admin/llm-models" element={<ServerLLMModelsPage user={user} onLogout={handleLogout} />} />
          <Route path="/admin/audit-logs" element={<AuditLogPage user={user} onLogout={handleLogout} />} />
        </Route>

        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </>
  )
}
