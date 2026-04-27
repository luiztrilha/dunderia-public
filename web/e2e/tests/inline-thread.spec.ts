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

test.describe('inline thread interaction', () => {
  test('clicking thread actions opens the thread inline instead of the side panel', async ({ page, request }) => {
    const channel = 'e2e-inline-thread';
    try {
      await request.post('/api/channels', {
        data: {
          action: 'create',
          slug: channel,
          name: 'E2E Inline Thread',
          description: 'Isolated thread test channel',
          created_by: 'you',
        },
      });
      await request.post('/api/channels/clear', { data: { channel } });
      const root = await request.post('/api/messages', {
        data: {
          from: 'you',
          channel,
          content: 'Inline thread root probe',
        },
      });
      const rootBody = await root.json();

      await request.post('/api/messages', {
        data: {
          from: 'ceo',
          channel,
          content: 'Inline thread child probe',
          reply_to: rootBody.id,
        },
      });

      await page.goto(`/#/channels/${channel}`);
      await waitForReactMount(page);

      await expect(
        page.locator('.message-text-plain').filter({ hasText: 'Inline thread root probe' }).first(),
      ).toBeVisible({ timeout: 10_000 });
      await page.getByRole('button', { name: /Reply|Responder/i }).first().click();

      await expect(page.locator('.thread-panel.open')).toHaveCount(0);
      const inlineThread = page.locator('.inline-thread-panel');
      await expect(inlineThread).toBeVisible({ timeout: 10_000 });
      await expect(inlineThread).toContainText('Inline thread child probe', { timeout: 10_000 });
    } finally {
      await request.post('/api/channels', {
        data: {
          action: 'remove',
          slug: channel,
        },
      });
    }
  });
});
