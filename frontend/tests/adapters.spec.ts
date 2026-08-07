import { test, expect } from '@playwright/test'
import { gotoAndEnsureAuth } from './auth.utils'

const adapterFixture = {
  id: 1,
  name: 'e2e-adapter',
  display_name: 'E2E Adapter',
  inbound_api_format: 'openai/chat_completions',
  status: 'enabled',
  bindings: [
    {
      id: 1,
      source_model_id: 'e2e-source-model',
      model_id: 1,
      enabled: true,
    },
  ],
}

test.describe('Admin Adapters Management', () => {
  test('renders adapters page without the navigation hook runtime error', async ({ page }) => {
    const pageErrors: Error[] = []
    page.on('pageerror', (error) => pageErrors.push(error))

    await gotoAndEnsureAuth(page, '/adapters')

    await expect(page.getByTestId('adapters-page-heading')).toBeVisible()
    await expect(page).not.toHaveTitle('500')
    expect(pageErrors.map((error) => error.message)).not.toContain('useNavigate is not defined')
  })

  test('executes the fixed adapter test without calling a real provider', async ({ page }) => {
    let testAdapterInput: { adapter?: string; modelID?: string } | undefined

    await page.route('**/admin/gateway/adapters', async (route) => {
      if (route.request().method() !== 'GET') {
        await route.continue()
        return
      }

      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ adapters: [adapterFixture] }),
      })
    })
    await page.route('**/admin/graphql', async (route) => {
      const request = route.request()
      if (request.method() !== 'POST') {
        await route.continue()
        return
      }

      const payload = request.postDataJSON() as {
        operationName?: string
        variables?: { input?: { adapter?: string; modelID?: string } }
      }
      if (payload.operationName !== 'TestAdapter') {
        await route.continue()
        return
      }

      testAdapterInput = payload.variables?.input
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          data: {
            testAdapter: {
              latency: 0.01,
              success: true,
              message: 'ok',
              error: null,
              requestID: null,
            },
          },
        }),
      })
    })

    const pageErrors: Error[] = []
    page.on('pageerror', (error) => pageErrors.push(error))

    await gotoAndEnsureAuth(page, '/adapters')

    const listTestButton = page
      .locator('tbody tr')
      .getByRole('button', { name: '测试', exact: true })
      .first()
    await expect(listTestButton).toBeEnabled()
    await listTestButton.click()

    const dialog = page.getByRole('dialog')
    await expect(dialog).toBeVisible()
    await dialog.getByRole('button', { name: /执行固定测试|Run fixed test/i }).click()

    await expect(dialog).toContainText(/成功|Success|Succeeded/i)
    expect(testAdapterInput).toEqual({
      adapter: adapterFixture.name,
      modelID: adapterFixture.bindings[0].source_model_id,
    })
    expect(pageErrors.map((error) => error.message)).not.toContain('adapterName is not defined')
  })
})
