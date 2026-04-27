import { expect, test, type Page } from '@playwright/test'

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

test.describe('inline thread noise reduction', () => {
  test('surfaces thread context and collapses automation bursts', async ({ page, request }) => {
    const channel = 'e2e-inline-thread-noise'

    try {
      await request.post('/api/channels', {
        data: {
          action: 'create',
          slug: channel,
          name: 'E2E Inline Thread Noise',
          description: 'Isolated inline thread noise test channel',
          created_by: 'you',
        },
      })
      await request.post('/api/channels/clear', { data: { channel } })

      const rootResponse = await request.post('/api/messages', {
        data: {
          from: 'you',
          channel,
          content: 'Inline thread root context',
        },
      })
      const rootBody = await rootResponse.json()

      await request.post('/api/messages', {
        data: {
          from: 'nex',
          channel,
          content: 'Automation pulse 1',
          reply_to: rootBody.id,
        },
      })
      await request.post('/api/messages', {
        data: {
          from: 'nex',
          channel,
          content: 'Automation pulse 2',
          reply_to: rootBody.id,
        },
      })
      await request.post('/api/messages', {
        data: {
          from: 'nex',
          channel,
          content: 'Automation pulse 3',
          reply_to: rootBody.id,
        },
      })
      await request.post('/api/messages', {
        data: {
          from: 'ceo',
          channel,
          content: 'Human follow-up after automation',
          reply_to: rootBody.id,
        },
      })

      await page.goto(`/#/channels/${channel}`)
      await waitForReactMount(page)

      await expect(page.locator('.message-text-plain').filter({ hasText: 'Inline thread root context' }).first()).toBeVisible({
        timeout: 10_000,
      })
      await page.getByRole('button', { name: /Reply|Responder/i }).first().click()

      const threadPanel = page.locator('.inline-thread-panel')
      await expect(threadPanel).toBeVisible({ timeout: 10_000 })

      const context = page.getByTestId('inline-thread-context')
      await expect(context).toBeVisible()
      await expect(context).toContainText('Inline thread root context')
      await expect(context).toContainText('Human follow-up after automation')

      const automationGroup = page.getByTestId('inline-thread-automation-group')
      await expect(automationGroup).toBeVisible()
      await expect(automationGroup).not.toContainText('Automation pulse 1')

      await automationGroup.getByRole('button').click()
      await expect(automationGroup).toContainText('Automation pulse 1')
      await expect(automationGroup).toContainText('Automation pulse 2')
      await expect(automationGroup).toContainText('Automation pulse 3')
    } finally {
      await request.post('/api/channels', {
        data: {
          action: 'remove',
          slug: channel,
        },
      })
    }
  })
})
