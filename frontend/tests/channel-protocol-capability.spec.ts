import { expect, Locator, Page, test } from '@playwright/test'
import { gotoAndEnsureAuth, signInAsAdmin, waitForGraphQLOperation } from './auth.utils'

type GraphQLError = { message: string }
type GraphQLPayload<T> = { data?: T; errors?: GraphQLError[] }
type CreatedChannel = { id: string; name: string; type: string; supportedModels: string[] }
type CreatedModel = { id: string; modelID: string; name: string }
type Association = {
  auto: boolean
  disabled: boolean
  channelModel?: { channelId: number; modelId: string } | null
}
type ProtocolPool = { format: string; associations: Association[] }
type ModelSnapshot = { modelID: string; settings?: { protocolPools: ProtocolPool[] } | null }

const CREATE_CHANNEL = `
  mutation CreateChannel($input: CreateChannelInput!) {
    createChannel(input: $input) {
      id
      name
      type
      supportedModels
    }
  }
`

const CREATE_MODEL = `
  mutation CreateModel($input: CreateModelInput!) {
    createModel(input: $input) {
      id
      modelID
      name
    }
  }
`

const GET_MODEL = `
  query GetModel($first: Int!) {
    models(first: $first) {
      edges {
        node {
          modelID
          settings {
            protocolPools {
              format
              associations {
                auto
                disabled
                channelModel { channelId modelId }
              }
            }
          }
        }
      }
    }
  }
`

async function graphql<T>(page: Page, query: string, variables: Record<string, unknown>): Promise<T> {
  const request = () => page.evaluate(async ({ query, variables }) => {
    const token = localStorage.getItem('axonhub_access_token')
    const response = await fetch('/admin/graphql', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        ...(token ? { Authorization: `Bearer ${token}` } : {}),
      },
      body: JSON.stringify({ query, variables }),
    })
    const payload = (await response.json()) as GraphQLPayload<T>
    if (!response.ok || payload.errors?.length || !payload.data) {
      throw new Error(payload.errors?.map((error) => error.message).join('; ') || `GraphQL HTTP ${response.status}`)
    }
    return payload.data
  }, { query, variables })

  try {
    return await request()
  } catch (error) {
    if (!(error instanceof Error) || !error.message.includes('GraphQL HTTP 401')) throw error
    await page.evaluate(() => localStorage.removeItem('axonhub_access_token'))
    await page.goto('/sign-in', { waitUntil: 'domcontentloaded' })
    await signInAsAdmin(page)
    return request()
  }
}

function uniqueSuffix(): string {
  return `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`
}

async function createModelAndChannel(page: Page, channelType: 'openai' | 'openai_responses' | 'anthropic') {
  await gotoAndEnsureAuth(page, '/channels')
  const suffix = uniqueSuffix()
  const modelID = `e2e-protocol-model-${suffix}`
  const channelName = `e2e-protocol-${channelType}-${suffix}`
  const endpoints = {
    openai: [
      { apiFormat: 'openai/chat_completions', path: '/chat/completions', baseURL: 'https://e2e.invalid/v1', transport: 'http' },
      { apiFormat: 'openai/responses', path: '/responses', baseURL: 'https://e2e.invalid/v1', transport: 'http' },
    ],
    openai_responses: [
      { apiFormat: 'openai/responses', path: '/responses', baseURL: 'https://e2e.invalid/v1', transport: 'http' },
    ],
    anthropic: [
      { apiFormat: 'anthropic/messages', path: '/messages', baseURL: 'https://e2e.invalid/v1', transport: 'http' },
    ],
  }[channelType]

  const model = await graphql<{ createModel: CreatedModel }>(page, CREATE_MODEL, {
    input: {
      developer: 'e2e',
      modelID,
      name: `E2E Protocol Model ${suffix}`,
      icon: 'OpenAI',
      group: 'e2e',
      modelCard: {},
      settings: { protocolPools: [] },
      remark: 'E2E protocol capability test data',
    },
  })
  const channel = await graphql<{ createChannel: CreatedChannel }>(page, CREATE_CHANNEL, {
    input: {
      type: channelType,
      baseURL: 'https://e2e.invalid/v1',
      name: channelName,
      credentials: { apiKey: `e2e-test-key-${suffix}` },
      supportedModels: [modelID],
      manualModels: [],
      autoSyncSupportedModels: false,
      defaultTestModel: modelID,
      endpoints,
    },
  })
  return { model: model.createModel, channel: channel.createChannel }
}

