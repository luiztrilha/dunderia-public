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

test.describe('config draft durability', () => {
  test('channel wizard restores manual draft after reload', async ({ page }) => {
    await page.goto('/#/channels/general');
    await waitForReactMount(page);

    await page.getByRole('button', { name: /new channel/i }).click();
    await page.getByRole('button', { name: /manual/i }).click();
    await page.locator('#channel-name').fill('Convenios Legado');
    await page.locator('#channel-description').fill('Incidentes e contexto do legado');

    await page.reload();
    await waitForReactMount(page);

    await page.getByRole('button', { name: /new channel/i }).click();
    await expect(page.locator('#channel-name')).toHaveValue('Convenios Legado');
    await expect(page.locator('#channel-description')).toHaveValue('Incidentes e contexto do legado');
  });

  test('agent wizard restores manual draft after reload', async ({ page }) => {
    await page.goto('/#/channels/general');
    await waitForReactMount(page);

    await page.getByRole('button', { name: /new agent/i }).click();
    await page.getByRole('button', { name: /manual/i }).click();
    await page.locator('#agent-name').fill('Reviewer Persistente');
    await page.locator('#agent-role').fill('Auditoria de confiabilidade');

    await page.reload();
    await waitForReactMount(page);

    await page.getByRole('button', { name: /new agent/i }).click();
    await expect(page.locator('#agent-name')).toHaveValue('Reviewer Persistente');
    await expect(page.locator('#agent-role')).toHaveValue('Auditoria de confiabilidade');
  });

  test('policies app restores unsaved rule after reload', async ({ page }) => {
    await page.goto('/#/apps/policies');
    await waitForReactMount(page);
    const draftText = 'Nunca perder contexto de incidentes do legado';

    const addRuleButton = page.getByRole('button', { name: /\+?\s*(add rule|adicionar regra)/i });
    const ruleInput = page.locator('input[type="text"]').first();

    await addRuleButton.click();
    await ruleInput.fill(draftText);

    const rawDraft = await page.evaluate(() => window.localStorage.getItem('dunderia.policies.ruleDraft.v1'));
    expect(JSON.parse(rawDraft || '""')).toBe(draftText);

    await page.reload();
    await waitForReactMount(page);

    const persistedDraft = JSON.parse((await page.evaluate(() => window.localStorage.getItem('dunderia.policies.ruleDraft.v1')) || '""'));
    expect(persistedDraft).toBe(draftText);

    if (!(await ruleInput.isVisible().catch(() => false))) {
      await addRuleButton.click();
    }

    const inputCount = await page.locator('input[type=\"text\"]').count();
    if (inputCount > 0) {
      await expect(page.locator('input[type=\"text\"]').first()).toHaveValue(draftText);
    }
  });

  test('settings app restores unsaved drafts across sections after reload', async ({ page }) => {
    await page.goto('/#/apps/settings');
    await waitForReactMount(page);

    await page.getByPlaceholder('e.g. ceo').fill('coordenacao-persistente');
    await page.getByPlaceholder('Operation blueprint ID').fill('legacy-stabilization');

    await page.getByRole('button', { name: /Company|Empresa/i }).click();
    await page.getByPlaceholder('Acme Corp').fill('Convenios Web');

    await page.getByRole('button', { name: /API Keys|Chaves de API/i }).click();
    await page.locator('input[type="password"]').first().fill('nex_persisted_key');

    await page.getByRole('button', { name: /Integrations|Integrações/i }).click();
    await page.getByPlaceholder('D:/Repos/dunderia/mcp/dunderia-mcp-settings.json').fill('D:/Repos/dunderia/mcp/custom-persisted.json');

    await page.getByRole('button', { name: /Polling|Sondagem/i }).click();
    await page.getByPlaceholder('60').fill('77');

    await page.reload();
    await waitForReactMount(page);

    await page.getByRole('button', { name: /^(\u2699\s*)?(General|Geral)$/i }).click();
    await expect(page.getByPlaceholder(/e\.g\. ceo|ex\.: ceo/i)).toHaveValue('coordenacao-persistente');
    await expect(page.getByPlaceholder(/Operation blueprint ID|ID do blueprint de operação/i)).toHaveValue('legacy-stabilization');

    await page.getByRole('button', { name: /Company|Empresa/i }).click();
    await expect(page.getByPlaceholder('Acme Corp')).toHaveValue('Convenios Web');

    await page.getByRole('button', { name: /API Keys|Chaves de API/i }).click();
    const keysDraft = await page.evaluate(() => window.localStorage.getItem('settings-draft:keys'));
    expect(keysDraft).toBeNull();
    await expect(page.locator('input[type="password"]').first()).toHaveValue('');

    await page.getByRole('button', { name: /Integrations|Integrações/i }).click();
    await expect(page.getByPlaceholder('D:/Repos/dunderia/mcp/dunderia-mcp-settings.json')).toHaveValue('D:/Repos/dunderia/mcp/custom-persisted.json');

    await page.getByRole('button', { name: /Polling|Sondagem/i }).click();
    await expect(page.getByPlaceholder('60')).toHaveValue('77');
  });

  test('settings app keeps integration draft stable during background refetch', async ({ page }) => {
    await page.goto('/#/apps/settings');
    await waitForReactMount(page);

    await page.getByRole('button', { name: /Integrations|Integrações/i }).click();

    const input = page.getByPlaceholder('D:/Repos/dunderia/mcp/dunderia-mcp-settings.json');
    await input.fill('D:/Repos/dunderia/mcp/in-flight-draft.json');
    await input.focus();

    await page.waitForTimeout(6500);

    await expect(input).toHaveValue('D:/Repos/dunderia/mcp/in-flight-draft.json');
    await expect(input).toBeFocused();
  });
});
