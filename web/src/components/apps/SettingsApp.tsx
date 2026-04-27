import { useState, useEffect, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  getConfig,
  updateConfig,
  type ConfigSnapshot,
  type ConfigUpdate,
  type LLMProvider,
} from '../../api/client'
import { usePersistentDraft } from '../../hooks/usePersistentDraft'
import { showNotice } from '../ui/Toast'

type SectionId =
  | 'general'
  | 'company'
  | 'keys'
  | 'integrations'
  | 'intervals'
  | 'flags'
  | 'danger'

interface Section {
  id: SectionId
  icon: string
  navKey: string
}

const SECTIONS: Section[] = [
  { id: 'general', icon: '\u2699', navKey: 'settings.nav.general' },
  { id: 'company', icon: '\uD83C\uDFE2', navKey: 'settings.nav.company' },
  { id: 'keys', icon: '\uD83D\uDD11', navKey: 'settings.nav.keys' },
  { id: 'integrations', icon: '\uD83D\uDD0C', navKey: 'settings.nav.integrations' },
  { id: 'intervals', icon: '\u23F1', navKey: 'settings.nav.intervals' },
  { id: 'flags', icon: '\uD83D\uDDA5', navKey: 'settings.nav.flags' },
  { id: 'danger', icon: '\u26A0\uFE0F', navKey: 'settings.nav.danger' },
]

// ─── Styles ─────────────────────────────────────────────────────────────

const styles = {
  shell: {
    display: 'flex',
    height: '100%',
    minHeight: 0,
    flex: 1,
    overflow: 'hidden',
  } as const,
  nav: {
    width: 200,
    flexShrink: 0,
    borderRight: '1px solid var(--border)',
    padding: '16px 0',
    overflowY: 'auto' as const,
  } as const,
  navItem: (active: boolean) => ({
    display: 'flex',
    alignItems: 'center',
    gap: 8,
    padding: '7px 16px',
    fontSize: 13,
    color: active ? 'var(--accent)' : 'var(--text-secondary)',
    cursor: 'pointer',
    border: 'none',
    background: active ? 'var(--accent-bg)' : 'none',
    width: '100%',
    textAlign: 'left' as const,
    fontFamily: 'var(--font-sans)',
    fontWeight: active ? 600 : 400,
  }),
  body: {
    flex: 1,
    overflowY: 'auto' as const,
    padding: '24px 32px',
    maxWidth: 680,
  } as const,
  sectionTitle: { fontSize: 18, fontWeight: 700, marginBottom: 4 } as const,
  sectionDesc: { fontSize: 13, color: 'var(--text-secondary)', marginBottom: 20, lineHeight: 1.5 } as const,
  banner: {
    display: 'flex',
    gap: 10,
    alignItems: 'flex-start',
    padding: '10px 14px',
    marginBottom: 16,
    background: 'var(--yellow-bg)',
    border: '1px solid var(--yellow)',
    borderRadius: 'var(--radius-md)',
    fontSize: 12,
    lineHeight: 1.5,
    color: 'var(--text)',
  } as const,
  row: { display: 'flex', alignItems: 'flex-start', gap: 12, marginBottom: 14 } as const,
  rowLabel: { width: 160, flexShrink: 0, paddingTop: 10 } as const,
  rowLabelName: { fontSize: 13, fontWeight: 500, color: 'var(--text)' } as const,
  rowLabelHint: { fontSize: 11, color: 'var(--text-tertiary)', marginTop: 2 } as const,
  rowField: { flex: 1, minWidth: 0 } as const,
  input: {
    background: 'var(--bg-card)',
    border: '1px solid var(--border)',
    color: 'var(--text)',
    borderRadius: 'var(--radius-sm)',
    height: 36,
    fontSize: 13,
    padding: '0 10px',
    outline: 'none',
    width: '100%',
    fontFamily: 'var(--font-sans)',
  } as const,
  textarea: {
    background: 'var(--bg-card)',
    border: '1px solid var(--border)',
    color: 'var(--text)',
    borderRadius: 'var(--radius-sm)',
    minHeight: 60,
    fontSize: 13,
    padding: '8px 10px',
    outline: 'none',
    width: '100%',
    fontFamily: 'var(--font-sans)',
    lineHeight: 1.5,
    resize: 'vertical' as const,
  },
  keyStatus: (set: boolean) => ({
    display: 'inline-flex',
    alignItems: 'center',
    fontSize: 11,
    fontWeight: 500,
    padding: '2px 8px',
    borderRadius: 'var(--radius-full)',
    whiteSpace: 'nowrap' as const,
    background: set ? 'var(--green-bg)' : 'var(--bg-warm)',
    color: set ? 'var(--green)' : 'var(--text-tertiary)',
  }),
  saveRow: { display: 'flex', gap: 8, marginTop: 20, paddingTop: 16, borderTop: '1px solid var(--border-light)' } as const,
  filePath: {
    fontFamily: 'var(--font-mono)',
    fontSize: 11,
    color: 'var(--text-tertiary)',
    padding: '6px 10px',
    background: 'var(--bg-warm)',
    borderRadius: 'var(--radius-sm)',
    border: '1px solid var(--border-light)',
    userSelect: 'all' as const,
    wordBreak: 'break-all' as const,
  },
  table: { width: '100%', borderCollapse: 'collapse' as const, fontSize: 12 } as const,
  th: {
    textAlign: 'left' as const,
    fontWeight: 600,
    fontSize: 11,
    textTransform: 'uppercase' as const,
    letterSpacing: '0.06em',
    color: 'var(--text-tertiary)',
    padding: '6px 8px',
    borderBottom: '1px solid var(--border)',
  } as const,
  td: { padding: '6px 8px', borderBottom: '1px solid var(--border-light)', verticalAlign: 'top' as const } as const,
  tdFlag: {
    padding: '6px 8px',
    borderBottom: '1px solid var(--border-light)',
    verticalAlign: 'top' as const,
    fontFamily: 'var(--font-mono)',
    color: 'var(--accent)',
    whiteSpace: 'nowrap' as const,
  } as const,
  tdDesc: {
    padding: '6px 8px',
    borderBottom: '1px solid var(--border-light)',
    verticalAlign: 'top' as const,
    color: 'var(--text-secondary)',
  } as const,
  groupTitle: {
    fontSize: 11,
    fontWeight: 600,
    textTransform: 'uppercase' as const,
    letterSpacing: '0.06em',
    color: 'var(--text-tertiary)',
    marginBottom: 10,
    paddingBottom: 6,
    borderBottom: '1px solid var(--border-light)',
  } as const,
}

