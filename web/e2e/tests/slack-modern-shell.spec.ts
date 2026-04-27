import { test, expect, type Page } from '@playwright/test'

async function waitForReactMount(page: Page): Promise<void> {
  await page.waitForFunction(
    () => {
      const root = document.getElementById('root')
      if (!root) return false
      if (document.getElementById('skeleton')) return false
      return root.children.length > 0
    },
    { timeout: 10_000 },
  )
}

test.describe('slack-modern shell', () => {
  test('boots into Home with a primary rail', async ({ page }) => {
    await page.goto('/')
    await waitForReactMount(page)

    await expect(page.getByTestId('primary-rail')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('rail-item-home')).toHaveAttribute('aria-pressed', 'true')
    await expect(page.getByTestId('context-sidebar')).toBeVisible()
    await expect(page.locator('[data-home-surface="studio"]')).toBeVisible()
  })
})
