import type { Page } from '@playwright/test'

const MOCK_SERVICE_BASE = '/arca.v1.MockService'

async function callMockService(page: Page, method: string, data: Record<string, unknown> = {}) {
  const response = await page.request.post(`${MOCK_SERVICE_BASE}/${method}`, { data })
  if (!response.ok()) {
    const body = await response.text()
    throw new Error(`MockService.${method} failed: ${response.status()} ${body}`)
  }
  return response.json()
}

export async function setDefaultBehavior(page: Page, behavior: { delayMs?: number; errorOn?: Record<string, string> }) {
  await callMockService(page, 'SetDefaultBehavior', { behavior })
}

export async function setMachineBehavior(page: Page, machineId: string, behavior: { delayMs?: number; errorOn?: Record<string, string> }) {
  await callMockService(page, 'SetMachineBehavior', { machineId, behavior })
}

export async function resetBehavior(page: Page) {
  await callMockService(page, 'ResetBehavior', {})
}