// ─── Small components ───────────────────────────────────────────────────

interface FieldProps {
  label: string
  hint?: string
  children: ReactNode
}

function Field({ label, hint, children }: FieldProps) {
  return (
    <div style={styles.row}>
      <div style={styles.rowLabel}>
        <div style={styles.rowLabelName}>{label}</div>
        {hint && <div style={styles.rowLabelHint}>{hint}</div>}
      </div>
      <div style={styles.rowField}>{children}</div>
    </div>
  )
}

interface SaveButtonProps {
  label: string
  onSave: () => Promise<void> | void
}

function SaveButton({ label, onSave }: SaveButtonProps) {
  const { t } = useTranslation()
  const [state, setState] = useState<'idle' | 'saving' | 'saved'>('idle')

  const handle = async () => {
    if (state === 'saving') return
    setState('saving')
    try {
      await onSave()
      setState('saved')
      setTimeout(() => setState('idle'), 1500)
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err)
      showNotice(t('settings.status.saveFailedWithReason', { error: msg }), 'error')
      setState('idle')
    }
  }

  return (
    <div style={styles.saveRow}>
      <button
        className="btn btn-primary btn-sm"
        onClick={handle}
        disabled={state === 'saving'}
      >
        {state === 'saving' ? t('settings.saveButton.saving') : state === 'saved' ? t('settings.saveButton.saved') : label}
      </button>
    </div>
  )
}

interface KeyFieldProps {
  hasValue: boolean
  placeholder: string
  value: string
  onChange: (v: string) => void
}

function KeyField({ hasValue, placeholder, value, onChange }: KeyFieldProps) {
  const { t } = useTranslation()
  return (
    <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
      <input
        type="password"
        className="input"
        style={{ ...styles.input, flex: 1, fontFamily: 'var(--font-mono)', fontSize: 12 }}
        placeholder={hasValue ? '\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022 (set)' : placeholder}
        value={value}
        onChange={(e) => onChange(e.target.value)}
      />
      <span style={styles.keyStatus(hasValue)}>{hasValue ? t('settings.keys.statusSet') : t('settings.keys.statusUnset')}</span>
    </div>
  )
}

// ─── Section components ─────────────────────────────────────────────────

interface SectionProps {
  cfg: ConfigSnapshot
  save: (patch: ConfigUpdate) => Promise<void>
}

