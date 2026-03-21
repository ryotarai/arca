import { useEffect, useState } from 'react'
import { Link, Navigate } from 'react-router-dom'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import {
  getUserSettings,
  updateUserSettings,
  getUserNotificationSettings,
  updateUserNotificationSettings,
  listUserLLMModels,
  createUserLLMModel,
  updateUserLLMModel,
  deleteUserLLMModel,
} from '@/lib/api'
import type { LLMModel } from '@/lib/api'
import { messageFromError } from '@/lib/errors'
import type { User } from '@/lib/types'

type SettingsPageProps = {
  user: User | null
  onLogout: () => Promise<void>
}

export function SettingsPage({ user, onLogout }: SettingsPageProps) {
  const [sshPublicKeysText, setSshPublicKeysText] = useState('')
  const [sshLoading, setSshLoading] = useState(false)
  const [sshSaving, setSshSaving] = useState(false)
  const [sshError, setSshError] = useState('')
  const [sshSaved, setSshSaved] = useState(false)

  useEffect(() => {
    if (user == null) {
      return
    }
    let cancelled = false
    const load = async () => {
      setSshLoading(true)
      setSshError('')
      try {
        const settings = await getUserSettings()
        if (cancelled) {
          return
        }
        setSshPublicKeysText(settings.sshPublicKeys.join('\n'))
      } catch (e) {
        if (!cancelled) {
          setSshError(messageFromError(e))
        }
      } finally {
        if (!cancelled) {
          setSshLoading(false)
        }
      }
    }
    void load()
    return () => {
      cancelled = true
    }
  }, [user?.id])

  if (user == null) {
    return <Navigate to="/login" replace />
  }

  const submitSSHSettings = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setSshError('')
    setSshSaved(false)
    setSshSaving(true)
    try {
      const keys = sshPublicKeysText
        .split(/\r?\n/)
        .map((value) => value.trim())
        .filter((value) => value !== '')
      const settings = await updateUserSettings(keys)
      setSshPublicKeysText(settings.sshPublicKeys.join('\n'))
      setSshSaved(true)
    } catch (e) {
      setSshError(messageFromError(e))
    } finally {
      setSshSaving(false)
    }
  }

  return (
    <main className="min-h-dvh px-6 py-10">
      <section className="mx-auto w-full max-w-3xl space-y-6">
        <header className="flex flex-col items-start justify-between gap-4 rounded-xl border border-border bg-muted/30 p-6 md:flex-row md:items-center">
          <div>
            <p className="text-xs font-medium uppercase tracking-[0.24em] text-muted-foreground">Arca</p>
            <h1 className="mt-2 text-2xl font-semibold text-foreground">User settings</h1>
            <p className="mt-1 text-sm text-muted-foreground">Manage SSH public keys for your interactive machine user.</p>
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
            <CardTitle className="text-xl">SSH public keys</CardTitle>
            <CardDescription>
              Configure keys added to your machine interactive user&apos;s <code>~/.ssh/authorized_keys</code>.
            </CardDescription>
          </CardHeader>
          <CardContent className="p-6 pt-3">
            <form className="space-y-4" onSubmit={submitSSHSettings}>
              <div className="space-y-2">
                <Label htmlFor="settings-ssh-public-keys">
                  Public keys (one per line)
                </Label>
                <textarea
                  id="settings-ssh-public-keys"
                  value={sshPublicKeysText}
                  onChange={(event) => setSshPublicKeysText(event.target.value)}
                  rows={8}
                  className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  placeholder={'ssh-ed25519 AAAA... you@example.com'}
                  disabled={sshLoading || sshSaving}
                />
              </div>
              <Button type="submit" className="h-10 w-full" disabled={sshLoading || sshSaving}>
                {sshSaving ? 'Saving...' : 'Save SSH keys'}
              </Button>
            </form>
            {sshSaved && <p className="mt-3 text-sm text-emerald-300">SSH keys updated.</p>}
            {sshError !== '' && <p className="mt-3 text-sm text-red-300">{sshError}</p>}
          </CardContent>
        </Card>

        <LLMModelsCard userId={user.id} />

        <NotificationSettingsCard userId={user.id} />
      </section>
    </main>
  )
}

