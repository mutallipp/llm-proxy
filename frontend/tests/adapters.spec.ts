import { test, expect } from '@playwright/test'
import { gotoAndEnsureAuth } from './auth.utils'

test.describe('Admin Adapters Management', () => {
  test('renders adapters page without the navigation hook runtime error', async ({ page }) => {
    const pageErrors: Error[] = []
    page.on('pageerror', (error) => pageErrors.push(error))

    await gotoAndEnsureAuth(page, '/adapters')

    await expect(page.getByTestId('adapters-page-heading')).toBeVisible()
    await expect(page).not.toHaveTitle('500')
    expect(pageErrors.map((error) => error.message)).not.toContain('useNavigate is not defined')
  })
})