function GeneralSection({ cfg, save }: SectionProps) {
  const { t } = useTranslation()
  const draft = usePersistentDraft(settingsDraftKey('general'), {
    provider: (cfg.llm_provider ?? 'claude-code') as LLMProvider,
    memory: 'none',
    teamLead: cfg.team_lead_slug ?? '',
    maxConcurrent: cfg.max_concurrent_agents ? String(cfg.max_concurrent_agents) : '',
    format: cfg.default_format ?? 'text',
    timeout: cfg.default_timeout ? String(cfg.default_timeout) : '',
    blueprint: cfg.blueprint ?? '',
    email: cfg.email ?? '',
    devUrl: cfg.dev_url ?? '',
  })
  const { value, setValue, clearStorage } = draft

  const onSave = async () => {
    const patch: ConfigUpdate = {
      llm_provider: value.provider as ConfigUpdate['llm_provider'],
      memory_backend: 'none',
      default_format: value.format,
      blueprint: value.blueprint,
      email: value.email,
      dev_url: value.devUrl,
      team_lead_slug: value.teamLead,
    }
    if (value.maxConcurrent) patch.max_concurrent_agents = parseInt(value.maxConcurrent, 10)
    if (value.timeout) patch.default_timeout = parseInt(value.timeout, 10)
    await save(patch)
    clearStorage()
  }

  return (
    <div>
      <h2 style={styles.sectionTitle}>{t('settings.general.title')}</h2>
      <p style={styles.sectionDesc}>{t('settings.general.description')}</p>

      <div style={styles.banner}>
        <span style={{ fontSize: 14, flexShrink: 0 }}>{'\u26A0'}</span>
        <div>
          <strong>{t('settings.general.banner.restartRequired')} </strong>
          {t('settings.general.banner.afterSave')}{' '}
          <code style={{ fontFamily: 'var(--font-mono)', padding: '1px 4px', background: 'var(--bg-warm)', borderRadius: 3 }}>
            wuphf kill
          </code>{' '}
          {t('settings.general.banner.afterCmd')}
        </div>
      </div>

      <Field label={t('settings.general.fields.provider.label')} hint={t('settings.general.fields.provider.hint')}>
        <select
          style={styles.input}
          value={value.provider}
          onChange={(e) => setValue((prev) => ({ ...prev, provider: e.target.value as LLMProvider }))}
        >
          <option value="claude-code">{t('settings.general.options.provider.claudeCode')}</option>
          <option value="codex">{t('settings.general.options.provider.codex')}</option>
          <option value="gemini">{t('settings.general.options.provider.gemini')}</option>
          <option value="ollama">{t('settings.general.options.provider.ollama')}</option>
          <option value="gemini-vertex">{t('settings.general.options.provider.geminiVertex')}</option>
        </select>
        <div style={{ ...styles.rowLabelHint, marginTop: 6 }}>
          {t('settings.general.fields.provider.desc')}
        </div>
      </Field>

      <Field label={t('settings.general.fields.memory.label')} hint={t('settings.general.fields.memory.hint')}>
        <div style={{ ...styles.input, display: 'flex', alignItems: 'center', color: 'var(--text-primary)' }}>
          {t('settings.general.options.memory.none')}
        </div>
      </Field>

      <Field label={t('settings.general.fields.teamLead.label')} hint={t('settings.general.fields.teamLead.hint')}>
        <input
          style={styles.input}
          placeholder={t('settings.general.fields.teamLead.placeholder')}
          value={value.teamLead}
          onChange={(e) => setValue((prev) => ({ ...prev, teamLead: e.target.value }))}
        />
      </Field>

      <Field label={t('settings.general.fields.maxConcurrent.label')} hint={t('settings.general.fields.maxConcurrent.hint')}>
        <input
          style={styles.input}
          type="number"
          min={1}
          placeholder={t('settings.general.fields.maxConcurrent.placeholder')}
          value={value.maxConcurrent}
          onChange={(e) => setValue((prev) => ({ ...prev, maxConcurrent: e.target.value }))}
        />
      </Field>

      <Field label={t('settings.general.fields.format.label')} hint={t('settings.general.fields.format.hint')}>
        <select
          style={styles.input}
          value={value.format}
          onChange={(e) => setValue((prev) => ({ ...prev, format: e.target.value }))}
        >
          <option value="text">{t('settings.general.options.format.text')}</option>
          <option value="json">{t('settings.general.options.format.json')}</option>
        </select>
      </Field>

      <Field label={t('settings.general.fields.timeout.label')} hint={t('settings.general.fields.timeout.hint')}>
        <input
          style={styles.input}
          type="number"
          min={1000}
          placeholder="120000"
          value={value.timeout}
          onChange={(e) => setValue((prev) => ({ ...prev, timeout: e.target.value }))}
        />
      </Field>

      <Field label={t('settings.general.fields.blueprint.label')} hint={t('settings.general.fields.blueprint.hint')}>
        <input
          style={styles.input}
          placeholder={t('settings.general.fields.blueprint.placeholder')}
          value={value.blueprint}
          onChange={(e) => setValue((prev) => ({ ...prev, blueprint: e.target.value }))}
        />
      </Field>

      <Field label={t('settings.general.fields.email.label')} hint={t('settings.general.fields.email.hint')}>
        <input
          style={styles.input}
          type="email"
          placeholder={t('settings.general.fields.email.placeholder')}
          value={value.email}
          onChange={(e) => setValue((prev) => ({ ...prev, email: e.target.value }))}
        />
      </Field>

      <Field label={t('settings.general.fields.devUrl.label')} hint={t('settings.general.fields.devUrl.hint')}>
        <input
          style={styles.input}
          placeholder={t('settings.general.fields.devUrl.placeholder')}
          value={value.devUrl}
          onChange={(e) => setValue((prev) => ({ ...prev, devUrl: e.target.value }))}
        />
      </Field>

      <SaveButton label={t('settings.general.saveLabel')} onSave={onSave} />

      {cfg.config_path && (
        <div style={{ marginTop: 24 }}>
          <div style={styles.groupTitle}>{t('settings.general.configFile')}</div>
          <div style={styles.filePath}>{cfg.config_path}</div>
        </div>
      )}
    </div>
  )
}