const ENDPOINT_TYPES = [
  { value: 'openai_chat', label: 'OpenAI Chat API' },
  { value: 'openai_response', label: 'OpenAI Response API' },
  { value: 'anthropic', label: 'Anthropic' },
  { value: 'google_gemini', label: 'Google Gemini' },
] as const

type LLMFormData = {
  configName: string
  endpointType: string
  customEndpoint: string
  modelName: string
  apiKey: string
  maxContextTokens: string
}

const emptyForm: LLMFormData = {
  configName: '',
  endpointType: 'openai_chat',
  customEndpoint: '',
  modelName: '',
  apiKey: '',
  maxContextTokens: '0',
}

function endpointTypeLabel(value: string): string {
  return ENDPOINT_TYPES.find((t) => t.value === value)?.label ?? value
}

function LLMModelsCard({ userId }: { userId: string }) {
  const [models, setModels] = useState<LLMModel[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [form, setForm] = useState<LLMFormData>(emptyForm)
  const [saving, setSaving] = useState(false)
  const [formError, setFormError] = useState('')
  const [deleteConfirmId, setDeleteConfirmId] = useState<string | null>(null)

  const loadModels = async () => {
    try {
      const result = await listUserLLMModels()
      setModels(result)
      setError('')
    } catch (e) {
      setError(messageFromError(e))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    let cancelled = false
    void (async () => {
      try {
        const result = await listUserLLMModels()
        if (!cancelled) {
          setModels(result)
        }
      } catch (e) {
        if (!cancelled) setError(messageFromError(e))
      } finally {
        if (!cancelled) setLoading(false)
      }
    })()
    return () => { cancelled = true }
  }, [userId])

  const openCreate = () => {
    setEditingId(null)
    setForm(emptyForm)
    setFormError('')
    setDialogOpen(true)
  }

  const openEdit = (model: LLMModel) => {
    setEditingId(model.id)
    setForm({
      configName: model.configName,
      endpointType: model.endpointType,
      customEndpoint: model.customEndpoint,
      modelName: model.modelName,
      apiKey: '',
      maxContextTokens: String(model.maxContextTokens),
    })
    setFormError('')
    setDialogOpen(true)
  }

  const submitForm = async () => {
    setFormError('')
    setSaving(true)
    try {
      const params = {
        configName: form.configName.trim(),
        endpointType: form.endpointType,
        customEndpoint: form.customEndpoint.trim(),
        modelName: form.modelName.trim(),
        apiKey: form.apiKey,
        maxContextTokens: parseInt(form.maxContextTokens, 10) || 0,
      }

      if (editingId != null) {
        await updateUserLLMModel({ id: editingId, ...params })
      } else {
        await createUserLLMModel(params)
      }
      setDialogOpen(false)
      await loadModels()
    } catch (e) {
      setFormError(messageFromError(e))
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = async (id: string) => {
    try {
      await deleteUserLLMModel(id)
      setDeleteConfirmId(null)
      await loadModels()
    } catch (e) {
      setError(messageFromError(e))
      setDeleteConfirmId(null)
    }
  }

  return (
    <>
      <Card className="py-0 shadow-sm">
        <CardHeader className="space-y-2 p-6 pb-3">
          <div className="flex items-center justify-between">
            <div className="space-y-1">
              <CardTitle className="text-xl">LLM Models</CardTitle>
              <CardDescription>
                Configure LLM model endpoints for your machines.
              </CardDescription>
            </div>
            <Button type="button" size="sm" onClick={openCreate}>Add model</Button>
          </div>
        </CardHeader>
        <CardContent className="p-6 pt-3">
          {loading ? (
            <p className="text-sm text-muted-foreground">Loading...</p>
          ) : models.length === 0 ? (
            <p className="text-sm text-muted-foreground">No LLM models configured yet.</p>
          ) : (
            <div className="space-y-3">
              {models.map((model) => (
                <div
                  key={model.id}
                  className="flex items-center justify-between rounded-md border border-border bg-muted/30 px-4 py-3"
                >
                  <div className="min-w-0 flex-1">
                    <p className="text-sm font-medium text-foreground">{model.configName}</p>
                    <p className="mt-0.5 text-xs text-muted-foreground">
                      {endpointTypeLabel(model.endpointType)} &middot; {model.modelName}
                      {model.maxContextTokens > 0 && ` \u00b7 ${model.maxContextTokens.toLocaleString()} tokens`}
                      {model.customEndpoint && ` \u00b7 ${model.customEndpoint}`}
                    </p>
                  </div>
                  <div className="ml-3 flex items-center gap-2">
                    <Button type="button" variant="ghost" size="sm" onClick={() => openEdit(model)}>
                      Edit
                    </Button>
                    <Button
                      type="button"
                      variant="ghost"
                      size="sm"
                      className="text-red-400 hover:text-red-300"
                      onClick={() => setDeleteConfirmId(model.id)}
                    >
                      Delete
                    </Button>
                  </div>
                </div>
              ))}
            </div>
          )}
          {error !== '' && <p className="mt-3 text-sm text-red-300">{error}</p>}
        </CardContent>
      </Card>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{editingId != null ? 'Edit LLM Model' : 'Add LLM Model'}</DialogTitle>
            <DialogDescription>
              {editingId != null
                ? 'Update the LLM model configuration. Leave API key empty to keep the existing key.'
                : 'Configure a new LLM model endpoint.'}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="llm-config-name">Config name</Label>
              <Input
                id="llm-config-name"
                value={form.configName}
                onChange={(e) => setForm({ ...form, configName: e.target.value })}
                placeholder="my-gpt4"
                disabled={saving}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="llm-endpoint-type">Endpoint type</Label>
              <Select
                value={form.endpointType}
                onValueChange={(value) => setForm({ ...form, endpointType: value })}
                disabled={saving}
              >
                <SelectTrigger id="llm-endpoint-type" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {ENDPOINT_TYPES.map((t) => (
                    <SelectItem key={t.value} value={t.value}>{t.label}</SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
            <div className="space-y-2">
              <Label htmlFor="llm-custom-endpoint">Custom endpoint (optional)</Label>
              <Input
                id="llm-custom-endpoint"
                value={form.customEndpoint}
                onChange={(e) => setForm({ ...form, customEndpoint: e.target.value })}
                placeholder="https://api.example.com/v1"
                disabled={saving}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="llm-model-name">Model name</Label>
              <Input
                id="llm-model-name"
                value={form.modelName}
                onChange={(e) => setForm({ ...form, modelName: e.target.value })}
                placeholder="gpt-4o"
                disabled={saving}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="llm-api-key">API key{editingId != null ? ' (leave empty to keep existing)' : ''}</Label>
              <Input
                id="llm-api-key"
                type="password"
                value={form.apiKey}
                onChange={(e) => setForm({ ...form, apiKey: e.target.value })}
                placeholder={editingId != null ? '(unchanged)' : 'sk-...'}
                disabled={saving}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="llm-max-context-tokens">Max context tokens</Label>
              <Input
                id="llm-max-context-tokens"
                type="number"
                value={form.maxContextTokens}
                onChange={(e) => setForm({ ...form, maxContextTokens: e.target.value })}
                placeholder="0"
                min={0}
                disabled={saving}
              />
            </div>
            {formError !== '' && <p className="text-sm text-red-300">{formError}</p>}
          </div>
          <DialogFooter>
            <Button type="button" variant="secondary" onClick={() => setDialogOpen(false)} disabled={saving}>
              Cancel
            </Button>
            <Button type="button" onClick={submitForm} disabled={saving}>
              {saving ? 'Saving...' : editingId != null ? 'Update' : 'Create'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={deleteConfirmId != null} onOpenChange={(open) => { if (!open) setDeleteConfirmId(null) }}>
        <DialogContent className="sm:max-w-sm">
          <DialogHeader>
            <DialogTitle>Delete LLM Model</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete this LLM model configuration? This action cannot be undone.
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button type="button" variant="secondary" onClick={() => setDeleteConfirmId(null)}>Cancel</Button>
            <Button
              type="button"
              variant="destructive"
              onClick={() => { if (deleteConfirmId != null) void handleDelete(deleteConfirmId) }}
            >
              Delete
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

function NotificationSettingsCard({ userId }: { userId: string }) {
  const [slackEnabled, setSlackEnabled] = useState(true)
  const [slackUserId, setSlackUserId] = useState('')
  const [slackAdminEnabled, setSlackAdminEnabled] = useState(false)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')
  const [saved, setSaved] = useState(false)

  useEffect(() => {
    let cancelled = false
    const load = async () => {
      try {
        const settings = await getUserNotificationSettings()
        if (cancelled) return
        setSlackEnabled(settings.slackEnabled)
        setSlackUserId(settings.slackUserId)
        setSlackAdminEnabled(settings.slackAdminEnabled)
      } catch {
        // Notification settings may not be available; use defaults.
      } finally {
        if (!cancelled) setLoading(false)
      }
    }
    void load()
    return () => { cancelled = true }
  }, [userId])

  if (!loading && !slackAdminEnabled) {
    return null
  }

  const submit = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setError('')
    setSaved(false)
    setSaving(true)
    try {
      const result = await updateUserNotificationSettings({
        slackEnabled,
        slackUserId: slackUserId.trim(),
      })
      setSlackEnabled(result.slackEnabled)
      setSlackUserId(result.slackUserId)
      setSaved(true)
    } catch (e) {
      setError(messageFromError(e))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Card className="py-0 shadow-sm">
      <CardHeader className="space-y-2 p-6 pb-3">
        <CardTitle className="text-xl">Slack notifications</CardTitle>
        <CardDescription>
          Receive Slack DMs when your machines change state (ready, auto-stop, failures).
        </CardDescription>
      </CardHeader>
      <CardContent className="p-6 pt-3">
        {loading ? (
          <p className="text-sm text-muted-foreground">Loading...</p>
        ) : (
          <form className="space-y-4" onSubmit={submit}>
            <label className="flex items-center gap-2 rounded-md border border-border bg-muted/30 px-3 py-2 text-sm text-foreground">
              <input
                type="checkbox"
                checked={slackEnabled}
                onChange={(e) => setSlackEnabled(e.target.checked)}
              />
              Enable Slack notifications
            </label>

            <div className="space-y-2">
              <Label htmlFor="settings-slack-user-id">Your Slack member ID</Label>
              <Input
                id="settings-slack-user-id"
                value={slackUserId}
                onChange={(e) => setSlackUserId(e.target.value)}
                className="h-10"
                placeholder="U0123456789"
                disabled={saving}
              />
              <p className="text-xs text-muted-foreground">
                Find your member ID in Slack: click your profile picture, then "Profile", then the three-dot menu and "Copy member ID".
              </p>
            </div>

            <Button type="submit" className="h-10 w-full" disabled={saving}>
              {saving ? 'Saving...' : 'Save notification settings'}
            </Button>
          </form>
        )}
        {saved && <p className="mt-3 text-sm text-emerald-300">Notification settings updated.</p>}
        {error !== '' && <p className="mt-3 text-sm text-red-300">{error}</p>}
      </CardContent>
    </Card>
  )
}