async function openCapabilityDialog(page: Page, channelName: string) {
  await gotoAndEnsureAuth(page, '/channels')
  const table = page.getByTestId('channels-table')
  await expect(table).toBeVisible({ timeout: 20000 })
  const row = table.locator('tbody tr').filter({ hasText: channelName }).first()
  await expect(row).toBeVisible({ timeout: 20000 })
  await row.getByTestId('row-actions').click()
  const capabilityAction = page.getByRole('menuitem', { name: /capabilit|能力|协议/i }).last()
  await expect(capabilityAction).toBeVisible()
  await capabilityAction.click()
  const dialog = page.getByTestId('capability-dialog')
  await expect(dialog).toBeVisible()
  return dialog
}

async function setProtocol(dialog: Locator, protocol: string, checked: boolean) {
  const checkbox = dialog.locator(`[data-testid="capability-protocol-checkbox"][data-protocol="${protocol}"]`)
  const current = (await checkbox.getAttribute('data-state')) === 'checked'
  if (current !== checked) await checkbox.click()
}

async function saveCapabilities(page: Page, dialog: Locator) {
  await Promise.all([
    waitForGraphQLOperation(page, 'SaveChannelCapabilities'),
    dialog.getByTestId('capability-save').click(),
  ])
  await expect(dialog.getByTestId('capability-summary')).toBeVisible({ timeout: 20000 })
}

async function dismissModelsOnboarding(page: Page) {
  const driverOverlay = page.locator('#driver-popover-content')
  if (await driverOverlay.isVisible().catch(() => false)) {
    const settingsButton = page.locator('[data-settings-button]')
    if (await settingsButton.isVisible().catch(() => false)) {
      await settingsButton.click()
      await page.waitForTimeout(500)
    }
    await expect(driverOverlay).not.toBeVisible({ timeout: 5000 }).catch(() => {})
  }
  const settingsDialog = page.getByRole('dialog').filter({ hasText: /Model Settings|模型设置/i })
  if (await settingsDialog.isVisible().catch(() => false)) {
    const settingsButton = page.locator('[data-settings-button]')
    if (await settingsButton.isVisible().catch(() => false)) {
      await settingsButton.evaluate((element) => (element as HTMLElement).click())
      await page.waitForTimeout(500)
    }
    await page.keyboard.press('Escape')
    await expect(settingsDialog).not.toBeVisible({ timeout: 5000 }).catch(() => {})
  }
}

async function fetchModelPools(page: Page, modelID: string): Promise<ProtocolPool[]> {
  const result = await graphql<{ models: { edges: Array<{ node: ModelSnapshot }> } }>(page, GET_MODEL, { first: 100 })
  const model = result.models.edges.map((edge) => edge.node).find((node) => node.modelID === modelID)
  if (!model) throw new Error(`模型 ${modelID} 不存在`)
  return model.settings?.protocolPools ?? []
}

function channelNumericID(channelID: string): number {
  const match = channelID.match(/(?:\/|^)(\d+)$/)
  if (!match) throw new Error(`无法解析渠道 ID: ${channelID}`)
  return Number(match[1])
}

function hasChannelAssociation(pool: ProtocolPool | undefined, channelID: string, modelID: string): boolean {
  const numericID = channelNumericID(channelID)
  return Boolean(pool?.associations.some((association) =>
    association.channelModel?.channelId === numericID && association.channelModel.modelId === modelID
  ))
}

function findChannelAssociation(pool: ProtocolPool | undefined, channelID: string, modelID: string): Association | undefined {
  const numericID = channelNumericID(channelID)
  return pool?.associations.find((association) =>
    association.channelModel?.channelId === numericID && association.channelModel.modelId === modelID
  )
}

async function expectProtocolAssociation(page: Page, modelID: string, channel: CreatedChannel, format: string, expected: boolean) {
  const pools = await fetchModelPools(page, modelID)
  const pool = pools.find((item) => item.format === format)
  expect(hasChannelAssociation(pool, channel.id, modelID)).toBe(expected)
  return findChannelAssociation(pool, channel.id, modelID)
}

test.describe('协议能力弹窗渲染', () => {
  test('渲染 OpenAI Chat Completions、OpenAI Responses、Anthropic Messages 三协议 checkbox', async ({ page }) => {
    test.setTimeout(90000)
    const { channel } = await createModelAndChannel(page, 'openai')
    const dialog = await openCapabilityDialog(page, channel.name)
    const checkboxes = dialog.getByTestId('capability-protocol-checkbox')
    await expect(checkboxes).toHaveCount(3)

    const labels: Record<string, string> = {
      openai: 'OpenAI Chat Completions',
      openai_responses: 'OpenAI Responses',
      anthropic: 'Anthropic Messages',
    }
    for (const [protocol, label] of Object.entries(labels)) {
      const checkbox = dialog.locator(`[data-testid="capability-protocol-checkbox"][data-protocol="${protocol}"]`)
      await expect(checkbox).toHaveCount(1)
      await expect(checkbox.locator('..')).toContainText(label)
    }
  })
})