function CompanySection({ cfg, save }: SectionProps) {
  const { t } = useTranslation()
  const draft = usePersistentDraft(settingsDraftKey('company'), {
    name: cfg.company_name ?? '',
    description: cfg.company_description ?? '',
    goals: cfg.company_goals ?? '',
    size: cfg.company_size ?? '',
    priority: cfg.company_priority ?? '',
  })
  const { value, setValue, clearStorage } = draft

  const onSave = async () => {
    await save({
      company_name: value.name,
      company_description: value.description,
      company_goals: value.goals,
      company_size: value.size,
      company_priority: value.priority,
    })
    clearStorage()
  }

  return (
    <div>
      <h2 style={styles.sectionTitle}>{t('settings.company.title')}</h2>
      <p style={styles.sectionDesc}>{t('settings.company.description')}</p>

      <Field label={t('settings.company.fields.name.label')} hint={t('settings.company.fields.name.hint')}>
        <input
          style={styles.input}
          placeholder={t('settings.company.fields.name.placeholder')}
          value={value.name}
          onChange={(e) => setValue((prev) => ({ ...prev, name: e.target.value }))}
        />
      </Field>

      <Field label={t('settings.company.fields.description.label')} hint={t('settings.company.fields.description.hint')}>
        <textarea
          style={styles.textarea}
          placeholder={t('settings.company.fields.description.placeholder')}
          value={value.description}
          onChange={(e) => setValue((prev) => ({ ...prev, description: e.target.value }))}
        />
      </Field>

      <Field label={t('settings.company.fields.goals.label')} hint={t('settings.company.fields.goals.hint')}>
        <textarea
          style={styles.textarea}
          placeholder={t('settings.company.fields.goals.placeholder')}
          value={value.goals}
          onChange={(e) => setValue((prev) => ({ ...prev, goals: e.target.value }))}
        />
      </Field>

      <Field label={t('settings.company.fields.size.label')} hint={t('settings.company.fields.size.hint')}>
        <input
          style={styles.input}
          placeholder={t('settings.company.fields.size.placeholder')}
          value={value.size}
          onChange={(e) => setValue((prev) => ({ ...prev, size: e.target.value }))}
        />
      </Field>

      <Field label={t('settings.company.fields.priority.label')} hint={t('settings.company.fields.priority.hint')}>
        <textarea
          style={styles.textarea}
          placeholder={t('settings.company.fields.priority.placeholder')}
          value={value.priority}
          onChange={(e) => setValue((prev) => ({ ...prev, priority: e.target.value }))}
        />
      </Field>

      <SaveButton label={t('settings.company.saveLabel')} onSave={onSave} />
    </div>
  )
}

interface KeyDef {
  field: keyof ConfigUpdate
  flag: keyof ConfigSnapshot
  labelKey: string
  placeholder: string
  env: string
}

