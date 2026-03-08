import { useState } from 'react'
import { Link, Navigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { updateDomainSettings } from '@/lib/api'
import {
  normalizeBaseDomainInput,
  normalizeDomainPrefixInput,
  validateBaseDomainInput,
  validateDomainPrefixInput,
} from '@/lib/domainValidation'
import { messageFromError } from '@/lib/errors'
import type { SetupStatus, User } from '@/lib/types'

type SettingsPageProps = {
  user: User | null
  setupStatus: SetupStatus
  onSetupStatusChange: (status: SetupStatus) => void
  onLogout: () => Promise<void>
}

export function SettingsPage({ user, setupStatus, onSetupStatusChange, onLogout }: SettingsPageProps) {
  const [baseDomain, setBaseDomain] = useState(setupStatus.baseDomain)
  const [domainPrefix, setDomainPrefix] = useState(setupStatus.domainPrefix)
  const [disableInternetPublicExposure, setDisableInternetPublicExposure] = useState(setupStatus.internetPublicExposureDisabled)
  const [oidcEnabled, setOidcEnabled] = useState(setupStatus.oidcEnabled)
  const [oidcIssuerURL, setOidcIssuerURL] = useState(setupStatus.oidcIssuerURL)
  const [oidcClientID, setOidcClientID] = useState(setupStatus.oidcClientID)
  const [oidcClientSecret, setOidcClientSecret] = useState('')
  const [oidcAllowedEmailDomainsText, setOidcAllowedEmailDomainsText] = useState(setupStatus.oidcAllowedEmailDomains.join('\n'))
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [saved, setSaved] = useState(false)
  const baseDomainError = validateBaseDomainInput(baseDomain)
  const domainPrefixError = validateDomainPrefixInput(domainPrefix)

  if (user == null) {
    return <Navigate to="/login" replace />
  }

  const submit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setError('')
    setSaved(false)
    setLoading(true)
    try {
      const normalizedBaseDomain = normalizeBaseDomainInput(baseDomain)
      const normalizedDomainPrefix = normalizeDomainPrefixInput(domainPrefix)
      const nextBaseDomainError = validateBaseDomainInput(normalizedBaseDomain)
      if (nextBaseDomainError != null) {
        throw new Error(nextBaseDomainError)
      }
      const nextDomainPrefixError = validateDomainPrefixInput(normalizedDomainPrefix)
      if (nextDomainPrefixError != null) {
        throw new Error(nextDomainPrefixError)
      }
      await updateDomainSettings(
        normalizedBaseDomain,
        normalizedDomainPrefix,
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
      )
      const normalizedOidcAllowedEmailDomains = oidcAllowedEmailDomainsText
        .split(/\r?\n/)
        .map((value) => value.trim().toLowerCase())
        .filter((value) => value !== '')
      onSetupStatusChange({
        ...setupStatus,
        baseDomain: normalizedBaseDomain,
        domainPrefix: normalizedDomainPrefix,
        internetPublicExposureDisabled: disableInternetPublicExposure,
        oidcEnabled,
        oidcIssuerURL: oidcIssuerURL.trim(),
        oidcClientID: oidcClientID.trim(),
        oidcClientSecretConfigured: setupStatus.oidcClientSecretConfigured || oidcClientSecret !== '',
        oidcAllowedEmailDomains: normalizedOidcAllowedEmailDomains,
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
            <h1 className="mt-2 text-2xl font-semibold text-foreground">Settings</h1>
            <p className="mt-1 text-sm text-muted-foreground">Update domain settings used for newly created machine exposures.</p>
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
                <Label htmlFor="settings-base-domain">
                  Base domain
                </Label>
                <Input
                  id="settings-base-domain"
                  value={baseDomain}
                  onChange={(event) => setBaseDomain(event.target.value)}
                  required
                  className="h-10"
                  placeholder="ryotarai.info"
                />
                {baseDomain !== '' && baseDomainError != null && (
                  <p className="text-sm text-red-300">{baseDomainError}</p>
                )}
              </div>
              <div className="space-y-2">
                <Label htmlFor="settings-domain-prefix">
                  Domain prefix
                </Label>
                <Input
                  id="settings-domain-prefix"
                  value={domainPrefix}
                  onChange={(event) => setDomainPrefix(event.target.value)}
                  className="h-10"
                  placeholder="arca-"
                />
                {domainPrefix !== '' && domainPrefixError != null && (
                  <p className="text-sm text-red-300">{domainPrefixError}</p>
                )}
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
                <Label htmlFor="settings-oidc-enabled">
                  Google/OIDC login
                </Label>
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
              </div>
              <Button
                type="submit"
                className="h-10 w-full"
                disabled={loading || baseDomainError != null || domainPrefixError != null}
              >
                {loading ? 'Saving...' : 'Save settings'}
              </Button>
            </form>
            {saved && <p className="mt-3 text-sm text-emerald-300">Settings updated.</p>}
            {error !== '' && <p className="mt-3 text-sm text-red-300">{error}</p>}
          </CardContent>
        </Card>
      </section>
    </main>
  )
}
