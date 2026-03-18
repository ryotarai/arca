import { useEffect, useState } from 'react'
import { Link, Navigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { updateDomainSettings, getSlackConfig, updateSlackConfig, testSlackNotification } from '@/lib/api'
import type { SlackConfigData } from '@/lib/api'
import { messageFromError } from '@/lib/errors'
import type { SetupStatus, User } from '@/lib/types'

type AdminSettingsPageProps = {
  user: User | null
  setupStatus: SetupStatus
  onSetupStatusChange: (status: SetupStatus) => void
  onLogout: () => Promise<void>
}

export function AdminSettingsPage({ user, setupStatus, onSetupStatusChange, onLogout }: AdminSettingsPageProps) {
  const [serverDomain, setServerDomain] = useState(setupStatus.serverDomain)
  const [disableInternetPublicExposure, setDisableInternetPublicExposure] = useState(setupStatus.internetPublicExposureDisabled)
  const [passwordLoginDisabled, setPasswordLoginDisabled] = useState(setupStatus.passwordLoginDisabled)
  const [iapEnabled, setIapEnabled] = useState(setupStatus.iapEnabled)
  const [iapAudience, setIapAudience] = useState(setupStatus.iapAudience)
  const [iapAutoProvisioning, setIapAutoProvisioning] = useState(setupStatus.iapAutoProvisioning)
  const [oidcAutoProvisioning, setOidcAutoProvisioning] = useState(setupStatus.oidcAutoProvisioning)
  const [oidcEnabled, setOidcEnabled] = useState(setupStatus.oidcEnabled)
  const [oidcIssuerURL, setOidcIssuerURL] = useState(setupStatus.oidcIssuerURL)
  const [oidcClientID, setOidcClientID] = useState(setupStatus.oidcClientID)
  const [oidcClientSecret, setOidcClientSecret] = useState('')
  const [oidcAllowedEmailDomainsText, setOidcAllowedEmailDomainsText] = useState(setupStatus.oidcAllowedEmailDomains.join('\n'))
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    setServerDomain(setupStatus.serverDomain)
    setDisableInternetPublicExposure(setupStatus.internetPublicExposureDisabled)
    setIapEnabled(setupStatus.iapEnabled)
    setIapAudience(setupStatus.iapAudience)
    setIapAutoProvisioning(setupStatus.iapAutoProvisioning)
    setOidcAutoProvisioning(setupStatus.oidcAutoProvisioning)
    setOidcEnabled(setupStatus.oidcEnabled)
    setOidcIssuerURL(setupStatus.oidcIssuerURL)
    setOidcClientID(setupStatus.oidcClientID)
    setOidcAllowedEmailDomainsText(setupStatus.oidcAllowedEmailDomains.join('\n'))
  }, [setupStatus])

  if (user == null) {
    return <Navigate to="/login" replace />
  }

  if (user.role !== 'admin') {
    return <Navigate to="/settings" replace />
  }

  const submit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setError('')
    setSaved(false)
    setLoading(true)
    try {
      await updateDomainSettings(
        disableInternetPublicExposure,
        oidcEnabled,
        oidcIssuerURL.trim(),
        oidcClientID.trim(),
        oidcClientSecret,
        oidcAllowedEmailDomainsText
          .split(/\r?\n/)
          .map((value) => value.trim().toLowerCase())
          .filter((value) => value !== ''),
        false,
        serverDomain.trim(),
        passwordLoginDisabled,
        iapEnabled,
        iapAudience.trim(),
        iapAutoProvisioning,
        oidcAutoProvisioning,
      )
      const normalizedOidcAllowedEmailDomains = oidcAllowedEmailDomainsText
        .split(/\r?\n/)
        .map((value) => value.trim().toLowerCase())
        .filter((value) => value !== '')
      onSetupStatusChange({
        ...setupStatus,
        internetPublicExposureDisabled: disableInternetPublicExposure,
        passwordLoginDisabled,
        iapEnabled,
        iapAudience: iapAudience.trim(),
        iapAutoProvisioning,
        oidcAutoProvisioning,
        oidcEnabled,
        oidcIssuerURL: oidcIssuerURL.trim(),
        oidcClientID: oidcClientID.trim(),
        oidcClientSecretConfigured: setupStatus.oidcClientSecretConfigured || oidcClientSecret !== '',
        oidcAllowedEmailDomains: normalizedOidcAllowedEmailDomains,
        serverDomain: serverDomain.trim(),
      })
      setOidcClientSecret('')
      setSaved(true)
    } catch (e) {
      setError(messageFromError(e))
    } finally {
      setLoading(false)
    }
  }

  return (
    <main className="min-h-dvh px-6 py-10">
      <section className="mx-auto w-full max-w-3xl space-y-6">
        <header className="flex flex-col items-start justify-between gap-4 rounded-xl border border-border bg-muted/30 p-6 md:flex-row md:items-center">
          <div>
            <p className="text-xs font-medium uppercase tracking-[0.24em] text-muted-foreground">Arca</p>
            <h1 className="mt-2 text-2xl font-semibold text-foreground">Admin settings</h1>
            <p className="mt-1 text-sm text-muted-foreground">Update domain and authentication settings for the workspace.</p>
          </div>
          <div className="flex items-center gap-3">
            <Button asChild type="button" variant="secondary">
              <Link to="/machines">Back</Link>
            </Button>
            <Button type="button" variant="secondary" onClick={onLogout}>
              Logout
            </Button>
          </div>
        </header>

        <Card className="py-0 shadow-sm">
          <CardHeader className="space-y-2 p-6 pb-3">
            <CardTitle className="text-xl">Domain settings</CardTitle>
            <CardDescription>
              Existing machine hostnames stay unchanged. New machines use this configuration.
            </CardDescription>
          </CardHeader>
          <CardContent className="p-6 pt-3">
            <form className="space-y-4" onSubmit={submit}>
              <div className="space-y-2">
                <Label htmlFor="settings-server-domain">
                  Server domain
                </Label>
                <Input
                  id="settings-server-domain"
                  value={serverDomain}
                  onChange={(event) => setServerDomain(event.target.value)}
                  className="h-10"
                  placeholder="arca.example.com"
                />
                <p className="text-xs text-muted-foreground">The domain where machines reach this server.</p>
              </div>

              <div className="space-y-2">
                <Label htmlFor="settings-disable-internet-public">
                  Internet public exposure
                </Label>
                <label className="flex items-center gap-2 rounded-md border border-border bg-muted/30 px-3 py-2 text-sm text-foreground">
                  <input
                    id="settings-disable-internet-public"
                    type="checkbox"
                    checked={disableInternetPublicExposure}
                    onChange={(event) => setDisableInternetPublicExposure(event.target.checked)}
                  />
                  Disable internet-public endpoint visibility
                </label>
                <p className="text-xs text-muted-foreground">
                  When enabled, users cannot set endpoint visibility to internet public.
                </p>
              </div>
              <div className="space-y-2 rounded-md border border-border bg-muted/30 p-4">
                <p className="text-sm font-medium text-foreground">Password login</p>
                <label className="flex items-center gap-2 text-sm text-foreground">
                  <input
                    id="settings-password-login-enabled"
                    type="checkbox"
                    checked={!passwordLoginDisabled}
                    onChange={(event) => setPasswordLoginDisabled(!event.target.checked)}
                  />
                  Enable password login
                </label>
                <p className="text-xs text-muted-foreground">
                  Recovery override: set <code>ARCA_ALLOW_PASSWORD_LOGIN=1</code> env var on the server.
                </p>
              </div>

              <div className="space-y-2 rounded-md border border-border bg-muted/30 p-4">
                <p className="text-sm font-medium text-foreground">Google/OIDC login</p>
                <label className="flex items-center gap-2 text-sm text-foreground">
                  <input
                    id="settings-oidc-enabled"
                    type="checkbox"
                    checked={oidcEnabled}
                    onChange={(event) => setOidcEnabled(event.target.checked)}
                  />
                  Enable OIDC login
                </label>
                <div className="space-y-2">
                  <Label htmlFor="settings-oidc-issuer">
                    OIDC issuer URL
                  </Label>
                  <Input
                    id="settings-oidc-issuer"
                    value={oidcIssuerURL}
                    onChange={(event) => setOidcIssuerURL(event.target.value)}
                    className="h-10"
                    placeholder="https://accounts.google.com"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="settings-oidc-client-id">
                    OIDC client ID
                  </Label>
                  <Input
                    id="settings-oidc-client-id"
                    value={oidcClientID}
                    onChange={(event) => setOidcClientID(event.target.value)}
                    className="h-10"
                    placeholder="your-client-id.apps.googleusercontent.com"
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="settings-oidc-client-secret">
                    OIDC client secret
                  </Label>
                  <Input
                    id="settings-oidc-client-secret"
                    type="password"
                    value={oidcClientSecret}
                    onChange={(event) => setOidcClientSecret(event.target.value)}
                    className="h-10"
                    placeholder={setupStatus.oidcClientSecretConfigured ? 'Leave empty to keep current secret' : 'Enter client secret'}
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="settings-oidc-domains">
                    Allowed email domains (one per line)
                  </Label>
                  <textarea
                    id="settings-oidc-domains"
                    value={oidcAllowedEmailDomainsText}
                    onChange={(event) => setOidcAllowedEmailDomainsText(event.target.value)}
                    rows={4}
                    className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                    placeholder={'example.com\ncorp.example.com'}
                  />
                  <p className="text-xs text-muted-foreground">
                    Leave empty to allow any verified email domain.
                  </p>
                </div>
                <label className="flex items-center gap-2 text-sm text-foreground">
                  <input
                    type="checkbox"
                    checked={oidcAutoProvisioning}
                    onChange={(event) => setOidcAutoProvisioning(event.target.checked)}
                  />
                  Auto-provision users authenticated via OIDC
                </label>
                <p className="text-xs text-muted-foreground">
                  When enabled, users who pass OIDC authentication are automatically created if they don't exist.
                </p>
              </div>

              <div className="space-y-2 rounded-md border border-border bg-muted/30 p-4">
                <p className="text-sm font-medium text-foreground">Cloud IAP authentication</p>
                <label className="flex items-center gap-2 text-sm text-foreground">
                  <input
                    id="settings-iap-enabled"
                    type="checkbox"
                    checked={iapEnabled}
                    onChange={(event) => setIapEnabled(event.target.checked)}
                  />
                  Enable Cloud IAP authentication
                </label>
                <div className="space-y-2">
                  <Label htmlFor="settings-iap-audience">
                    IAP audience
                  </Label>
                  <Input
                    id="settings-iap-audience"
                    value={iapAudience}
                    onChange={(event) => setIapAudience(event.target.value)}
                    className="h-10"
                    placeholder="/projects/PROJECT_NUMBER/global/backendServices/BACKEND_SERVICE_ID"
                  />
                  <p className="text-xs text-muted-foreground">
                    Configure IAP in Google Cloud Console. The audience string is found in the IAP settings.
                  </p>
                </div>
                <label className="flex items-center gap-2 text-sm text-foreground">
                  <input
                    type="checkbox"
                    checked={iapAutoProvisioning}
                    onChange={(event) => setIapAutoProvisioning(event.target.checked)}
                  />
                  Auto-provision users authenticated via IAP
                </label>
                <p className="text-xs text-muted-foreground">
                  When enabled, users who pass IAP authentication are automatically created if they don't exist.
                </p>
              </div>
              <Button
                type="submit"
                className="h-10 w-full"
                disabled={loading}
              >
                {loading ? 'Saving...' : 'Save settings'}
              </Button>
            </form>
            {saved && <p className="mt-3 text-sm text-emerald-300">Settings updated.</p>}
            {error !== '' && <p className="mt-3 text-sm text-red-300">{error}</p>}
          </CardContent>
        </Card>

        <SlackSettingsCard />
      </section>
    </main>
  )
}