const KEY_DEFS: KeyDef[] = [
  { field: 'api_key', flag: 'api_key_set', labelKey: 'settings.keys.labels.nex', placeholder: 'api_...', env: 'WUPHF_API_KEY' },
  { field: 'anthropic_api_key', flag: 'anthropic_key_set', labelKey: 'settings.keys.labels.anthropic', placeholder: 'sk-ant-...', env: 'ANTHROPIC_API_KEY' },
  { field: 'openai_api_key', flag: 'openai_key_set', labelKey: 'settings.keys.labels.openai', placeholder: 'sk-...', env: 'OPENAI_API_KEY' },
  { field: 'gemini_api_key', flag: 'gemini_key_set', labelKey: 'settings.keys.labels.gemini', placeholder: 'AI...', env: 'GEMINI_API_KEY' },
  { field: 'minimax_api_key', flag: 'minimax_key_set', labelKey: 'settings.keys.labels.minimax', placeholder: 'mm-...', env: 'MINIMAX_API_KEY' },
  { field: 'brave_api_key', flag: 'brave_key_set', labelKey: 'settings.keys.labels.brave', placeholder: 'BSA...', env: 'BRAVE_API_KEY' },
  { field: 'one_api_key', flag: 'one_key_set', labelKey: 'settings.keys.labels.one', placeholder: 'one_...', env: 'ONE_SECRET' },
  { field: 'composio_api_key', flag: 'composio_key_set', labelKey: 'settings.keys.labels.composio', placeholder: 'cmp_...', env: 'COMPOSIO_API_KEY' },
  { field: 'telegram_bot_token', flag: 'telegram_token_set', labelKey: 'settings.keys.labels.telegram', placeholder: '123456:ABC...', env: 'WUPHF_TELEGRAM_BOT_TOKEN' },
]

function settingsDraftKey(section: Extract<SectionId, 'general' | 'company' | 'keys' | 'integrations' | 'intervals'>): string {
  return `settings-draft:${section}`
}

function KeysSection({ cfg, save }: SectionProps) {
  const { t } = useTranslation()
  const draft = usePersistentDraft<Record<string, string>>(settingsDraftKey('keys'), {}, {
    persist: false,
  })
  const { value: values, setValue: setValues, clearStorage } = draft

  useEffect(() => {
    clearStorage()
  }, [clearStorage])

  const onSave = async () => {
    const entries = Object.entries(values).filter(([, v]) => v.trim() !== '')
    if (entries.length === 0) {
      showNotice(t('settings.keys.emptySubmit'), 'info')
      throw new Error('no_keys_entered')
    }
    const patch: ConfigUpdate = {}
    for (const [k, v] of entries) {
      ;(patch as Record<string, string>)[k] = v
    }
    await save(patch)
    setValues({})
    clearStorage()
  }

  return (
    <div>
      <h2 style={styles.sectionTitle}>{t('settings.keys.title')}</h2>
      <p style={styles.sectionDesc}>{t('settings.keys.description')}</p>

      {KEY_DEFS.map((def) => (
        <Field key={def.field} label={t(def.labelKey)} hint={t('settings.keys.envHint', { env: def.env })}>
          <KeyField
            hasValue={Boolean(cfg[def.flag])}
            placeholder={def.placeholder}
            value={values[def.field] ?? ''}
            onChange={(v) => setValues((prev) => ({ ...prev, [def.field]: v }))}
          />
        </Field>
      ))}

      <SaveButton label={t('settings.keys.saveLabel')} onSave={onSave} />
    </div>
  )
}

