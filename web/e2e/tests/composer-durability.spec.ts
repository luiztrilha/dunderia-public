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

test.describe('composer durability', () => {
  test('draft survives reload and only clears after persisted ack', async ({ page, request }) => {
    const channel = 'e2e-composer-durability';
    const content = 'erro do legado\nstack trace importante';

    try {
      await request.post('/api/channels', {
        data: {
          action: 'create',
          slug: channel,
          name: 'E2E Composer Durability',
          description: 'Isolated composer durability channel',
          created_by: 'you',
        },
      });
      await request.post('/api/channels/clear', { data: { channel } });

      await page.goto(`/#/channels/${channel}`);
      await waitForReactMount(page);

      const composer = page.locator('.composer-input').first();
      await composer.fill(content);
      await page.reload();
      await waitForReactMount(page);
      await expect(page.locator('.composer-input').first()).toHaveValue(content);

      await page.locator('.composer-send').first().click();
      await expect(page.locator('.composer-status')).toContainText(/persist(ed|ing)|persistida|persistido/i);
      await expect(page.locator('.composer-input').first()).toHaveValue('');

      await page.reload();
      await waitForReactMount(page);
      await expect(page.locator('.composer-input').first()).toHaveValue('');
    } finally {
      await request.post('/api/channels', {
        data: {
          action: 'remove',
          slug: channel,
        },
      });
    }
  });

  test('failed send keeps the draft visible', async ({ page, request }) => {
    const channel = 'e2e-composer-failed-send';
    const content = 'erro critico do sistema legado';

    try {
      await request.post('/api/channels', {
        data: {
          action: 'create',
          slug: channel,
          name: 'E2E Composer Failed Send',
          description: 'Isolated composer failed send channel',
          created_by: 'you',
        },
      });
      await request.post('/api/channels/clear', { data: { channel } });

      await page.goto(`/#/channels/${channel}`);
      await waitForReactMount(page);

      const composer = page.locator('.composer-input').first();
      await composer.fill(content);

      await request.post('/api/channels', {
        data: {
          action: 'remove',
          slug: channel,
        },
      });

      await page.locator('.composer-send').first().click();
      await expect(page.locator('.composer-input').first()).toHaveValue(content);
      await expect(page.locator('.composer-status')).toContainText(/Falha|Failed/);
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
