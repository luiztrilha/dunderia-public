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

const PROTECTED_CHANNEL_SLUGS = new Set(['migracao-exampleworkflow', 'geral', 'general']);
const SAFE_HASH_SLUG = 'e2e-hash-router-smoke';

function assertSafeHash(hash: string): void {
  const match = hash.match(/^\/#\/channels\/([^/?#]+)/);
  if (!match) return;
  const slugEncoded = match[1] || '';
  try {
    const slug = decodeURIComponent(slugEncoded);
    if (PROTECTED_CHANNEL_SLUGS.has(slug)) {
      throw new Error(`Blocked protected slug in e2e route: ${slug}`);
    }
  } catch {
    // allow intentionally malformed hashes; parsing/normalization is validated by app logic.
    return;
  }
}

async function gotoSafeChannel(page: Page, hash: string): Promise<void> {
  assertSafeHash(hash);
  await page.goto(hash);
}

function collectErrors(page: Page): () => string[] {
  const errors: string[] = [];
  page.on('pageerror', (error) => errors.push(error.message));
  page.on('console', (msg) => {
    if (msg.type() === 'error') {
      const text = msg.text();
      if (text.includes('Error boundary') || text.includes('Minified React error')) {
        errors.push(text);
      }
    }
  });
  return () => errors;
}

test.describe('hash routing resilience', () => {
  test('loads direct channel route without mutating channels', async ({ page }) => {
    const getErrors = collectErrors(page);
    await gotoSafeChannel(page, `/#/channels/${SAFE_HASH_SLUG}`);
    await waitForReactMount(page);

    await expect(page.getByTestId('error-boundary')).toHaveCount(0);

    const errors = getErrors();
    expect(errors).toHaveLength(0);
  });

  test('loads direct dm route without posting channel creation', async ({ page }) => {
    const getErrors = collectErrors(page);
    let dmPostCount = 0;

    await page.route('**/api/channels/dm', async (route) => {
      dmPostCount += 1;
      await route.fulfill({
        contentType: 'application/json',
        body: JSON.stringify({ slug: 'dm-reviewer' }),
      });
    });

    await page.goto('/#/dm/reviewer');
    await waitForReactMount(page);

    await expect(page.getByTestId('error-boundary')).toHaveCount(0);
    expect(dmPostCount).toBe(0);

    const errors = getErrors();
    expect(errors).toHaveLength(0);
  });

  test('falls back safely on malformed channel hash without hard failure', async ({ page }) => {
    const getErrors = collectErrors(page);
    await gotoSafeChannel(page, '/#/channels/%E0%A');
    await page.waitForLoadState('domcontentloaded');
    await page.waitForSelector('#root', { state: 'attached', timeout: 10_000 });

    await expect(page.getByTestId('error-boundary')).toHaveCount(0);
    await expect(page).toHaveURL(/\/#\/channels\//);

    const errors = getErrors();
    expect(errors).toHaveLength(0);
  });
});

