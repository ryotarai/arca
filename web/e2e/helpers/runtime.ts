import type { APIResponse, Page } from '@playwright/test'

async function parseJSONSafe(response: APIResponse): Promise<Record<string, unknown> | null> {
  try {
    return (await response.json()) as Record<string, unknown>
  } catch {
    return null
  }
}

type RuntimeRecord = {
  id: string
  name: string
  type: string
}

type CreateRuntimeOptions = {
  name: string
  type: string
  config: Record<string, unknown>
}

export async function createRuntimeViaAPI(
  page: Page,
  options: CreateRuntimeOptions,
): Promise<RuntimeRecord> {
  const typeMap: Record<string, number> = {
    libvirt: 1,
    gce: 2,
    lxd: 3,
  }

  const response = await page.request.post('/arca.v1.RuntimeService/CreateRuntime', {
    data: {
      name: options.name,
      type: typeMap[options.type] ?? 0,
      config: options.config,
    },
  })

  if (!response.ok()) {
    const payload = await parseJSONSafe(response)
    throw new Error(`CreateRuntime failed: ${response.status()} ${JSON.stringify(payload)}`)
  }

  const payload = (await response.json()) as {
    runtime?: { id?: string; name?: string; type?: string }
  }
  return {
    id: payload.runtime?.id?.trim() ?? '',
    name: payload.runtime?.name?.trim() ?? '',
    type: payload.runtime?.type?.trim() ?? '',
  }
}

export async function deleteRuntimeViaAPI(page: Page, runtimeId: string) {
  const response = await page.request.post('/arca.v1.RuntimeService/DeleteRuntime', {
    data: { runtimeId },
    failOnStatusCode: false,
  })

  if (!response.ok()) {
    const payload = await parseJSONSafe(response)
    const code = String(payload?.code ?? '').toLowerCase()
    if (code.includes('not_found')) {
      return
    }
    throw new Error(`DeleteRuntime failed: ${response.status()} ${JSON.stringify(payload)}`)
  }
}

export async function ensureLxdRuntime(
  page: Page,
): Promise<RuntimeRecord> {
  const serverPort = new URL(
    process.env.PLAYWRIGHT_BASE_URL ?? 'http://127.0.0.1:18080',
  ).port
  try {
    return await createRuntimeViaAPI(page, {
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
      const listResp = await page.request.post('/arca.v1.RuntimeService/ListRuntimes', {
        data: {},
      })
      const listPayload = (await listResp.json()) as {
        runtimes?: Array<{ id?: string; name?: string; type?: string }>
      }
      const existing = listPayload.runtimes?.find((r) => r.name === 'lxd-e2e')
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

export async function ensureLxdRuntimeWithProxyExposure(
  page: Page,
  opts?: { name?: string; domainPrefix?: string; baseDomain?: string },
): Promise<RuntimeRecord> {
  const name = opts?.name ?? 'lxd-proxy-e2e'
  const domainPrefix = opts?.domainPrefix ?? 'arca-'
  const baseDomain = opts?.baseDomain ?? 'localhost'
  const serverPort = new URL(
    process.env.PLAYWRIGHT_BASE_URL ?? 'http://127.0.0.1:18080',
  ).port

  try {
    return await createRuntimeViaAPI(page, {
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
      const listResp = await page.request.post('/arca.v1.RuntimeService/ListRuntimes', {
        data: {},
      })
      const listPayload = (await listResp.json()) as {
        runtimes?: Array<{ id?: string; name?: string; type?: string }>
      }
      const existing = listPayload.runtimes?.find((r) => r.name === name)
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
