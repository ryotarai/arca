import { useEffect, useState } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import { getSetupStatus, logout, me } from '@/lib/api'
import type { SetupStatus, User } from '@/lib/types'
import { HomePage } from '@/pages/HomePage'
import { LoginPage } from '@/pages/LoginPage'
import { MachineDetailPage } from '@/pages/MachineDetailPage'
import { MachinesPage } from '@/pages/MachinesPage'
import { SettingsPage } from '@/pages/SettingsPage'
import { SetupPage } from '@/pages/SetupPage'

export function App() {
  const [loading, setLoading] = useState(true)
  const [user, setUser] = useState<User | null>(null)
  const [setupStatus, setSetupStatus] = useState<SetupStatus>({
    isConfigured: true,
    hasAdmin: true,
    cloudflareZoneID: '',
    baseDomain: '',
    domainPrefix: '',
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
      <Route path="/" element={<HomePage user={user} onLogout={handleLogout} />} />
      <Route path="/setup" element={<Navigate to="/" replace />} />
      <Route path="/login" element={<LoginPage user={user} onLogin={setUser} />} />
      <Route path="/machines" element={<MachinesPage user={user} onLogout={handleLogout} />} />
      <Route path="/machines/:machineID" element={<MachineDetailPage user={user} onLogout={handleLogout} />} />
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
      <Route path="*" element={<Navigate to="/" replace />} />
    </Routes>
  )
}