function IntegrationsSection({ cfg, save }: SectionProps) {
  const { t } = useTranslation()
  const draft = usePersistentDraft(settingsDraftKey('integrations'), {
    actionProvider: cfg.action_provider ?? 'auto',
    webSearchProvider: cfg.web_search_provider ?? 'none',
    customMCPConfigPath: cfg.custom_mcp_config_path ?? '',
    cloudBackupProvider: cfg.cloud_backup_provider ?? 'none',
    cloudBackupBucket: cfg.cloud_backup_bucket ?? '',
    cloudBackupPrefix: cfg.cloud_backup_prefix ?? '',
  })
  const { value, setValue, clearStorage } = draft

  const onSave = async () => {
    const patch: ConfigUpdate = {
      action_provider: value.actionProvider as ConfigUpdate['action_provider'],
      web_search_provider: value.webSearchProvider as ConfigUpdate['web_search_provider'],
      custom_mcp_config_path: value.customMCPConfigPath.trim(),
      cloud_backup_provider: value.cloudBackupProvider,
      cloud_backup_bucket: value.cloudBackupBucket.trim(),
      cloud_backup_prefix: value.cloudBackupPrefix.trim(),
    }
    await save(patch)
    clearStorage()
  }

  return (
    <div>
      <h2 style={styles.sectionTitle}>{t('settings.integrations.title')}</h2>
      <p style={styles.sectionDesc}>{t('settings.integrations.description')}</p>

      <Field label={t('settings.integrations.fields.actionProvider.label')} hint={t('settings.integrations.fields.actionProvider.hint')}>
        <select
          style={styles.input}
          value={value.actionProvider}
          onChange={(e) => setValue((prev) => ({ ...prev, actionProvider: e.target.value as typeof prev.actionProvider }))}
        >
          <option value="auto">{t('settings.integrations.options.actionProvider.auto')}</option>
          <option value="one">{t('settings.integrations.options.actionProvider.one')}</option>
          <option value="composio">{t('settings.integrations.options.actionProvider.composio')}</option>
        </select>
      </Field>

      <Field label={t('settings.integrations.fields.webSearchProvider.label')} hint={t('settings.integrations.fields.webSearchProvider.hint')}>
        <select
          style={styles.input}
          value={value.webSearchProvider}
          onChange={(e) => setValue((prev) => ({ ...prev, webSearchProvider: e.target.value as typeof prev.webSearchProvider }))}
        >
          <option value="none">{t('settings.integrations.options.webSearchProvider.none')}</option>
          <option value="brave">{t('settings.integrations.options.webSearchProvider.brave')}</option>
        </select>
      </Field>

      <Field label={t('settings.integrations.fields.customMCPConfig.label')} hint={t('settings.integrations.fields.customMCPConfig.hint')}>
        <input
          style={styles.input}
          placeholder={t('settings.integrations.placeholders.customMCPConfig')}
          value={value.customMCPConfigPath}
          onChange={(e) => setValue((prev) => ({ ...prev, customMCPConfigPath: e.target.value }))}
        />
      </Field>

      <div style={{ marginTop: 20 }}>
        <div style={styles.groupTitle}>{t('settings.integrations.groups.cloudBackup')}</div>
        <Field
          label={t('settings.integrations.fields.cloudBackupProvider.label')}
          hint={t('settings.integrations.fields.cloudBackupProvider.hint')}
        >
          <select
            style={styles.input}
            value={value.cloudBackupProvider}
            onChange={(e) => setValue((prev) => ({ ...prev, cloudBackupProvider: e.target.value as typeof prev.cloudBackupProvider }))}
          >
            <option value="none">{t('settings.integrations.options.cloudBackupProvider.none')}</option>
            <option value="gcs">{t('settings.integrations.options.cloudBackupProvider.gcs')}</option>
          </select>
        </Field>

        <Field label={t('settings.integrations.fields.cloudBackupBucket.label')} hint={t('settings.integrations.fields.cloudBackupBucket.hint')}>
          <input
            style={styles.input}
            placeholder={t('settings.integrations.placeholders.cloudBackupBucket')}
            value={value.cloudBackupBucket}
            onChange={(e) => setValue((prev) => ({ ...prev, cloudBackupBucket: e.target.value }))}
          />
        </Field>

        <Field label={t('settings.integrations.fields.cloudBackupPrefix.label')} hint={t('settings.integrations.fields.cloudBackupPrefix.hint')}>
          <input
            style={styles.input}
            placeholder={t('settings.integrations.placeholders.cloudBackupPrefix')}
            value={value.cloudBackupPrefix}
            onChange={(e) => setValue((prev) => ({ ...prev, cloudBackupPrefix: e.target.value }))}
          />
        </Field>
      </div>

      <div style={{ marginTop: 20 }}>
        <div style={styles.groupTitle}>{t('settings.integrations.groups.workspace')}</div>
        <Field label={t('settings.integrations.fields.workspaceId.label')} hint={t('settings.integrations.fields.workspaceId.hint')}>
          <input
            style={{ ...styles.input, opacity: 0.6, cursor: 'default' }}
            readOnly
            placeholder={t('settings.integrations.placeholders.nexRegistration')}
            value={cfg.workspace_id ?? ''}
          />
        </Field>
        <Field label={t('settings.integrations.fields.workspaceSlug.label')} hint={t('settings.integrations.fields.workspaceSlug.hint')}>
          <input
            style={{ ...styles.input, opacity: 0.6, cursor: 'default' }}
            readOnly
            placeholder={t('settings.integrations.placeholders.nexRegistration')}
            value={cfg.workspace_slug ?? ''}
          />
        </Field>
      </div>

      <SaveButton label={t('settings.integrations.saveLabel')} onSave={onSave} />
    </div>
  )
}

