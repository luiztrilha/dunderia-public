import { test, expect, type Page, type Route } from '@playwright/test'

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

async function fulfillJson(route: Route, payload: unknown) {
  await route.fulfill({
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify(payload),
  })
}

async function installRuntimeSummaryMocks(page: Page) {
  await page.route('**/api/office-members', async (route) => {
    await fulfillJson(route, {
      members: [
        { slug: 'ceo', name: 'Coordenacao', role: 'Lead', provider: { kind: 'codex' } },
        { slug: 'builder', name: 'Full Stack', role: 'Builder', provider: { kind: 'codex' } },
        { slug: 'frontend', name: 'Frontend', role: 'Frontend', provider: { kind: 'claude-code' } },
        { slug: 'pm', name: 'Produto', role: 'PM', provider: { kind: 'gemini' } },
      ],
    })
  })

  await page.route('**/api/members?**channel=general**', async (route) => {
    await fulfillJson(route, {
      channel: 'general',
      members: [
        {
          slug: 'ceo',
          name: 'Coordenacao',
          role: 'Lead',
          status: 'idle',
          activity: 'idle',
          detail: 'waiting for work',
          liveActivity: 'waiting for work',
        },
        {
          slug: 'builder',
          name: 'Full Stack',
          role: 'Builder',
          status: 'active',
          activity: 'tool',
          detail: 'running tests',
          liveActivity: 'running tests',
        },
        {
          slug: 'frontend',
          name: 'Frontend',
          role: 'Frontend',
          status: 'idle',
          activity: 'idle',
        },
      ],
    })
  })

  await page.route('**/api/tasks?**channel=general**', async (route) => {
    await fulfillJson(route, {
      tasks: [
        {
          id: 'task-1',
          channel: 'general',
          title: 'Fix blocked layout',
          owner: 'frontend',
          status: 'blocked',
          blocked: true,
        },
      ],
    })
  })

  await page.route('**/api/tasks?**all_channels=true**', async (route) => {
    await fulfillJson(route, {
      tasks: [
        {
          id: 'task-1',
          channel: 'general',
          title: 'Fix blocked layout',
          owner: 'frontend',
          status: 'blocked',
          blocked: true,
        },
      ],
    })
  })

  await page.route('**/api/messages?**channel=general**', async (route) => {
    await fulfillJson(route, {
      channel: 'general',
      messages: [],
      execution_nodes: [
        {
          id: 'exec-1',
          root_message_id: 'msg-1',
          owner_agent: 'ceo',
          awaiting_human_input: true,
          awaiting_human_reason: 'Need approval',
          status: 'pending',
        },
      ],
      has_more: false,
    })
  })

  await page.route('**/api/requests**', async (route) => {
    await fulfillJson(route, { requests: [] })
  })

  await page.route('**/api/usage**', async (route) => {
    await fulfillJson(route, { total: { total_tokens: 0 } })
  })

  await page.route('**/api/studio/dev-console', async (route) => {
    await fulfillJson(route, {
      office: {
        status: 'healthy',
        provider: 'codex',
        focus_mode: true,
        memory_backend: 'none',
        health: {},
        bootstrap: {
          summary: 'Ready',
          members: 4,
          channels: 1,
          tasks: 1,
          requests: 0,
          workspaces: 0,
          workflows: 0,
        },
        task_counts: {
          total: 1,
          blocked: 1,
        },
      },
      environment: {
        status: 'healthy',
        broker_reachable: true,
        api_reachable: true,
        web_reachable: true,
        memory_backend_ready: true,
        degraded: false,
      },
      active_context: {
        primary_channel: 'general',
        focus: 'general',
        channels: [
          {
            slug: 'general',
            name: 'general',
            members: ['ceo', 'builder', 'frontend'],
            task_counts: { total: 1, blocked: 1 },
            request_count: 0,
            blockers: [],
          },
        ],
        tasks: [
          {
            id: 'task-1',
            channel: 'general',
            title: 'Fix blocked layout',
            owner: 'frontend',
            status: 'blocked',
            blocked: true,
          },
        ],
        flows: [],
        workspaces: [],
        requests: [],
        messages: [],
      },
      blockers: [],
      actions: [],
    })
  })

  await page.route('**/api/operations/bootstrap-package', async (route) => {
    await fulfillJson(route, {
      package: {
        name: 'bootstrap-package',
        description: 'Package',
        workflows: [],
      },
    })
  })
}

test.describe('agent runtime summary', () => {
  test('shows explicit runtime groups and agent labels in the sidebar', async ({ page }) => {
    await installRuntimeSummaryMocks(page)

    await page.goto('/#/channels/general')
    await waitForReactMount(page)

    await expect(page.getByTestId('agent-status-summary')).toBeVisible()
    await expect(page.getByTestId('agent-status-count-working')).toHaveText('1')
    await expect(page.getByTestId('agent-status-count-blocked')).toHaveText('1')
    await expect(page.getByTestId('agent-status-count-waiting')).toHaveText('1')
    await expect(page.getByTestId('agent-status-count-silent')).toHaveText('1')

    await expect(page.getByTestId('sidebar-agent-state-builder')).toBeVisible()
    await expect(page.getByTestId('sidebar-agent-state-frontend')).toBeVisible()
    await expect(page.getByTestId('sidebar-agent-state-ceo')).toBeVisible()
    await expect(page.getByTestId('sidebar-agent-state-pm')).toBeVisible()
  })

  test('shows the same grouped runtime snapshot in Studio', async ({ page }) => {
    await installRuntimeSummaryMocks(page)

    await page.goto('/#/apps/studio')
    await waitForReactMount(page)

    await expect(page.getByTestId('studio-agent-status-card')).toBeVisible()
    await expect(page.getByTestId('studio-agent-status-count-working')).toHaveText('1')
    await expect(page.getByTestId('studio-agent-status-count-blocked')).toHaveText('1')
    await expect(page.getByTestId('studio-agent-status-count-waiting')).toHaveText('1')
    await expect(page.getByTestId('studio-agent-status-count-silent')).toHaveText('1')
  })
})
