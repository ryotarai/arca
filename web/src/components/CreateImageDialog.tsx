import { useState, useEffect } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { createImageFromMachine } from '@/lib/api'
import { messageFromError } from '@/lib/errors'

const IMAGE_NAME_RE = /^[a-z]([-a-z0-9]*[a-z0-9])?$/

function sanitizeName(raw: string): string {
  return raw
    .toLowerCase()
    .replace(/[^a-z0-9-]/g, '-')
    .replace(/-+/g, '-')
    .replace(/^-/, '')
    .replace(/-$/, '')
}

function defaultImageName(machineName: string): string {
  const now = new Date()
  const ts = [
    now.getFullYear(),
    String(now.getMonth() + 1).padStart(2, '0'),
    String(now.getDate()).padStart(2, '0'),
    '-',
    String(now.getHours()).padStart(2, '0'),
    String(now.getMinutes()).padStart(2, '0'),
    String(now.getSeconds()).padStart(2, '0'),
  ].join('')
  return sanitizeName(`${machineName}-image-${ts}`)
}

type CreateImageDialogProps = {
  machineId: string
  machineName: string
  open: boolean
  onOpenChange: (open: boolean) => void
  onSuccess: () => void
}

export function CreateImageDialog({
  machineId,
  machineName,
  open,
  onOpenChange,
  onSuccess,
}: CreateImageDialogProps) {
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    if (open) {
      setName(defaultImageName(machineName))
      setDescription('')
      setError('')
      setSubmitting(false)
    }
  }, [open, machineName])

  const nameValid = IMAGE_NAME_RE.test(name)

  const handleSubmit = async () => {
    if (!nameValid) return
    setSubmitting(true)
    setError('')
    try {
      await createImageFromMachine(machineId, name, description)
      onOpenChange(false)
      onSuccess()
    } catch (e) {
      setError(messageFromError(e))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>Create image from machine</DialogTitle>
          <DialogDescription>
            Create a custom image from the current state of this machine.
          </DialogDescription>
          <div className="rounded-md border border-yellow-500/30 bg-yellow-500/10 px-3 py-2 text-sm text-yellow-200">
            The machine will be stopped before the image is created and restarted afterward.
          </div>
        </DialogHeader>
        <div className="space-y-4 py-2">
          <div className="space-y-2">
            <Label htmlFor="image-name">Image name</Label>
            <Input
              id="image-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              disabled={submitting}
              placeholder="my-custom-image"
            />
            {name !== '' && !nameValid && (
              <p className="text-xs text-red-400">
                Must start with a letter, contain only lowercase letters, digits, and hyphens, and end with a letter or digit.
              </p>
            )}
          </div>
          <div className="space-y-2">
            <Label htmlFor="image-description">Description (optional)</Label>
            <textarea
              id="image-description"
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              disabled={submitting}
              rows={3}
              className="flex w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:cursor-not-allowed disabled:opacity-50"
              placeholder="Describe what this image contains..."
            />
          </div>
          {error !== '' && (
            <p className="rounded-md border border-red-400/30 bg-red-500/12 px-3 py-2 text-sm text-red-200">
              {error}
            </p>
          )}
        </div>
        <DialogFooter>
          <Button
            type="button"
            variant="secondary"
            onClick={() => onOpenChange(false)}
            disabled={submitting}
          >
            Cancel
          </Button>
          <Button
            type="button"
            onClick={() => void handleSubmit()}
            disabled={submitting || !nameValid}
          >
            {submitting ? 'Creating...' : 'Create image'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