function IntervalsSection({ cfg, save }: SectionProps) {
  const { t } = useTranslation()
  const draft = usePersistentDraft(settingsDraftKey('intervals'), {
    insights: String(cfg.insights_poll_minutes ?? 15),
    followUp: String(cfg.task_follow_up_minutes ?? 60),
    reminder: String(cfg.task_reminder_minutes ?? 30),
    recheck: String(cfg.task_recheck_minutes ?? 15),
  })
  const { value, setValue, clearStorage } = draft

  const onSave = async () => {
    await save({
      insights_poll_minutes: parseInt(value.insights, 10) || 15,
      task_follow_up_minutes: parseInt(value.followUp, 10) || 60,
      task_reminder_minutes: parseInt(value.reminder, 10) || 30,
      task_recheck_minutes: parseInt(value.recheck, 10) || 15,
    })
    clearStorage()
  }

  return (
    <div>
      <h2 style={styles.sectionTitle}>{t('settings.intervals.title')}</h2>
      <p style={styles.sectionDesc}>{t('settings.intervals.description')}</p>

      <Field label={t('settings.intervals.fields.insights.label')} hint={t('settings.intervals.fields.insights.hint')}>
        <input
          style={styles.input}
          type="number"
          min={2}
          placeholder="15"
          value={value.insights}
          onChange={(e) => setValue((prev) => ({ ...prev, insights: e.target.value }))}
        />
      </Field>
      <Field label={t('settings.intervals.fields.followUp.label')} hint={t('settings.intervals.fields.followUp.hint')}>
        <input
          style={styles.input}
          type="number"
          min={2}
          placeholder="60"
          value={value.followUp}
          onChange={(e) => setValue((prev) => ({ ...prev, followUp: e.target.value }))}
        />
      </Field>
      <Field label={t('settings.intervals.fields.reminder.label')} hint={t('settings.intervals.fields.reminder.hint')}>
        <input
          style={styles.input}
          type="number"
          min={2}
          placeholder="30"
          value={value.reminder}
          onChange={(e) => setValue((prev) => ({ ...prev, reminder: e.target.value }))}
        />
      </Field>
      <Field label={t('settings.intervals.fields.recheck.label')} hint={t('settings.intervals.fields.recheck.hint')}>
        <input
          style={styles.input}
          type="number"
          min={2}
          placeholder="15"
          value={value.recheck}
          onChange={(e) => setValue((prev) => ({ ...prev, recheck: e.target.value }))}
        />
      </Field>

      <SaveButton label={t('settings.intervals.saveLabel')} onSave={onSave} />
    </div>
  )
}

const CLI_FLAGS: [string, string][] = [
  ['--provider <name>', 'settings.flags.cli.provider'],
  ['--memory-backend <name>', 'settings.flags.cli.memoryBackend'],
  ['--blueprint <id>', 'settings.flags.cli.blueprint'],
  ['--tui', 'settings.flags.cli.tui'],
  ['--web-port <port>', 'settings.flags.cli.webPort'],
  ['--broker-port <port>', 'settings.flags.cli.brokerPort'],
  ['--opus-ceo', 'settings.flags.cli.opusCeo'],
  ['--collab', 'settings.flags.cli.collab'],
  ['--1o1', 'settings.flags.cli.oneOnOne'],
  ['--unsafe', 'settings.flags.cli.unsafe'],
  ['--no-open', 'settings.flags.cli.noOpen'],
  ['--from-scratch', 'settings.flags.cli.fromScratch'],
  ['--threads-collapsed', 'settings.flags.cli.threadsCollapsed'],
  ['--cmd <command>', 'settings.flags.cli.cmd'],
  ['--format <fmt>', 'settings.flags.cli.format'],
  ['--version', 'settings.flags.cli.version'],
  ['--help-all', 'settings.flags.cli.helpAll'],
]

const ENV_VARS: [string, string][] = [
  ['WUPHF_LLM_PROVIDER', 'settings.flags.env.llmProvider'],
  ['WUPHF_MEMORY_BACKEND', 'settings.flags.env.memoryBackend'],
  ['WUPHF_BROKER_PORT', 'settings.flags.env.brokerPort'],
  ['WUPHF_CONFIG_PATH', 'settings.flags.env.configPath'],
  ['WUPHF_RUNTIME_HOME', 'settings.flags.env.runtimeHome'],
  ['WUPHF_START_FROM_SCRATCH', 'settings.flags.env.startFromScratch'],
  ['WUPHF_ONE_ON_ONE', 'settings.flags.env.oneOnOne'],
  ['WUPHF_HEADLESS_PROVIDER', 'settings.flags.env.headlessProvider'],
  ['WUPHF_INSIGHTS_INTERVAL_MINUTES', 'settings.flags.env.insightsInterval'],
  ['WUPHF_TASK_FOLLOWUP_MINUTES', 'settings.flags.env.taskFollowup'],
  ['WUPHF_TASK_REMINDER_MINUTES', 'settings.flags.env.taskReminder'],
  ['WUPHF_TASK_RECHECK_MINUTES', 'settings.flags.env.taskRecheck'],
]

