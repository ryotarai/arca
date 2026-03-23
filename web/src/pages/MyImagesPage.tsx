import { useEffect, useState } from 'react'
import { Link, Navigate } from 'react-router-dom'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { ConfirmDialog } from '@/components/ConfirmDialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  deleteCustomImage,
  listCustomImages,
  updateCustomImage,
} from '@/lib/api'
import type { CustomImage } from '@/lib/api'
import { messageFromError } from '@/lib/errors'
import { PageHeader } from '@/components/PageHeader'
import type { User } from '@/lib/types'

type MyImagesPageProps = {
  user: User | null
  onLogout: () => Promise<void>
}

type EditFormData = {
  name: string
  description: string
}

export function MyImagesPage({ user }: MyImagesPageProps) {
  const [images, setImages] = useState<CustomImage[]>([])
  const [loading, setLoading] = useState(true)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [form, setForm] = useState<EditFormData>({ name: '', description: '' })
  const [saving, setSaving] = useState(false)
  const [confirmAction, setConfirmAction] = useState<{
    title: string
    description: string
    confirmLabel: string
    variant: 'default' | 'destructive'
    onConfirm: () => void
  } | null>(null)

  const refresh = async () => {
    try {
      const imgs = await listCustomImages()
      setImages(imgs)
    } catch (e) {
      toast.error(messageFromError(e))
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => {
    if (user != null) {
      void refresh()
    }
  }, [user])

  if (user == null) return <Navigate to="/login" replace />

  const isOwn = (img: CustomImage) => img.createdByUserId === user.id

  const handleEdit = (img: CustomImage) => {
    setEditingId(img.id)
    setForm({ name: img.name, description: img.description })
  }

  const handleSave = async () => {
    if (!editingId) return
    setSaving(true)
    try {
      const img = images.find((i) => i.id === editingId)
      if (!img) return
      await updateCustomImage({
        id: editingId,
        name: form.name,
        templateType: img.templateType,
        data: { ...img.data },
        description: form.description,
        templateIds: [...img.associatedTemplateIds],
      })
      setEditingId(null)
      setForm({ name: '', description: '' })
      toast.success('Image updated.')
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

  return (
    <main className="min-h-dvh px-6 py-10">
      <section className="mx-auto w-full max-w-5xl space-y-6">
        <PageHeader
          label="Images"
          title="My Images"
          description="Your custom images and shared images from other users."
        />

        {editingId && (
          <Card className="py-0 shadow-sm">
            <CardHeader className="p-6 pb-3">
              <CardTitle className="text-xl">Edit image</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4 p-6 pt-3">
              <div className="space-y-2">
                <Label>Name</Label>
                <Input
                  value={form.name}
                  onChange={(e) => setForm((p) => ({ ...p, name: e.target.value }))}
                  placeholder="my-custom-image"
                />
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

              <div className="flex gap-2">
                <Button onClick={handleSave} disabled={saving}>
                  {saving ? 'Saving...' : 'Update'}
                </Button>
                <Button variant="secondary" onClick={() => setEditingId(null)}>
                  Cancel
                </Button>
              </div>
            </CardContent>
          </Card>
        )}

        {loading ? (
          <p className="text-sm text-muted-foreground">Loading...</p>
        ) : images.length === 0 ? (
          <p className="text-sm text-muted-foreground">No images available.</p>
        ) : (
          <div className="overflow-x-auto rounded-xl border border-border">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-border bg-muted/30">
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">Name</th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">Provider type</th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">Description</th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">Source Machine</th>
                  <th className="px-4 py-3 text-left font-medium text-muted-foreground">Visibility</th>
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
                      {img.sourceMachineId ? (
                        <Link
                          to={`/machines/${img.sourceMachineId}`}
                          className="text-muted-foreground underline underline-offset-2 hover:text-foreground transition-colors"
                        >
                          {img.sourceMachineId}
                        </Link>
                      ) : (
                        '-'
                      )}
                    </td>
                    <td className="px-4 py-3">
                      {img.visibility === 'shared' ? (
                        <span className="inline-flex items-center rounded-full bg-blue-500/10 px-2 py-0.5 text-xs font-medium text-blue-400 ring-1 ring-inset ring-blue-500/20">
                          Shared
                        </span>
                      ) : (
                        <span className="inline-flex items-center rounded-full bg-zinc-500/10 px-2 py-0.5 text-xs font-medium text-zinc-400 ring-1 ring-inset ring-zinc-500/20">
                          Private
                        </span>
                      )}
                    </td>
                    <td className="px-4 py-3 text-muted-foreground">
                      {img.createdAt ? new Date(img.createdAt).toLocaleDateString() : '-'}
                    </td>
                    <td className="px-4 py-3 text-right">
                      {isOwn(img) && (
                        <div className="flex justify-end gap-2">
                          <Button variant="secondary" size="sm" onClick={() => handleEdit(img)}>
                            Edit
                          </Button>
                          <Button variant="destructive" size="sm" onClick={() => handleDelete(img.id)}>
                            Delete
                          </Button>
                        </div>
                      )}
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
        onOpenChange={(open) => {
          if (!open) setConfirmAction(null)
        }}
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
