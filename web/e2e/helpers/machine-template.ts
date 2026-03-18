import type { APIResponse, Page } from '@playwright/test'

async function parseJSONSafe(response: APIResponse): Promise<Record<string, unknown> | null> {
  try {
    return (await response.json()) as Record<string, unknown>
  } catch {
    return null
  }
}

type TemplateRecord = {
  id: string
  name: string
  type: string
}

type CreateTemplateOptions = {
  name: string
  type: string
  config: Record<string, unknown>
}

export async function createMachineTemplateViaAPI(
  page: Page,
  options: CreateTemplateOptions,
): Promise<TemplateRecord> {
  const typeMap: Record<string, number> = {
    libvirt: 1,
    gce: 2,
    lxd: 3,
  }

  const response = await page.request.post('/arca.v1.MachineTemplateService/CreateMachineTemplate', {
    data: {
      name: options.name,
      type: typeMap[options.type] ?? 0,
      config: options.config,
    },
  })

  if (!response.ok()) {
    const payload = await parseJSONSafe(response)
    throw new Error(`CreateMachineTemplate failed: ${response.status()} ${JSON.stringify(payload)}`)
  }

  const payload = (await response.json()) as {
    template?: { id?: string; name?: string; type?: string }
  }
  return {
    id: payload.template?.id?.trim() ?? '',
    name: payload.template?.name?.trim() ?? '',
    type: payload.template?.type?.trim() ?? '',
  }
}

export async function deleteMachineTemplateViaAPI(page: Page, templateId: string) {
  const response = await page.request.post('/arca.v1.MachineTemplateService/DeleteMachineTemplate', {
    data: { templateId },
    failOnStatusCode: false,
  })

  if (!response.ok()) {
    const payload = await parseJSONSafe(response)
    const code = String(payload?.code ?? '').toLowerCase()
    if (code.includes('not_found')) {
      return
    }
    throw new Error(`DeleteMachineTemplate failed: ${response.status()} ${JSON.stringify(payload)}`)
  }
}

export async function ensureLxdTemplate(
  page: Page,
): Promise<TemplateRecord> {
  const serverPort = new URL(
    process.env.PLAYWRIGHT_BASE_URL ?? 'http://127.0.0.1:18080',
  ).port
  try {
    return await createMachineTemplateViaAPI(page, {
      name: 'lxd-e2e',
      type: 'lxd',
      config: {
        lxd: { endpoint: 'https://localhost:8443' },
        exposure: {
          method: 2, // PROXY_VIA_SERVER
          domainPrefix: 'arca-',
          baseDomain: 'localhost',
          connectivity: 1, // PRIVATE_IP
        },
        serverApiUrl: `http://10.200.0.1:${serverPort}`,
      },
    })
  } catch (error) {
    if (String(error).includes('already_exists')) {
      const listResp = await page.request.post('/arca.v1.MachineTemplateService/ListMachineTemplates', {
        data: {},
      })
      const listPayload = (await listResp.json()) as {
        templates?: Array<{ id?: string; name?: string; type?: string }>
      }
      const existing = listPayload.templates?.find((r) => r.name === 'lxd-e2e')
      if (existing) {
        return {
          id: existing.id ?? '',
          name: existing.name ?? '',
          type: existing.type ?? '',
        }
      }
    }
    throw error
  }
}

export async function ensureLxdTemplateWithProxyExposure(
  page: Page,
  opts?: { name?: string; domainPrefix?: string; baseDomain?: string },
): Promise<TemplateRecord> {
  const name = opts?.name ?? 'lxd-proxy-e2e'
  const domainPrefix = opts?.domainPrefix ?? 'arca-'
  const baseDomain = opts?.baseDomain ?? 'localhost'
  const serverPort = new URL(
    process.env.PLAYWRIGHT_BASE_URL ?? 'http://127.0.0.1:18080',
  ).port

  try {
    return await createMachineTemplateViaAPI(page, {
      name,
      type: 'lxd',
      config: {
        lxd: { endpoint: 'https://localhost:8443' },
        exposure: {
          method: 2, // PROXY_VIA_SERVER
          domainPrefix,
          baseDomain,
          connectivity: 1, // PRIVATE_IP
        },
        serverApiUrl: `http://10.200.0.1:${serverPort}`,
      },
    })
  } catch (error) {
    if (String(error).includes('already_exists')) {
      const listResp = await page.request.post('/arca.v1.MachineTemplateService/ListMachineTemplates', {
        data: {},
      })
      const listPayload = (await listResp.json()) as {
        templates?: Array<{ id?: string; name?: string; type?: string }>
      }
      const existing = listPayload.templates?.find((r) => r.name === name)
      if (existing) {
        return {
          id: existing.id ?? '',
          name: existing.name ?? '',
          type: existing.type ?? '',
        }
      }
    }
    throw error
  }
}
