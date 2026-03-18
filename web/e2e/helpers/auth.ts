import { expect, type APIResponse, type Browser, type Page } from '@playwright/test'

export const adminEmail = 'admin@example.com'
export const adminPassword = 'password123'

const defaultSetupConfig = {
  baseDomain: 'example.com',
  domainPrefix: 'arca-',
}

async function parseJSONSafe(response: APIResponse): Promise<Record<string, unknown> | null> {
  try {
    return (await response.json()) as Record<string, unknown>
  } catch {
    return null
  }
}

export async function ensureSetupAdmin(page: Page) {
  const setupConfig = defaultSetupConfig

  const response = await page.request.post('/arca.v1.SetupService/CompleteSetup', {
    data: {
      adminEmail,
      adminPassword,
      baseDomain: setupConfig.baseDomain,
      domainPrefix: setupConfig.domainPrefix,
      serverDomain: setupConfig.baseDomain,
    },
  })

  if (response.ok()) {
    return
  }

  const payload = await parseJSONSafe(response)
  const code = String(payload?.code ?? '').toLowerCase()
  if (code === 'failed_precondition' || code === 'already_exists') {
    return
  }

  throw new Error(`setup failed: ${response.status()} ${JSON.stringify(payload)}`)
}

export async function loginAsAdmin(page: Page) {
  await ensureSetupAdmin(page)
  await page.goto('/login')
  await page.getByLabel('Email').fill(adminEmail)
  await page.getByLabel('Password', { exact: true }).fill(adminPassword)
  await page.getByRole('button', { name: 'Login' }).click()
  await expect(page).toHaveURL('/machines')
}

export async function loginAsUser(page: Page, email: string, password: string) {
  await page.goto('/login')
  await page.getByLabel('Email').fill(email)
  await page.getByLabel('Password', { exact: true }).fill(password)
  await page.getByRole('button', { name: 'Login' }).click()
  await expect(page).toHaveURL('/machines')
}

export async function createSecondaryUser(
  page: Page,
  email: string,
): Promise<{ userId: string; setupToken: string }> {
  const response = await page.request.post('/arca.v1.UserService/CreateUser', {
    data: { email },
  })

  if (!response.ok()) {
    const payload = await parseJSONSafe(response)
    throw new Error(`CreateUser failed: ${response.status()} ${JSON.stringify(payload)}`)
  }

  const payload = (await response.json()) as {
    user?: { id?: string }
    setupToken?: string
  }

  const userId = payload.user?.id?.trim() ?? ''
  const setupToken = payload.setupToken?.trim() ?? ''
  expect(userId).not.toBe('')
  expect(setupToken).not.toBe('')
  return { userId, setupToken }
}

export async function completeUserSetup(page: Page, setupToken: string, password: string) {
  const response = await page.request.post('/arca.v1.UserService/CompleteUserSetup', {
    data: { setupToken, password },
  })

  if (!response.ok()) {
    const payload = await parseJSONSafe(response)
    throw new Error(`CompleteUserSetup failed: ${response.status()} ${JSON.stringify(payload)}`)
  }
}

export async function createAuthedUserContext(
  browser: Browser,
  baseURL: string,
  email: string,
  password: string,
): Promise<{ page: Page; close: () => Promise<void> }> {
  const context = await browser.newContext({ baseURL })
  const page = await context.newPage()
  await loginAsUser(page, email, password)
  return {
    page,
    close: () => context.close(),
  }
}
