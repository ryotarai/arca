import { useEffect, useState } from 'react'
import { Link, Navigate } from 'react-router-dom'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  createCustomImage,
  deleteCustomImage,
  listCustomImages,
  listMachineProfiles,
  updateCustomImage,
} from '@/lib/api'
import type { CustomImage } from '@/lib/api'
import { messageFromError } from '@/lib/errors'
import { PageHeader } from '@/components/PageHeader'
import type { MachineProfileItem, User } from '@/lib/types'

type CustomImagesPageProps = {
  user: User | null
  onLogout: () => Promise<void>
}

type ImageFormData = {
  name: string
  templateType: string
  description: string
  data: Record<string, string>
  templateIds: string[]
}

const emptyForm: ImageFormData = {
  name: '',
  templateType: 'gce',
  description: '',
  data: {},
  templateIds: [],
}

export function CustomImagesPage({ user }: CustomImagesPageProps) {
  const [images, setImages] = useState<CustomImage[]>([])
  const [profiles, setProfiles] = useState<MachineProfileItem[]>([])
  const [loading, setLoading] = useState(true)
  const [showForm, setShowForm] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [form, setForm] = useState<ImageFormData>(emptyForm)
  const [saving, setSaving] = useState(false)
  const [confirmAction, setConfirmAction] = useState<{ title: string; description: string; confirmLabel: string; variant: 'default' | 'destructive'; onConfirm: () => void } | null>(null)

  const refresh = async () => {
    try {
      const [imgs, rts] = await Promise.all([listCustomImages(), listMachineProfiles()])
      setImages(imgs)
      setProfiles(rts)
    } catch (e) {
      toast.error(messageFromError(e))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    if (user?.role === 'admin') {
      void refresh()
    }
  }, [user])

  if (user == null) return <Navigate to="/login" replace />
  if (user.role !== 'admin') return <Navigate to="/" replace />

  const filteredProfiles = profiles.filter((r) => r.type === form.templateType)

  const handleSave = async () => {
    setSaving(true)
    try {
      if (editingId) {
        await updateCustomImage({
          id: editingId,
          name: form.name,
          templateType: form.templateType,
          data: form.data,
          description: form.description,
          templateIds: form.templateIds,
        })
      } else {
        await createCustomImage({
          name: form.name,
          templateType: form.templateType,
          data: form.data,
          description: form.description,
          templateIds: form.templateIds,
        })
      }
      setShowForm(false)
      setEditingId(null)
      setForm(emptyForm)
      toast.success(editingId ? 'Image updated.' : 'Image created.')
      await refresh()
    } catch (e) {
      toast.error(messageFromError(e))
    } finally {
      setSaving(false)
    }
  }

  const handleDelete = (id: string) => {
    setConfirmAction({
      title: 'Delete custom image',
      description: 'Are you sure you want to delete this custom image?',
      confirmLabel: 'Delete',
      variant: 'destructive',
      onConfirm: () => {
        void (async () => {
          try {
            await deleteCustomImage(id)
            toast.success('Image deleted.')
            await refresh()
          } catch (e) {
            toast.error(messageFromError(e))
          }
        })()
      },
    })
  }

  const handleEdit = (img: CustomImage) => {
    setEditingId(img.id)
    setForm({
      name: img.name,
      templateType: img.templateType,
      description: img.description,
      data: { ...img.data },
      templateIds: [...img.associatedTemplateIds],
    })
    setShowForm(true)
  }

  const handleCreate = () => {
    setEditingId(null)
    setForm(emptyForm)
    setShowForm(true)
  }

  const setDataField = (key: string, value: string) => {
    setForm((prev) => ({ ...prev, data: { ...prev.data, [key]: value } }))
  }

  const toggleProfile = (templateId: string) => {
    setForm((prev) => ({
      ...prev,
      templateIds: prev.templateIds.includes(templateId)
        ? prev.templateIds.filter((id) => id !== templateId)
        : [...prev.templateIds, templateId],
    }))
  }

  return (
    <main className="min-h-dvh px-6 py-10">
      <section className="mx-auto w-full max-w-5xl space-y-6">
        <PageHeader
          label="Admin"
          title="Custom Images"
          description="Manage custom machine images for profiles."
          actions={<Button onClick={handleCreate}>New image</Button>}
        />

        {showForm && (
          <Card className="py-0 shadow-sm">
            <CardHeader className="p-6 pb-3">
              <CardTitle className="text-xl">{editingId ? 'Edit image' : 'New image'}</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4 p-6 pt-3">
              <div className="space-y-2">
                <Label>Name</Label>
                <Input value={form.name} onChange={(e) => setForm((p) => ({ ...p, name: e.target.value }))} placeholder="my-custom-image" />
              </div>

              <div className="space-y-2">
                <Label>Provider type</Label>
                <select
                  value={form.templateType}
                  onChange={(e) => setForm((p) => ({ ...p, templateType: e.target.value, data: {}, templateIds: [] }))}
                  className="h-10 w-full rounded-md border border-input bg-background px-3 text-sm text-foreground"
                >
                  <option value="gce">GCE</option>
                  <option value="lxd">LXD</option>
                  <option value="libvirt">Libvirt</option>
                </select>
              </div>

              <div className="space-y-2">
                <Label>Description</Label>
                <textarea
                  value={form.description}
                  onChange={(e) => setForm((p) => ({ ...p, description: e.target.value }))}
                  className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground"
                  rows={2}
                  placeholder="Optional description"
                />
              </div>

              {form.templateType === 'gce' && (
                <>
                  <div className="space-y-2">
                    <Label>Image project</Label>
                    <Input value={form.data.image_project ?? ''} onChange={(e) => setDataField('image_project', e.target.value)} placeholder="my-gcp-project" />
                  </div>
                  <div className="space-y-2">
                    <Label>Image name</Label>
                    <Input value={form.data.image_name ?? ''} onChange={(e) => setDataField('image_name', e.target.value)} placeholder="arca-user-my-image" />
                  </div>
                  <div className="space-y-2">
                    <Label>Image family</Label>
                    <Input value={form.data.image_family ?? ''} onChange={(e) => setDataField('image_family', e.target.value)} placeholder="my-custom-family" />
                  </div>
                </>
              )}

              {form.templateType === 'lxd' && (
                <>
                  <div className="space-y-2">
                    <Label>Image alias</Label>
                    <Input value={form.data.image_alias ?? ''} onChange={(e) => setDataField('image_alias', e.target.value)} placeholder="my-custom-image" />
                  </div>
                  <div className="space-y-2">
                    <Label>Image fingerprint (alternative)</Label>
                    <Input value={form.data.image_fingerprint ?? ''} onChange={(e) => setDataField('image_fingerprint', e.target.value)} placeholder="abc123..." />
                  </div>
                </>
              )}

              {form.templateType === 'libvirt' && (
                <div className="space-y-2">
                  <Label>Volume name</Label>
                  <Input value={form.data.volume_name ?? ''} onChange={(e) => setDataField('volume_name', e.target.value)} placeholder="/var/lib/libvirt/images/my-image.qcow2" />
                </div>
              )}

              {filteredProfiles.length > 0 && (
                <div className="space-y-2">
                  <Label>Associated profiles</Label>
                  <div className="space-y-1">
                    {filteredProfiles.map((rt) => (
                      <label key={rt.id} className="flex items-center gap-2 text-sm text-foreground">
                        <input
                          type="checkbox"
                          checked={form.templateIds.includes(rt.id)}
                          onChange={() => toggleProfile(rt.id)}
                          className="rounded border-input"
                        />
                        {rt.name}
                      </label>
                    ))}
                  </div>
                </div>
              )}

              <div className="flex gap-2">
                <Button onClick={handleSave} disabled={saving}>
                  {saving ? 'Saving...' : editingId ? 'Update' : 'Create'}
                </Button>
                <Button variant="secondary" onClick={() => { setShowForm(false); setEditingId(null) }}>
                  Cancel
                </Button>
              </div>
            </CardContent>
          </Card>
        )}

        {loading ? (
          <p className="text-sm text-muted-foreground">Loading...</p>
        ) : images.length === 0 ? (
          <p className="text-sm text-muted-foreground">No custom images yet.</p>
        ) : (
          <div className="overflow-x-auto rounded-xl border border-border">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border bg-muted/30">
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">Name</th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">Provider type</th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">Description</th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">Profiles</th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">Source Machine</th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">Created</th>
                  <th className="px-4 py-3 text-right font-medium text-muted-foreground">Actions</th>
                </tr>
              </thead>
              <tbody>
                {images.map((img) => (
                  <tr key={img.id} className="border-b border-border last:border-0">
                    <td className="px-4 py-3 font-medium text-foreground">{img.name}</td>
                    <td className="px-4 py-3 text-muted-foreground uppercase">{img.templateType}</td>
                    <td className="px-4 py-3 text-muted-foreground">{img.description || '-'}</td>
                    <td className="px-4 py-3 text-muted-foreground">
                      {img.associatedTemplateIds.length > 0
                        ? img.associatedTemplateIds.map((rid) => profiles.find((r) => r.id === rid)?.name ?? rid).join(', ')
                        : '-'}
                    </td>
                    <td className="px-4 py-3 text-muted-foreground">
                      {img.sourceMachineId ? (
                        <Link to={`/machines/${img.sourceMachineId}`} className="text-muted-foreground underline underline-offset-2 hover:text-foreground transition-colors">
                          {img.sourceMachineId}
                        </Link>
                      ) : '-'}
                    </td>
                    <td className="px-4 py-3 text-muted-foreground">{img.createdAt ? new Date(img.createdAt).toLocaleDateString() : '-'}</td>
                    <td className="px-4 py-3 text-right">
                      <div className="flex justify-end gap-2">
                        <Button variant="secondary" size="sm" onClick={() => handleEdit(img)}>
                          Edit
                        </Button>
                        <Button variant="destructive" size="sm" onClick={() => handleDelete(img.id)}>
                          Delete
                        </Button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>

      <ConfirmDialog
        open={confirmAction != null}
        onOpenChange={(open) => { if (!open) setConfirmAction(null) }}
        title={confirmAction?.title ?? ''}
        description={confirmAction?.description ?? ''}
        confirmLabel={confirmAction?.confirmLabel ?? 'Confirm'}
        variant={confirmAction?.variant ?? 'default'}
        onConfirm={() => {
          confirmAction?.onConfirm()
          setConfirmAction(null)
        }}
      />
    </main>
  )
}
