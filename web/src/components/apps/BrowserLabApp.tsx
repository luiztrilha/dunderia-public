import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { postMessage } from '../../api/client'
import { useChannels } from '../../hooks/useChannels'
import { useAppStore } from '../../stores/app'
import { showNotice } from '../ui/Toast'

interface BrowserLabViewport {
  width: number
  height: number
  label: string
}

interface BrowserLabClick {
  x: number
  y: number
}

interface BrowserLabElement {
  tagName: string
  id?: string
  classes?: string[]
  textContent?: string
  selector: string
  pageUrl: string
  viewport: BrowserLabViewport
  click: BrowserLabClick
  boundingBox?: { x: number; y: number; width: number; height: number }
  attributes?: Record<string, string>
}

interface BrowserLabBounds {
  x: number
  y: number
  width: number
  height: number
}

interface BrowserLabRuntimeEvent {
  url?: string
  title?: string
  loading?: boolean
  ready?: boolean
  canGoBack?: boolean
  canGoForward?: boolean
  error?: string
}

interface BrowserLabDesktopApi {
  setBounds: (bounds: BrowserLabBounds) => Promise<unknown>
  navigate: (url: string) => Promise<unknown>
  back: () => Promise<unknown>
  forward: () => Promise<unknown>
  reload: () => Promise<unknown>
  hide: () => Promise<unknown>
  setInspectMode: (enabled: boolean) => Promise<unknown>
  consumeSelection: () => Promise<BrowserLabElement | null>
  getState: () => Promise<BrowserLabRuntimeEvent>
  onEvent: (callback: (event: BrowserLabRuntimeEvent) => void) => () => void
}

declare global {
  interface Window {
    dunderiaDesktop?: {
      isDesktop?: boolean
      runtime?: string
      browserLab?: BrowserLabDesktopApi
    }
  }
}

const VIEWPORTS: BrowserLabViewport[] = [
  { label: 'Janela', width: 0, height: 0 },
  { label: 'Desktop', width: 1366, height: 768 },
  { label: 'Laptop', width: 1280, height: 800 },
  { label: 'Tablet', width: 820, height: 1180 },
  { label: 'Mobile', width: 390, height: 844 },
]

const DEFAULT_BROWSER_LAB_URL = 'https://www.google.com/'
const INSPECTOR_WIDTH = 384