function FlagsSection() {
  const { t } = useTranslation()
  return (
    <div>
      <h2 style={styles.sectionTitle}>{t('settings.flags.title')}</h2>
      <p style={styles.sectionDesc}>{t('settings.flags.description')}</p>

      <table style={styles.table}>
        <thead>
          <tr>
            <th style={styles.th}>{t('settings.flags.table.flag')}</th>
            <th style={styles.th}>{t('settings.flags.table.description')}</th>
          </tr>
        </thead>
        <tbody>
          {CLI_FLAGS.map(([flag, descKey]) => (
            <tr key={flag}>
              <td style={styles.tdFlag}>{flag}</td>
              <td style={styles.tdDesc}>{t(descKey)}</td>
            </tr>
          ))}
        </tbody>
      </table>

      <div style={{ marginTop: 24 }}>
        <div style={styles.groupTitle}>{t('settings.flags.envVars.title')}</div>
        <p style={{ fontSize: 12, color: 'var(--text-secondary)', lineHeight: 1.6, marginBottom: 12 }}>
          {t('settings.flags.envVars.hint')}
        </p>
        <table style={styles.table}>
          <thead>
            <tr>
              <th style={styles.th}>{t('settings.flags.table.variable')}</th>
              <th style={styles.th}>{t('settings.flags.table.purpose')}</th>
            </tr>
          </thead>
          <tbody>
            {ENV_VARS.map(([v, purposeKey]) => (
              <tr key={v}>
                <td style={styles.tdFlag}>{v}</td>
                <td style={styles.tdDesc}>{t(purposeKey)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

// ─── Data Safety ────────────────────────────────────────────────────────

function DangerZoneSection() {
  const { t } = useTranslation()

  return (
    <div>
      <div style={styles.sectionTitle}>{t('settings.danger.title')}</div>
      <div style={styles.sectionDesc}>{t('settings.danger.description')}</div>
      <div style={styles.banner}>
        <span aria-hidden="true">✓</span>
        <div>{t('settings.danger.disabled')}</div>
      </div>
    </div>
  )
}

// ─── Main component ─────────────────────────────────────────────────────

export function SettingsApp() {
  const { t } = useTranslation()
  const [section, setSection] = useState<SectionId>('general')
  const queryClient = useQueryClient()

  const { data, isLoading, error } = useQuery({
    queryKey: ['config'],
    queryFn: getConfig,
    staleTime: 0,
    refetchOnMount: 'always',
    refetchOnWindowFocus: true,
    refetchOnReconnect: true,
    refetchInterval:
      section === 'keys' || section === 'integrations' ? 5_000 : false,
  })

  const saveMutation = useMutation({
    mutationFn: (patch: ConfigUpdate) => updateConfig(patch),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['config'] })
      showNotice(t('settings.status.saved'), 'success')
    },
    onError: (err: unknown) => {
      const message = err instanceof Error ? err.message : t('settings.status.saveFailed')
      showNotice(message, 'error')
    },
  })

  const save = async (patch: ConfigUpdate) => {
    await saveMutation.mutateAsync(patch)
  }

  if (isLoading) {
    return (
      <div style={{ padding: 40, textAlign: 'center', color: 'var(--text-tertiary)', fontSize: 14 }}>
        {t('settings.status.loading')}
      </div>
    )
  }

  if (error || !data) {
    return (
      <div style={{ padding: 40, textAlign: 'center', color: 'var(--text-tertiary)', fontSize: 14 }}>
        {t('settings.status.loadFailed', { error: error instanceof Error ? error.message : String(error) })}
      </div>
    )
  }

  return (
    <div style={styles.shell}>
      <nav style={styles.nav}>
        {SECTIONS.map((sec) => (
          <button key={sec.id} style={styles.navItem(sec.id === section)} onClick={() => setSection(sec.id)}>
            <span style={{ width: 16, textAlign: 'center', flexShrink: 0 }}>{sec.icon}</span>
            <span>{t(sec.navKey)}</span>
          </button>
        ))}
      </nav>
      <div style={styles.body}>
        {section === 'general' && <GeneralSection cfg={data} save={save} />}
        {section === 'company' && <CompanySection cfg={data} save={save} />}
        {section === 'keys' && <KeysSection cfg={data} save={save} />}
        {section === 'integrations' && <IntegrationsSection cfg={data} save={save} />}
        {section === 'intervals' && <IntervalsSection cfg={data} save={save} />}
        {section === 'flags' && <FlagsSection />}
        {section === 'danger' && <DangerZoneSection />}
      </div>
    </div>
  )
}
