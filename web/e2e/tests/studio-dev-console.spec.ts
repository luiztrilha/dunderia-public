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

test.describe('studio dev console', () => {
  test('renders the three dev-console zones without crashing', async ({ page }) => {
    const errors: string[] = []
    page.on('pageerror', (err) => errors.push(err.message))

    await page.goto('/#/apps/studio')
    await waitForReactMount(page)

    await expect(page.getByTestId('studio-dev-console')).toBeVisible({ timeout: 10_000 })
    await expect(page.getByTestId('studio-office-snapshot')).toBeVisible()
    await expect(page.getByTestId('studio-active-context')).toBeVisible()
    await expect(page.getByTestId('studio-blockers')).toBeVisible()
    await expect(page.getByTestId('error-boundary')).toHaveCount(0)
    expect(errors).toHaveLength(0)
  })
})
