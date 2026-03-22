import type { ReactNode } from 'react'

type PageHeaderProps = {
  label?: string
  title: string
  description?: string
  actions?: ReactNode
}

export function PageHeader({ label = 'Arca', title, description, actions }: PageHeaderProps) {
  return (
    <header className="flex flex-col items-start justify-between gap-4 rounded-xl border border-border bg-muted/30 p-6 md:flex-row md:items-center">
      <div>
        <p className="text-xs font-medium uppercase tracking-[0.24em] text-muted-foreground">{label}</p>
        <h1 className="mt-2 text-2xl font-semibold text-foreground">{title}</h1>
        {description != null && (
          <p className="mt-1 text-sm text-muted-foreground">{description}</p>
        )}
      </div>
      {actions != null && (
        <div className="flex items-center gap-3">
          {actions}
        </div>
      )}
    </header>
  )
}
