import { expect, test } from '@playwright/test'
import { loginAsAdmin } from './helpers/auth'

test.describe('LLM models settings', () => {
  test('settings page shows LLM Models section', async ({ page }) => {
    await loginAsAdmin(page)
    await page.goto('/settings')
    await expect(page.locator('[data-slot="card-title"]').getByText('LLM Models')).toBeVisible()
    await expect(page.getByText('No LLM models configured yet')).toBeVisible()
    await expect(page.getByRole('button', { name: 'Add model' })).toBeVisible()
  })

  test('create, edit, and delete LLM model', async ({ page }) => {
    await loginAsAdmin(page)
    await page.goto('/settings')

    // Open create dialog
    await page.getByRole('button', { name: 'Add model' }).click()
    await expect(page.getByText('Add LLM Model')).toBeVisible()

    // Fill in the form
    await page.getByLabel('Config name').fill('test-model')
    await page.getByLabel('Model name').fill('gpt-4o')
    await page.getByLabel('API key').fill('sk-test-key-12345')
    await page.getByLabel('Max context tokens').fill('128000')

    // Submit
    await page.getByRole('button', { name: 'Create' }).click()

    // Verify model appears in list
    await expect(page.getByText('test-model')).toBeVisible()
    await expect(page.getByText('gpt-4o')).toBeVisible()
    await expect(page.getByText('128,000 tokens')).toBeVisible()

    // Edit the model
    await page.getByRole('button', { name: 'Edit' }).click()
    await expect(page.getByText('Edit LLM Model')).toBeVisible()

    await page.getByLabel('Config name').fill('test-model-updated')
    await page.getByLabel('Model name').fill('gpt-4o-mini')
    await page.getByRole('button', { name: 'Update' }).click()

    // Verify updated
    await expect(page.getByText('test-model-updated')).toBeVisible()
    await expect(page.getByText('gpt-4o-mini')).toBeVisible()

    // Delete the model - click the Delete button on the model row
    await page.locator('button', { hasText: 'Delete' }).first().click()
    await expect(page.getByText('Delete LLM Model')).toBeVisible()
    // Click the destructive Delete button in the dialog
    await page.getByRole('button', { name: 'Delete' }).filter({ hasText: 'Delete' }).last().click()

    // Verify deleted
    await expect(page.getByText('No LLM models configured yet')).toBeVisible()
  })

  test('validation prevents creating model with empty fields', async ({ page }) => {
    await loginAsAdmin(page)
    await page.goto('/settings')

    await page.getByRole('button', { name: 'Add model' }).click()

    // Try to create with empty fields
    await page.getByRole('button', { name: 'Create' }).click()

    // Should show error
    await expect(page.locator('.text-red-300')).toBeVisible()
  })
})
