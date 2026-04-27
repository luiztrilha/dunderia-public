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

test.describe('message feed noise reduction', () => {
  test('surfaces recent context and collapses automation bursts', async ({ page, request }) => {
    const channel = 'e2e-message-feed-noise';
    const substantiveIntro = 'Please keep this channel readable for humans.';
    const substantiveUpdate = 'Atualizei a validacao do extrato e removi a excecao principal.';

    try {
      await request.post('/api/channels', {
        data: {
          action: 'create',
          slug: channel,
          name: 'E2E Message Feed Noise',
          description: 'Isolated feed compaction test channel',
          created_by: 'you',
        },
      });
      await request.post('/api/channels/clear', { data: { channel } });

      await request.post('/api/messages', {
        data: {
          from: 'you',
          channel,
          content: substantiveIntro,
        },
      });

      await request.post('/api/notifications/nex', {
        data: {
          channel,
          source: 'github',
          source_label: 'GitHub',
          title: 'Checks',
          content: 'Checks passed for PR #42',
          event_id: 'e2e-feed-noise-1',
        },
      });
      await request.post('/api/notifications/nex', {
        data: {
          channel,
          source: 'github',
          source_label: 'GitHub',
          title: 'Label sync',
          content: 'Labels refreshed on PR #42',
          event_id: 'e2e-feed-noise-2',
        },
      });
      await request.post('/api/notifications/nex', {
        data: {
          channel,
          source: 'github',
          source_label: 'GitHub',
          title: 'Reviewer ping',
          content: 'Reviewer requested a follow-up on PR #42',
          event_id: 'e2e-feed-noise-3',
        },
      });

      await request.post('/api/messages', {
        data: {
          from: 'you',
          channel,
          content: substantiveUpdate,
        },
      });

      await page.goto(`/#/channels/${channel}`);
      await waitForReactMount(page);

      const contextCard = page.getByTestId('message-feed-context');
      await expect(contextCard).toBeVisible({ timeout: 10_000 });
      await expect(contextCard).toContainText(substantiveIntro);
      await expect(contextCard).toContainText(substantiveUpdate);

      const automationGroup = page.getByTestId('message-feed-automation-group');
      await expect(automationGroup).toBeVisible();
      await expect(automationGroup).toContainText(/3/);
      await expect(page.getByText('Checks passed for PR #42')).toHaveCount(0);

      await automationGroup.getByRole('button', { name: /show|mostrar/i }).click();
      await expect(page.getByText('Checks passed for PR #42')).toBeVisible();
      await expect(page.getByText('Labels refreshed on PR #42')).toBeVisible();
      await expect(page.getByText('Reviewer requested a follow-up on PR #42', { exact: true })).toBeVisible();
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
