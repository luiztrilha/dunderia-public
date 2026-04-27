import { test, expect, type Page } from '@playwright/test';

async function waitForReactMount(page: Page): Promise<void> {
  await page.waitForFunction(
    () => {
      const root = document.getElementById('root');
      if (!root) return false;
      if (document.getElementById('skeleton')) return false;
      return root.children.length > 0;
    },
    { timeout: 10_000 },
  );
}

test.describe('search modal message search', () => {
  test('search ignores stale responses from an older query', async ({ page }) => {
    await page.route('**/api/onboarding/state', async (route) => {
      await route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({ onboarded: true }),
      });
    });

    await page.route('**/api/channels', async (route) => {
      await route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({
          channels: [
            {
              slug: 'search-lab',
              name: 'Search Lab',
              description: 'Race-safe search fixtures',
            },
          ],
        }),
      });
    });

    await page.route('**/api/office-members', async (route) => {
      await route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({ members: [] }),
      });
    });

    await page.route('**/api/config', async (route) => {
      await route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({}),
      });
    });

    let messageCallCount = 0;
    await page.route('**/api/messages**', async (route) => {
      messageCallCount += 1;
      await page.waitForTimeout(messageCallCount === 1 ? 400 : 20);
      await route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({
          messages: [
            {
              id: 'alpha-hit',
              from: 'ceo',
              channel: 'search-lab',
              content: 'alpha issue',
              timestamp: '2026-04-21T10:00:00Z',
            },
            {
              id: 'beta-hit',
              from: 'ceo',
              channel: 'search-lab',
              content: 'beta issue',
              timestamp: '2026-04-21T10:01:00Z',
            },
          ],
          execution_nodes: [],
          has_more: false,
        }),
      });
    });

    await page.goto('/#/apps/settings');
    await waitForReactMount(page);

    await page.getByRole('button', { name: /Search|Buscar/i }).click();

    const input = page.getByPlaceholder(/Search channels, agents, commands, messages|Buscar canais, agentes, comandos, mensagens/i);
    await expect(input).toBeVisible({ timeout: 10_000 });

    await input.fill('alpha');
    await page.waitForTimeout(320);
    await input.fill('beta');

    await expect(page.getByText(/ceo: beta issue/i)).toBeVisible({ timeout: 10_000 });
    await expect(page.getByText(/ceo: alpha issue/i)).toHaveCount(0);
  });
});
