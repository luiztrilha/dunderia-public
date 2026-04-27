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

test.describe('message deletion', () => {
  test('shows a visual delete option for any deletable leaf message', async ({ page }) => {
    let deleted = false;
    const messageId = 'msg-delete-ui';

    await page.route('**/api/messages?**channel=general**', async (route) => {
      const url = new URL(route.request().url());
      if (url.searchParams.get('thread_id')) {
        await route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ channel: 'general', messages: [], execution_nodes: [], has_more: false }),
        });
        return;
      }
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          channel: 'general',
          execution_nodes: [],
          has_more: false,
          messages: deleted ? [] : [{
            id: messageId,
            from: 'ceo',
            channel: 'general',
            content: 'Delete me from the timeline',
            timestamp: '2026-04-23T18:30:00Z',
            tagged: [],
            can_delete: true,
          }],
        }),
      });
    });

    await page.route('**/api/messages', async (route) => {
      if (route.request().method() !== 'DELETE') {
        await route.fallback();
        return;
      }
      deleted = true;
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ ok: true, id: messageId, channel: 'general', total: 0 }),
      });
    });

    await page.route('**/api/messages/threads**', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ threads: [] }),
      });
    });

    await page.goto('/#/channels/general');
    await waitForReactMount(page);

    const messageBubble = page.locator(`[data-msg-id="${messageId}"]`);
    await expect(messageBubble).toContainText('Delete me from the timeline');
    await messageBubble.hover();

    const deleteButton = messageBubble.getByRole('button', { name: /Delete message|Excluir mensagem/i });
    await expect(deleteButton).toBeVisible();
    await deleteButton.click();
    await page.locator('.confirm-card').getByRole('button', { name: /Delete|Excluir/i }).click();

    await expect(messageBubble).toHaveCount(0);
    await expect(page.getByText(/is empty\. For now\.|está vazio\. Por enquanto\./i)).toBeVisible();
  });
});
