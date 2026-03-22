import { useEffect, useState } from 'react'
import { Navigate } from 'react-router-dom'
import { Cpu } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import {
  listServerLLMModels,
  createServerLLMModel,
  updateServerLLMModel,
  deleteServerLLMModel,
} from '@/lib/api'
import type { ServerLLMModel } from '@/lib/api'
import { messageFromError } from '@/lib/errors'
import { EmptyState } from '@/components/EmptyState'
import type { User } from '@/lib/types'

type ServerLLMModelsPageProps = {
  user: User | null
  onLogout: () => Promise<void>
}

const ENDPOINT_TYPES = [
  { value: 'openai_chat', label: 'OpenAI Chat API' },
  { value: 'openai_response', label: 'OpenAI Response API' },
  { value: 'anthropic', label: 'Anthropic' },
  { value: 'google_gemini', label: 'Google Gemini' },
] as const

type FormData = {
  configName: string
  endpointType: string
  customEndpoint: string
  modelName: string
  tokenCommand: string
  maxContextTokens: string
}

const emptyForm: FormData = {
  configName: '',
  endpointType: 'openai_chat',
  customEndpoint: '',
  modelName: '',
  tokenCommand: '',
  maxContextTokens: '0',
}

function endpointTypeLabel(value: string): string {
  return ENDPOINT_TYPES.find((t) => t.value === value)?.label ?? value
}

export function ServerLLMModelsPage({ user }: ServerLLMModelsPageProps) {
  const [models, setModels] = useState<ServerLLMModel[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [form, setForm] = useState<FormData>(emptyForm)
  const [saving, setSaving] = useState(false)
  const [formError, setFormError] = useState('')
  const [deleteConfirmId, setDeleteConfirmId] = useState<string | null>(null)

  if (user == null || user.role !== 'admin') {
    return <Navigate to="/settings" replace />
  }

  const loadModels = async () => {
    try {
      const result = await listServerLLMModels()
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
        const result = await listServerLLMModels()
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
  }, [])

  const openCreate = () => {
    setEditingId(null)
    setForm(emptyForm)
    setFormError('')
    setDialogOpen(true)
  }

  const openEdit = (model: ServerLLMModel) => {
    setEditingId(model.id)
    setForm({
      configName: model.configName,
      endpointType: model.endpointType,
      customEndpoint: model.customEndpoint,
      modelName: model.modelName,
      tokenCommand: model.tokenCommand,
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
        tokenCommand: form.tokenCommand.trim(),
        maxContextTokens: parseInt(form.maxContextTokens, 10) || 0,
      }

      if (editingId != null) {
        await updateServerLLMModel({ id: editingId, ...params })
      } else {
        await createServerLLMModel(params)
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
      await deleteServerLLMModel(id)
      setDeleteConfirmId(null)
      await loadModels()
    } catch (e) {
      setError(messageFromError(e))
      setDeleteConfirmId(null)
    }
  }

  return (
    <>
      <main className="mx-auto max-w-3xl px-6 py-10">
        <div className="mb-8">
          <p className="text-xs font-medium uppercase tracking-[0.24em] text-muted-foreground">Arca</p>
          <h1 className="mt-2 text-3xl font-bold tracking-tight text-foreground">Server LLM Models</h1>
          <p className="mt-1 text-sm text-muted-foreground">
            Manage server-wide LLM model configurations. These models are available to all users via token commands.
          </p>
        </div>

        <Card className="py-0 shadow-sm">
          <CardHeader className="space-y-2 p-6 pb-3">
            <div className="flex items-center justify-between">
              <div className="space-y-1">
                <CardTitle className="text-xl">LLM Models</CardTitle>
                <CardDescription>
                  Server-wide LLM models with token commands for dynamic credential retrieval.
                </CardDescription>
              </div>
              <Button type="button" size="sm" onClick={openCreate}>Add model</Button>
            </div>
          </CardHeader>
          <CardContent className="p-6 pt-3">
            {loading ? (
              <p className="text-sm text-muted-foreground">Loading...</p>
            ) : models.length === 0 ? (
              <EmptyState
                icon={<Cpu className="size-6" />}
                title="No server LLM models configured yet"
                description="Server-wide LLM models use token commands for dynamic credential retrieval."
              />
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
                      <p className="mt-0.5 text-xs text-muted-foreground font-mono">
                        cmd: {model.tokenCommand}
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
      </main>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="sm:max-w-md">
          <DialogHeader>
            <DialogTitle>{editingId != null ? 'Edit Server LLM Model' : 'Add Server LLM Model'}</DialogTitle>
            <DialogDescription>
              {editingId != null
                ? 'Update the server-wide LLM model configuration.'
                : 'Configure a new server-wide LLM model with a token command.'}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="slm-config-name">Config name</Label>
              <Input
                id="slm-config-name"
                value={form.configName}
                onChange={(e) => setForm({ ...form, configName: e.target.value })}
                placeholder="my-gpt4"
                disabled={saving}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="slm-endpoint-type">Endpoint type</Label>
              <Select
                value={form.endpointType}
                onValueChange={(value) => setForm({ ...form, endpointType: value })}
                disabled={saving}
              >
                <SelectTrigger id="slm-endpoint-type" className="w-full">
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
              <Label htmlFor="slm-custom-endpoint">Custom endpoint (optional)</Label>
              <Input
                id="slm-custom-endpoint"
                value={form.customEndpoint}
                onChange={(e) => setForm({ ...form, customEndpoint: e.target.value })}
                placeholder="https://api.example.com/v1"
                disabled={saving}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="slm-model-name">Model name</Label>
              <Input
                id="slm-model-name"
                value={form.modelName}
                onChange={(e) => setForm({ ...form, modelName: e.target.value })}
                placeholder="gpt-4o"
                disabled={saving}
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="slm-token-command">Token command</Label>
              <Input
                id="slm-token-command"
                value={form.tokenCommand}
                onChange={(e) => setForm({ ...form, tokenCommand: e.target.value })}
                placeholder="/usr/local/bin/get-llm-token"
                disabled={saving}
              />
              <p className="text-xs text-muted-foreground">
                Command executed on the server to obtain an API token. Receives user info JSON on stdin, must output {'{"token": "...", "expire_at": <unix_ts>}'} on stdout.
              </p>
            </div>
            <div className="space-y-2">
              <Label htmlFor="slm-max-context-tokens">Max context tokens</Label>
              <Input
                id="slm-max-context-tokens"
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
            <DialogTitle>Delete Server LLM Model</DialogTitle>
            <DialogDescription>
              Are you sure you want to delete this server LLM model? This action cannot be undone.
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
