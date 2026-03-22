import type { APIResponse, Page } from '@playwright/test'

async function parseJSONSafe(response: APIResponse): Promise<Record<string, unknown> | null> {
  try {
    return (await response.json()) as Record<string, unknown>
  } catch {
    return null
  }
}

type ProfileRecord = {
  id: string
  name: string
  type: string
}

type CreateProfileOptions = {
  name: string
  type: string
  config: Record<string, unknown>
}

export async function createMachineProfileViaAPI(
  page: Page,
  options: CreateProfileOptions,
): Promise<ProfileRecord> {
  const typeMap: Record<string, number> = {
    libvirt: 1,
    gce: 2,
    lxd: 3,
  }

  const response = await page.request.post('/arca.v1.MachineProfileService/CreateMachineProfile', {
    data: {
      name: options.name,
      type: typeMap[options.type] ?? 0,
      config: options.config,
    },
  })

  if (!response.ok()) {
    const payload = await parseJSONSafe(response)
    throw new Error(`CreateMachineProfile failed: ${response.status()} ${JSON.stringify(payload)}`)
  }

  const payload = (await response.json()) as {
    profile?: { id?: string; name?: string; type?: string }
  }
  return {
    id: payload.profile?.id?.trim() ?? '',
    name: payload.profile?.name?.trim() ?? '',
    type: payload.profile?.type?.trim() ?? '',
  }
}

export async function deleteMachineProfileViaAPI(page: Page, profileId: string) {
  const response = await page.request.post('/arca.v1.MachineProfileService/DeleteMachineProfile', {
    data: { profileId },
    failOnStatusCode: false,
  })

  if (!response.ok()) {
    const payload = await parseJSONSafe(response)
    const code = String(payload?.code ?? '').toLowerCase()
    if (code.includes('not_found')) {
      return
    }
    throw new Error(`DeleteMachineProfile failed: ${response.status()} ${JSON.stringify(payload)}`)
  }
}

export async function ensureLxdProfile(
  page: Page,
): Promise<ProfileRecord> {
  const serverPort = new URL(
    process.env.PLAYWRIGHT_BASE_URL ?? 'http://127.0.0.1:18080',
  ).port
  try {
    return await createMachineProfileViaAPI(page, {
      name: 'lxd-e2e',
      type: 'lxd',
      config: {
        lxd: { endpoint: 'https://localhost:8443' },
        exposure: {
          method: 2, // PROXY_VIA_SERVER
          connectivity: 1, // PRIVATE_IP
        },
        serverApiUrl: `http://10.200.0.1:${serverPort}`,
      },
    })
  } catch (error) {
    if (String(error).includes('already_exists')) {
      const listResp = await page.request.post('/arca.v1.MachineProfileService/ListMachineProfiles', {
        data: {},
      })
      const listPayload = (await listResp.json()) as {
        profiles?: Array<{ id?: string; name?: string; type?: string }>
      }
      const existing = listPayload.profiles?.find((r) => r.name === 'lxd-e2e')
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

export async function ensureLxdProfileWithProxyExposure(
  page: Page,
  opts?: { name?: string },
): Promise<ProfileRecord> {
  const name = opts?.name ?? 'lxd-proxy-e2e'
  const serverPort = new URL(
    process.env.PLAYWRIGHT_BASE_URL ?? 'http://127.0.0.1:18080',
  ).port

  try {
    return await createMachineProfileViaAPI(page, {
      name,
      type: 'lxd',
      config: {
        lxd: { endpoint: 'https://localhost:8443' },
        exposure: {
          method: 2, // PROXY_VIA_SERVER
          connectivity: 1, // PRIVATE_IP
        },
        serverApiUrl: `http://10.200.0.1:${serverPort}`,
      },
    })
  } catch (error) {
    if (String(error).includes('already_exists')) {
      const listResp = await page.request.post('/arca.v1.MachineProfileService/ListMachineProfiles', {
        data: {},
      })
      const listPayload = (await listResp.json()) as {
        profiles?: Array<{ id?: string; name?: string; type?: string }>
      }
      const existing = listPayload.profiles?.find((r) => r.name === name)
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