function SlackSettingsCard() {
  const [slackEnabled, setSlackEnabled] = useState(false)
  const [slackBotToken, setSlackBotToken] = useState('')
  const [slackDefaultChannelId, setSlackDefaultChannelId] = useState('')
  const [slackBotTokenConfigured, setSlackBotTokenConfigured] = useState(false)
  const [slackLoading, setSlackLoading] = useState(true)
  const [slackSaving, setSlackSaving] = useState(false)
  const [slackTesting, setSlackTesting] = useState(false)
  const [slackError, setSlackError] = useState('')
  const [slackSaved, setSlackSaved] = useState(false)
  const [slackTestResult, setSlackTestResult] = useState('')

  useEffect(() => {
    let cancelled = false
    const load = async () => {
      try {
        const config: SlackConfigData = await getSlackConfig()
        if (cancelled) return
        setSlackEnabled(config.enabled)
        setSlackDefaultChannelId(config.defaultChannelId)
        setSlackBotTokenConfigured(config.botTokenConfigured)
      } catch (e) {
        if (!cancelled) setSlackError(messageFromError(e))
      } finally {
        if (!cancelled) setSlackLoading(false)
      }
    }
    void load()
    return () => { cancelled = true }
  }, [])

  const submitSlack = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setSlackError('')
    setSlackSaved(false)
    setSlackSaving(true)
    try {
      const result = await updateSlackConfig({
        enabled: slackEnabled,
        botToken: slackBotToken,
        defaultChannelId: slackDefaultChannelId.trim(),
      })
      setSlackBotTokenConfigured(result.botTokenConfigured)
      setSlackDefaultChannelId(result.defaultChannelId)
      setSlackEnabled(result.enabled)
      setSlackBotToken('')
      setSlackSaved(true)
    } catch (e) {
      setSlackError(messageFromError(e))
    } finally {
      setSlackSaving(false)
    }
  }

  const handleTestSlack = async () => {
    setSlackTestResult('')
    setSlackTesting(true)
    try {
      await testSlackNotification(slackDefaultChannelId.trim())
      setSlackTestResult('Test notification sent successfully.')
    } catch (e) {
      setSlackTestResult(messageFromError(e))
    } finally {
      setSlackTesting(false)
    }
  }

  return (
    <Card className="py-0 shadow-sm">
      <CardHeader className="space-y-2 p-6 pb-3">
        <CardTitle className="text-xl">Slack notifications</CardTitle>
        <CardDescription>
          Send machine lifecycle notifications (ready, auto-stop, failures) to users via Slack.
          Requires a Slack App with <code>chat:write</code> bot token scope.
        </CardDescription>
      </CardHeader>
      <CardContent className="p-6 pt-3">
        {slackLoading ? (
          <p className="text-sm text-muted-foreground">Loading...</p>
        ) : (
          <form className="space-y-4" onSubmit={submitSlack}>
            <label className="flex items-center gap-2 rounded-md border border-border bg-muted/30 px-3 py-2 text-sm text-foreground">
              <input
                type="checkbox"
                checked={slackEnabled}
                onChange={(e) => setSlackEnabled(e.target.checked)}
              />
              Enable Slack notifications
            </label>

            <div className="space-y-2">
              <Label htmlFor="settings-slack-bot-token">Bot token</Label>
              <Input
                id="settings-slack-bot-token"
                type="password"
                value={slackBotToken}
                onChange={(e) => setSlackBotToken(e.target.value)}
                className="h-10"
                placeholder={slackBotTokenConfigured ? 'Leave empty to keep current token' : 'xoxb-...'}
              />
              <p className="text-xs text-muted-foreground">
                Bot User OAuth Token from your Slack App settings.
              </p>
            </div>

            <div className="space-y-2">
              <Label htmlFor="settings-slack-channel-id">Default channel ID</Label>
              <Input
                id="settings-slack-channel-id"
                value={slackDefaultChannelId}
                onChange={(e) => setSlackDefaultChannelId(e.target.value)}
                className="h-10"
                placeholder="C0123456789"
              />
              <p className="text-xs text-muted-foreground">
                Fallback channel for test notifications. User notifications are sent as DMs to each user's Slack ID.
              </p>
            </div>

            <div className="flex gap-3">
              <Button type="submit" className="h-10 flex-1" disabled={slackSaving}>
                {slackSaving ? 'Saving...' : 'Save Slack settings'}
              </Button>
              <Button
                type="button"
                variant="secondary"
                className="h-10"
                disabled={slackTesting || !slackBotTokenConfigured}
                onClick={handleTestSlack}
              >
                {slackTesting ? 'Sending...' : 'Send test'}
              </Button>
            </div>
          </form>
        )}
        {slackSaved && <p className="mt-3 text-sm text-emerald-300">Slack settings updated.</p>}
        {slackError !== '' && <p className="mt-3 text-sm text-red-300">{slackError}</p>}
        {slackTestResult !== '' && (
          <p className={`mt-3 text-sm ${slackTestResult.startsWith('Test notification') ? 'text-emerald-300' : 'text-red-300'}`}>
            {slackTestResult}
          </p>
        )}
      </CardContent>
    </Card>
  )
}