export function BrowserLabApp() {
  const [url, setUrl] = useState(DEFAULT_BROWSER_LAB_URL)
  const [currentURL, setCurrentURL] = useState(DEFAULT_BROWSER_LAB_URL)
  const [title, setTitle] = useState('')
  const [viewport, setViewport] = useState<BrowserLabViewport>(VIEWPORTS[0])
  const [selected, setSelected] = useState<BrowserLabElement | null>(null)
  const [instruction, setInstruction] = useState('')
  const [inspectMode, setInspectMode] = useState(false)
  const [isLoading, setIsLoading] = useState(false)
  const [canGoBack, setCanGoBack] = useState(false)
  const [canGoForward, setCanGoForward] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [inspectorOpen, setInspectorOpen] = useState(false)
  const [stageBounds, setStageBounds] = useState<BrowserLabBounds>({ x: 0, y: 0, width: 0, height: 0 })
  const stageRef = useRef<HTMLDivElement>(null)
  const didOpenInitialURL = useRef(false)
  const currentChannel = useAppStore((s) => s.currentChannel)
  const setCurrentApp = useAppStore((s) => s.setCurrentApp)
  const queryClient = useQueryClient()
  const { data: channels = [] } = useChannels()
  const [targetChannel, setTargetChannel] = useState(currentChannel)

  const browserLab = window.dunderiaDesktop?.browserLab
  const isDesktop = Boolean(window.dunderiaDesktop?.isDesktop && browserLab)
  const isFluidViewport = viewport.width <= 0 || viewport.height <= 0

  useEffect(() => {
    setTargetChannel(currentChannel)
  }, [currentChannel])

  const status = useMemo(() => {
    if (!isDesktop) return 'Requer desktop'
    if (inspectMode) return 'Modo visão'
    if (isFluidViewport) return 'Janela'
    return `${viewport.width}x${viewport.height}`
  }, [inspectMode, isDesktop, isFluidViewport, viewport.height, viewport.width])

  const applyRuntimeEvent = useCallback((event: BrowserLabRuntimeEvent) => {
    if (typeof event.url === 'string' && event.url) {
      setCurrentURL(event.url)
      setUrl(event.url)
    }
    if (typeof event.title === 'string') setTitle(event.title)
    if (typeof event.loading === 'boolean') setIsLoading(event.loading)
    if (typeof event.canGoBack === 'boolean') setCanGoBack(event.canGoBack)
    if (typeof event.canGoForward === 'boolean') setCanGoForward(event.canGoForward)
    if (event.error) setError(event.error)
  }, [])

  useEffect(() => {
    if (!browserLab) return
    const unsubscribe = browserLab.onEvent(applyRuntimeEvent)
    void browserLab.getState().then(applyRuntimeEvent).catch(() => undefined)
    if (!didOpenInitialURL.current) {
      didOpenInitialURL.current = true
      void browserLab.navigate(DEFAULT_BROWSER_LAB_URL).catch((err) => {
        setError(err instanceof Error ? err.message : 'Falha ao abrir o Chromium embutido.')
      })
    }
    return () => {
      unsubscribe()
      void browserLab.hide().catch(() => undefined)
    }
  }, [applyRuntimeEvent, browserLab])

  useLayoutEffect(() => {
    const stage = stageRef.current
    if (!stage) return

    const measure = () => {
      const rect = stage.getBoundingClientRect()
      setStageBounds({
        x: Math.max(0, Math.round(rect.left)),
        y: Math.max(0, Math.round(rect.top)),
        width: Math.max(0, Math.round(rect.width)),
        height: Math.max(0, Math.round(rect.height)),
      })
    }

    measure()
    const observer = new ResizeObserver(measure)
    observer.observe(stage)
    window.addEventListener('resize', measure)
    window.addEventListener('scroll', measure, true)
    return () => {
      observer.disconnect()
      window.removeEventListener('resize', measure)
      window.removeEventListener('scroll', measure, true)
    }
  }, [])

  useEffect(() => {
    if (!browserLab || stageBounds.width <= 0 || stageBounds.height <= 0) return
    const inspectorReserve = inspectorOpen ? Math.min(INSPECTOR_WIDTH, Math.max(0, stageBounds.width - 280)) : 0
    const nextBounds = {
      x: stageBounds.x,
      y: stageBounds.y,
      width: Math.max(1, stageBounds.width - inspectorReserve),
      height: Math.max(1, stageBounds.height),
    }
    void browserLab.setBounds(nextBounds).catch((err) => {
      setError(err instanceof Error ? err.message : 'Falha ao redimensionar o Chromium embutido.')
    })
  }, [browserLab, inspectorOpen, stageBounds.height, stageBounds.width, stageBounds.x, stageBounds.y])

  const navigate = useCallback(() => {
    if (!browserLab) return
    const nextURL = normalizeBrowserLabURL(url)
    setUrl(nextURL)
    setCurrentURL(nextURL)
    setSelected(null)
    setInstruction('')
    setInspectMode(false)
    setError(null)
    void browserLab.navigate(nextURL).catch((err) => {
      setError(err instanceof Error ? err.message : 'Falha ao navegar no Chromium embutido.')
    })
  }, [browserLab, url])

  useEffect(() => {
    if (!browserLab) return
    void browserLab.setInspectMode(inspectMode).catch((err) => {
      setError(err instanceof Error ? err.message : 'Falha ao ativar modo visão.')
      setInspectMode(false)
    })
    if (!inspectMode) return

    const timer = window.setInterval(() => {
      void browserLab.consumeSelection()
        .then((element) => {
          if (!element) return
          setSelected(element)
          setInstruction('')
          setInspectorOpen(true)
          setInspectMode(false)
          void browserLab.setInspectMode(false).catch(() => undefined)
        })
        .catch(() => undefined)
    }, 250)

    return () => window.clearInterval(timer)
  }, [browserLab, inspectMode])

  const sendVisualContext = useCallback(async () => {
    if (!selected || !instruction.trim()) return
    const channel = targetChannel.trim() || currentChannel
    setError(null)
    try {
      await postMessage(buildBrowserLabPrompt(selected, instruction.trim()), channel)
      await queryClient.invalidateQueries({ queryKey: ['messages', channel] })
      showNotice(`Contexto visual enviado para #${channel}.`, 'success')
      setCurrentApp(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Falha ao enviar contexto visual.')
    }
  }, [currentChannel, instruction, queryClient, selected, setCurrentApp, targetChannel])

  if (!isDesktop) {
    return (
      <div className="browser-lab">
        <div className="browser-lab-header">
          <div>
            <h3>Browser Lab</h3>
            <p>Abra o MaestrIA Desktop para usar o Chromium embutido e apontar elementos da tela.</p>
          </div>
          <div className="browser-lab-status">{status}</div>
        </div>
        <div className="browser-lab-empty browser-lab-empty-panel">
          <span>Este modo usa o BrowserView do Electron. No navegador comum ele fica desativado.</span>
        </div>
      </div>
    )
  }

  return (
    <div className="browser-lab">
      <div className="browser-lab-header">
        <div>
          <h3>Browser Lab</h3>
          <p>Use o Chromium embutido, navegue normalmente e ative a visão para apontar o que quer alterar.</p>
        </div>
        <div className="browser-lab-status">{status}</div>
      </div>

      <div className="browser-lab-toolbar browser-lab-toolbar-desktop">
        <button className="btn btn-sm" disabled={!canGoBack} onClick={() => void browserLab?.back()} title="Voltar">
          ←
        </button>
        <button className="btn btn-sm" disabled={!canGoForward} onClick={() => void browserLab?.forward()} title="Avançar">
          →
        </button>
        <button className="btn btn-sm" onClick={() => void browserLab?.reload()} title="Recarregar">
          ↻
        </button>
        <input
          className="browser-lab-url"
          value={url}
          onChange={(event) => setUrl(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter') navigate()
          }}
          placeholder={DEFAULT_BROWSER_LAB_URL}
          spellCheck={false}
        />
        <select
          className="browser-lab-select"
          value={viewport.label}
          onChange={(event) => {
            const next = VIEWPORTS.find((item) => item.label === event.target.value)
            if (next) setViewport(next)
          }}
        >
          {VIEWPORTS.map((item) => (
            <option key={item.label} value={item.label}>
              {item.width > 0 && item.height > 0 ? `${item.label} - ${item.width}x${item.height}` : item.label}
            </option>
          ))}
        </select>
        <button className="btn btn-primary btn-sm" onClick={navigate}>
          Abrir
        </button>
        <button
          className={`btn btn-sm ${inspectMode ? 'btn-primary' : ''}`}
          onClick={() => setInspectMode((value) => !value)}
        >
          {inspectMode ? 'Visão ativa' : 'Modo visão'}
        </button>
        <button
          className={`btn btn-sm ${inspectorOpen ? 'btn-primary' : ''}`}
          onClick={() => setInspectorOpen((value) => !value)}
        >
          Seleção
        </button>
      </div>

      {error ? <div className="browser-lab-error">{error}</div> : null}

      <div className="browser-lab-workspace browser-lab-workspace-desktop">
        <div
          ref={stageRef}
          className={[
            'browser-lab-stage browser-lab-stage-desktop browser-lab-stage-native',
            isFluidViewport ? 'browser-lab-stage-fluid' : 'browser-lab-stage-fixed',
            inspectMode ? 'browser-lab-stage-inspect' : '',
          ].join(' ')}
        >
          {isLoading ? <div className="browser-lab-busy">Carregando...</div> : null}
        </div>

        <aside className={`browser-lab-inspector${inspectorOpen ? ' browser-lab-inspector-open' : ''}`}>
          <div className="browser-lab-inspector-head">
            <div className="browser-lab-inspector-title">Seleção</div>
            <button className="btn btn-sm" onClick={() => setInspectorOpen(false)} title="Ocultar seleção">
              ×
            </button>
          </div>
          {selected ? (
            <>
              <div className="browser-lab-element">
                <strong>{formatElementName(selected)}</strong>
                <code>{selected.selector}</code>
                {selected.textContent ? <p>{selected.textContent}</p> : null}
                <small>{selected.pageUrl}</small>
              </div>
              <textarea
                value={instruction}
                onChange={(event) => setInstruction(event.target.value)}
                placeholder="O que você quer alterar nesse ponto da tela?"
                rows={5}
              />
              <label className="browser-lab-channel-field">
                <span>Enviar para</span>
                <select
                  className="browser-lab-select"
                  value={targetChannel}
                  onChange={(event) => setTargetChannel(event.target.value)}
                >
                  {channels.length === 0 ? (
                    <option value={currentChannel}>#{currentChannel}</option>
                  ) : channels.map((channel) => (
                    <option key={channel.slug} value={channel.slug}>
                      #{channel.slug}{channel.name && channel.name !== channel.slug ? ` - ${channel.name}` : ''}
                    </option>
                  ))}
                </select>
              </label>
              <button
                className="btn btn-primary btn-sm"
                disabled={!instruction.trim()}
                onClick={() => void sendVisualContext()}
              >
                Enviar para #{targetChannel || currentChannel}
              </button>
            </>
          ) : (
            <p>
              {inspectMode
                ? 'Clique diretamente no Chromium embutido para selecionar um componente.'
                : title
                  ? `${title} aberto. Ative o modo visão quando quiser apontar um elemento.`
                  : 'Navegue no Chromium embutido e ative o modo visão quando quiser apontar um elemento.'}
            </p>
          )}
        </aside>
      </div>
    </div>
  )
}

function normalizeBrowserLabURL(raw: string): string {
  const value = raw.trim()
  if (!value) return DEFAULT_BROWSER_LAB_URL
  if (/^[a-z][a-z\d+.-]*:\/\//i.test(value)) return value
  return `http://${value}`
}

function formatElementName(element: BrowserLabElement): string {
  const id = element.id ? `#${element.id}` : ''
  const classes = element.classes?.length ? `.${element.classes.join('.')}` : ''
  return `<${element.tagName}${id}${classes}>`
}

function buildBrowserLabPrompt(element: BrowserLabElement, instruction: string): string {
  const box = element.boundingBox
    ? `${Math.round(element.boundingBox.width)}x${Math.round(element.boundingBox.height)} em ${Math.round(element.boundingBox.x)},${Math.round(element.boundingBox.y)}`
    : 'nao capturado'
  const attrs = element.attributes && Object.keys(element.attributes).length
    ? Object.entries(element.attributes).map(([key, value]) => `${key}="${value}"`).join(', ')
    : 'sem atributos relevantes'

  return [
    'Contexto visual selecionado no Browser Lab:',
    `- URL: ${element.pageUrl}`,
    `- Elemento: ${formatElementName(element)}`,
    `- Seletor CSS: ${element.selector}`,
    `- Texto visivel: ${element.textContent || '(sem texto)'}`,
    `- Clique: x=${element.click.x}, y=${element.click.y}`,
    `- Viewport: ${element.viewport.width}x${element.viewport.height} (${element.viewport.label})`,
    `- Caixa: ${box}`,
    `- Atributos: ${attrs}`,
    '',
    `Pedido do usuario: ${instruction}`,
    '',
    'Use esse contexto para localizar a parte correspondente da interface/codigo e aplicar a alteracao solicitada.',
  ].join('\n')
}
