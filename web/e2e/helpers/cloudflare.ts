import { test, type APIResponse, type Page } from '@playwright/test'

type CloudflareConfig = {
  cloudflareToken: string
  cloudflareAccountID: string
  cloudflareZoneID: string
  baseDomain: string
  domainPrefix: string
}

const requiredCloudflareEnvVarNames = [
  'CLOUDFLARE_TOKEN',
  'CLOUDFLARE_ACCOUNT_ID',
  'CLOUDFLARE_ZONE_ID',
  'BASE_DOMAIN',
  'DOMAIN_PREFIX',
] as const

function trimEnv(value: string | undefined): string {
  return value?.trim() ?? ''
}

async function parseJSONSafe(response: APIResponse): Promise<Record<string, unknown> | null> {
  try {
    return (await response.json()) as Record<string, unknown>
  } catch {
    return null
  }
}

function cloudflareEnvConfig(): { config: CloudflareConfig | null; missing: string[] } {
  const config: CloudflareConfig = {
    cloudflareToken: trimEnv(process.env.CLOUDFLARE_TOKEN),
    cloudflareAccountID: trimEnv(process.env.CLOUDFLARE_ACCOUNT_ID),
    cloudflareZoneID: trimEnv(process.env.CLOUDFLARE_ZONE_ID),
    baseDomain: trimEnv(process.env.BASE_DOMAIN).toLowerCase(),
    domainPrefix: trimEnv(process.env.DOMAIN_PREFIX).toLowerCase(),
  }

  const missing: string[] = []
  for (const key of requiredCloudflareEnvVarNames) {
    if (trimEnv(process.env[key]) === '') {
      missing.push(key)
    }
  }

  if (missing.length > 0) {
    return { config: null, missing }
  }
  return { config, missing: [] }
}

export function cloudflareIntegrationConfig(): {
  config: CloudflareConfig | null
  missing: string[]
} {
  return cloudflareEnvConfig()
}

export function skipCloudflareIntegrationIfMissing() {
  const { missing } = cloudflareEnvConfig()
  const guidance =
    'Cloudflare integration E2E requires env vars: CLOUDFLARE_TOKEN, CLOUDFLARE_ACCOUNT_ID, CLOUDFLARE_ZONE_ID, BASE_DOMAIN, DOMAIN_PREFIX'
  test.skip(missing.length > 0, `${guidance}. Missing: ${missing.join(', ')}`)
}

export async function validateCloudflareToken(page: Page, config: CloudflareConfig) {
  const response = await page.request.post('/arca.v1.SetupService/ValidateCloudflareToken', {
    data: {
      apiToken: config.cloudflareToken,
      accountId: config.cloudflareAccountID,
    },
  })

  if (!response.ok()) {
    const payload = await parseJSONSafe(response)
    throw new Error(
      `ValidateCloudflareToken failed: ${response.status()} ${JSON.stringify(payload)}`,
    )
  }

  const payload = await parseJSONSafe(response)
  const valid = payload?.valid === true
  if (!valid) {
    throw new Error(
      `ValidateCloudflareToken returned invalid for account/zone config: ${JSON.stringify(payload)}`,
    )
  }
}

export type { CloudflareConfig }