test.describe('OpenAI Chat Completions 协议池', () => {
  test('派生 openai 池并保留未触及声明后新增 openai_responses 池', async ({ page }) => {
    test.setTimeout(120000)
    const { model, channel } = await createModelAndChannel(page, 'openai')
    const dialog = await openCapabilityDialog(page, channel.name)

    await setProtocol(dialog, 'openai', true)
    await setProtocol(dialog, 'openai_responses', false)
    await setProtocol(dialog, 'anthropic', false)
    await saveCapabilities(page, dialog)
    await expect(dialog.getByTestId('capability-summary')).toContainText(/新增|added/i)
    await expectProtocolAssociation(page, model.modelID, channel, 'openai', true)
    const firstResponseAssociation = await expectProtocolAssociation(page, model.modelID, channel, 'openai', true)
    expect(firstResponseAssociation?.auto).toBe(true)
    expect(firstResponseAssociation?.disabled).toBe(true)

    const responseCheckbox = dialog.locator(
      `[data-testid="capability-model-protocol-checkbox"][data-model="${model.modelID}"][data-protocol="openai_responses"]`
    )
    await expect(responseCheckbox).not.toBeChecked()
    await setProtocol(dialog, 'openai_responses', true)
    await expect(responseCheckbox).toBeChecked()
    await saveCapabilities(page, dialog)

    await expectProtocolAssociation(page, model.modelID, channel, 'openai', true)
    const responsePools = await fetchModelPools(page, model.modelID)
    expect(hasChannelAssociation(responsePools.find((pool) => pool.format === 'openai_responses'), channel.id, model.modelID)).toBe(true)
  })
})

test.describe('OpenAI Responses 协议池', () => {
  test('派生 openai_responses、不跨协议兜底，并支持 bulkEnable', async ({ page }) => {
    test.setTimeout(120000)
    const { model, channel } = await createModelAndChannel(page, 'openai_responses')
    const dialog = await openCapabilityDialog(page, channel.name)

    await setProtocol(dialog, 'openai_responses', true)
    await setProtocol(dialog, 'openai', false)
    await setProtocol(dialog, 'anthropic', false)
    await saveCapabilities(page, dialog)
    await expect(dialog.getByTestId('capability-summary')).toContainText(/新增|added/i)

    const derivedAssociation = await expectProtocolAssociation(page, model.modelID, channel, 'openai_responses', true)
    expect(derivedAssociation?.auto).toBe(true)
    expect(derivedAssociation?.disabled).toBe(true)
    const pools = await fetchModelPools(page, model.modelID)
    expect(hasChannelAssociation(pools.find((pool) => pool.format === 'openai'), channel.id, model.modelID)).toBe(false)

    const bulkEnable = dialog.getByTestId('capability-bulk-enable')
    await expect(bulkEnable).toBeVisible()
    await Promise.all([
      waitForGraphQLOperation(page, 'BulkEnableDerivedAssociations'),
      bulkEnable.click(),
    ])
    await expect(dialog.getByTestId('capability-summary')).toContainText(/成功|success|启用|enable/i)

    await gotoAndEnsureAuth(page, '/models')
    await dismissModelsOnboarding(page)
    const modelRow = page.getByTestId('models-table').locator('tbody tr').filter({ hasText: model.name }).first()
    await expect(modelRow).toBeVisible({ timeout: 20000 })
    await modelRow.getByTestId('row-edit-button').click()
    const editDialog = page.getByRole('dialog').filter({ hasText: /Edit Model|编辑模型|协议池|Protocol Pool/i }).last()
    await expect(editDialog).toBeVisible()
    await expect(editDialog.getByText('openai_responses', { exact: true }).first()).toBeVisible()
  })
})

test.describe('Anthropic Messages 协议池', () => {
  test('派生 anthropic 池且不写入两个 OpenAI 协议池', async ({ page }) => {
    test.setTimeout(120000)
    const { model, channel } = await createModelAndChannel(page, 'anthropic')
    const dialog = await openCapabilityDialog(page, channel.name)

    await setProtocol(dialog, 'anthropic', true)
    await setProtocol(dialog, 'openai', false)
    await setProtocol(dialog, 'openai_responses', false)
    await saveCapabilities(page, dialog)
    await expect(dialog.getByTestId('capability-summary')).toContainText(/新增|added/i)

    const association = await expectProtocolAssociation(page, model.modelID, channel, 'anthropic', true)
    expect(association?.auto).toBe(true)
    expect(association?.disabled).toBe(true)
    const pools = await fetchModelPools(page, model.modelID)
    expect(hasChannelAssociation(pools.find((pool) => pool.format === 'openai'), channel.id, model.modelID)).toBe(false)
    expect(hasChannelAssociation(pools.find((pool) => pool.format === 'openai_responses'), channel.id, model.modelID)).toBe(false)
  })
})
