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

test.describe('message formatting', () => {
  test('human messages preserve explicit line breaks', async ({ page, request }) => {
    const channel = 'e2e-message-formatting';
    const content = 'primeira linha\nsegunda linha\n\nquarta linha';

    try {
      await request.post('/api/channels', {
        data: {
          action: 'create',
          slug: channel,
          name: 'E2E Message Formatting',
          description: 'Isolated formatting test channel',
          created_by: 'you',
        },
      });
      await request.post('/api/channels/clear', { data: { channel } });
      await request.post('/api/messages', {
        data: {
          from: 'you',
          channel,
          content,
        },
      });

      await page.goto(`/#/channels/${channel}`);
      await waitForReactMount(page);

      const textBlock = page.locator('.message-text').first();
      await expect(textBlock).toBeVisible({ timeout: 10_000 });
      await expect(textBlock).toHaveJSProperty('innerText', content);
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
