const { app, BrowserWindow, Menu, Tray, ipcMain, nativeImage, shell } = require('electron')
const { spawn } = require('node:child_process')
const http = require('node:http')
const path = require('node:path')

const repoRoot = path.resolve(__dirname, '..')
const defaultWebURL = 'http://127.0.0.1:7891'
const webURL = process.env.MAESTRIA_WEB_URL || process.env.DUNDERIA_WEB_URL || defaultWebURL
let brokerProcess
let mainWindow
let browserLabWindow
let tray
const browserLabState = {
  url: 'https://www.google.com/',
  title: '',
  loading: false,
  ready: false,
  bounds: { x: 0, y: 0, width: 0, height: 0 },
}

const browserLabInspectScript = String.raw`
(() => {
  if (window.__dunderiaBrowserLab) return;

  const cssEscape = (value) => {
    if (window.CSS && typeof window.CSS.escape === 'function') return window.CSS.escape(value);
    return String(value).replace(/[^a-zA-Z0-9_-]/g, '\\$&');
  };

  const selectorFor = (element) => {
    if (!element || element.nodeType !== Node.ELEMENT_NODE) return '';
    if (element.id) return '#' + cssEscape(element.id);
    const segments = [];
    let current = element;
    while (current && current.nodeType === Node.ELEMENT_NODE && segments.length < 5) {
      let segment = current.localName || current.tagName.toLowerCase();
      const classes = Array.from(current.classList || []).slice(0, 3);
      if (classes.length) segment += '.' + classes.map(cssEscape).join('.');
      const parent = current.parentElement;
      if (parent) {
        const siblings = Array.from(parent.children).filter((item) => item.localName === current.localName);
        if (siblings.length > 1) segment += ':nth-of-type(' + (siblings.indexOf(current) + 1) + ')';
      }
      segments.unshift(segment);
      current = parent;
    }
    return segments.join(' > ');
  };

  const attributesFor = (element) => {
    const result = {};
    for (const attr of Array.from(element.attributes || [])) {
      if (['id', 'class', 'name', 'type', 'role', 'aria-label', 'placeholder', 'href', 'data-testid'].includes(attr.name)) {
        result[attr.name] = attr.value.slice(0, 180);
      }
    }
    return result;
  };

  window.__dunderiaBrowserLab = {
    inspectMode: false,
    selection: null,
    setInspectMode(value) {
      this.inspectMode = Boolean(value);
      document.documentElement.style.cursor = this.inspectMode ? 'crosshair' : '';
    },
    consumeSelection() {
      const next = this.selection;
      this.selection = null;
      return next;
    },
  };

  window.addEventListener('click', (event) => {
    const state = window.__dunderiaBrowserLab;
    if (!state || !state.inspectMode) return;
    event.preventDefault();
    event.stopPropagation();
    event.stopImmediatePropagation();

    const element = event.target instanceof Element ? event.target : document.elementFromPoint(event.clientX, event.clientY);
    if (!element) return;
    const rect = element.getBoundingClientRect();
    state.selection = {
      tagName: element.tagName.toLowerCase(),
      id: element.id || undefined,
      classes: Array.from(element.classList || []).slice(0, 8),
      textContent: (element.innerText || element.textContent || '').replace(/\s+/g, ' ').trim().slice(0, 400),
      selector: selectorFor(element),
      pageUrl: window.location.href,
      viewport: {
        width: window.innerWidth,
        height: window.innerHeight,
        label: 'Embedded',
      },
      click: {
        x: Math.round(event.clientX),
        y: Math.round(event.clientY),
      },
      boundingBox: {
        x: Math.round(rect.x),
        y: Math.round(rect.y),
        width: Math.round(rect.width),
        height: Math.round(rect.height),
      },
      attributes: attributesFor(element),
    };
    state.setInspectMode(false);
  }, true);
})();
`

function checkURL(url) {
  return new Promise((resolve) => {
    const req = http.get(url, (res) => {
      res.resume()
      resolve(res.statusCode && res.statusCode < 500)
    })
    req.on('error', () => resolve(false))
    req.setTimeout(800, () => {
      req.destroy()
      resolve(false)
    })
  })
}

async function ensureBroker() {
  if (process.env.MAESTRIA_DESKTOP_NO_BROKER === '1' || process.env.DUNDERIA_DESKTOP_NO_BROKER === '1') return
  if (await checkURL(webURL)) return

  const exe = process.env.MAESTRIA_BROKER_EXE || process.env.DUNDERIA_BROKER_EXE || path.join(repoRoot, 'wuphf.exe')
  brokerProcess = spawn(exe, ['--no-open', '--web-port', '7891'], {
    cwd: repoRoot,
    detached: false,
    stdio: 'ignore',
    windowsHide: true,
  })

  brokerProcess.on('exit', () => {
    brokerProcess = undefined
    updateTray()
  })
  updateTray()

  for (let i = 0; i < 25; i += 1) {
    if (await checkURL(webURL)) return
    await new Promise((resolve) => setTimeout(resolve, 250))
  }
}

async function createWindow() {
  await ensureBroker()

  const win = new BrowserWindow({
    width: 1420,
    height: 900,
    minWidth: 1100,
    minHeight: 720,
    title: 'DunderIA',
    backgroundColor: '#f7f3ec',
    autoHideMenuBar: true,
    webPreferences: {
      preload: path.join(__dirname, 'preload.cjs'),
      contextIsolation: true,
      nodeIntegration: false,
      webviewTag: false,
    },
  })
  mainWindow = win

  win.webContents.setWindowOpenHandler(({ url }) => {
    shell.openExternal(url)
    return { action: 'deny' }
  })
  win.on('move', applyBrowserLabWindowBounds)
  win.on('resize', applyBrowserLabWindowBounds)

  await win.loadURL(webURL)
  updateTray()
}

function createTray() {
  if (tray) return
  const iconPath = path.join(repoRoot, 'web', 'public', 'favicon-32.png')
  const image = nativeImage.createFromPath(iconPath)
  tray = new Tray(image.isEmpty() ? nativeImage.createEmpty() : image)
  tray.setToolTip('DunderIA')
  tray.on('click', () => {
    if (mainWindow && !mainWindow.isDestroyed()) {
      mainWindow.show()
      mainWindow.focus()
    }
  })
  updateTray()
}

function updateTray() {
  if (!tray) return
  const brokerLabel = brokerProcess ? 'Runtime iniciado pelo desktop' : 'Runtime externo ou já ativo'
  tray.setContextMenu(Menu.buildFromTemplate([
    { label: 'DunderIA', enabled: false },
    { label: brokerLabel, enabled: false },
    { type: 'separator' },
    {
      label: 'Abrir',
      click: () => {
        if (!mainWindow || mainWindow.isDestroyed()) {
          createWindow().catch((err) => console.error('[dunderia-desktop] failed to open', err))
          return
        }
        mainWindow.show()
        mainWindow.focus()
      },
    },
    {
      label: 'Recarregar',
      click: () => {
        if (mainWindow && !mainWindow.isDestroyed()) mainWindow.webContents.reload()
      },
    },
    {
      label: 'Runtime doctor',
      click: () => {
        if (mainWindow && !mainWindow.isDestroyed()) {
          mainWindow.loadURL(`${webURL}/#/apps/studio`).catch(() => {})
          mainWindow.show()
          mainWindow.focus()
        } else {
          shell.openExternal(`${webURL}/#/apps/studio`)
        }
      },
    },
    { type: 'separator' },
    { label: 'Sair', click: () => app.quit() },
  ]))
}

function emitBrowserLabEvent(extra = {}) {
  if (!mainWindow || mainWindow.isDestroyed()) return
  const webContents = browserLabWindow && !browserLabWindow.isDestroyed() ? browserLabWindow.webContents : null
  mainWindow.webContents.send('browser-lab:event', {
    url: browserLabState.url,
    title: browserLabState.title,
    loading: browserLabState.loading,
    ready: browserLabState.ready,
    canGoBack: Boolean(webContents?.canGoBack()),
    canGoForward: Boolean(webContents?.canGoForward()),
    ...extra,
  })
}

function applyBrowserLabWindowBounds() {
  if (!mainWindow || mainWindow.isDestroyed() || !browserLabWindow || browserLabWindow.isDestroyed()) return
  const bounds = browserLabState.bounds
  if (!bounds.width || !bounds.height) {
    browserLabWindow.hide()
    return
  }
  const content = mainWindow.getContentBounds()
  browserLabWindow.setBounds({
    x: content.x + bounds.x,
    y: content.y + bounds.y,
    width: bounds.width,
    height: bounds.height,
  })
  if (!browserLabWindow.isVisible()) {
    browserLabWindow.show()
  }
  browserLabWindow.moveTop()
}

function ensureBrowserLabWindow() {
  if (!mainWindow || mainWindow.isDestroyed()) {
    throw new Error('DunderIA window is not available')
  }
  if (browserLabWindow && !browserLabWindow.isDestroyed()) return browserLabWindow

  browserLabWindow = new BrowserWindow({
    parent: mainWindow,
    frame: false,
    show: false,
    skipTaskbar: true,
    resizable: false,
    movable: false,
    minimizable: false,
    maximizable: false,
    closable: false,
    backgroundColor: '#ffffff',
    webPreferences: {
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: true,
    },
  })

  browserLabWindow.setMenuBarVisibility(false)
  applyBrowserLabWindowBounds()

  const webContents = browserLabWindow.webContents
  webContents.setWindowOpenHandler(({ url }) => {
    shell.openExternal(url)
    return { action: 'deny' }
  })
  webContents.on('did-start-loading', () => {
    browserLabState.loading = true
    browserLabState.ready = false
    emitBrowserLabEvent()
  })
  webContents.on('did-stop-loading', () => {
    browserLabState.loading = false
    browserLabState.url = webContents.getURL() || browserLabState.url
    applyBrowserLabWindowBounds()
    emitBrowserLabEvent()
  })
  webContents.on('dom-ready', () => {
    browserLabState.ready = true
    browserLabState.url = webContents.getURL() || browserLabState.url
    browserLabState.title = webContents.getTitle() || browserLabState.title
    applyBrowserLabWindowBounds()
    void webContents.executeJavaScript(browserLabInspectScript).catch(() => {})
    emitBrowserLabEvent()
  })
  webContents.on('page-title-updated', (_event, title) => {
    browserLabState.title = title || ''
    emitBrowserLabEvent()
  })
  webContents.on('did-navigate', (_event, url) => {
    browserLabState.url = url || browserLabState.url
    browserLabState.ready = false
    emitBrowserLabEvent()
  })
  webContents.on('did-navigate-in-page', (_event, url) => {
    browserLabState.url = url || browserLabState.url
    emitBrowserLabEvent()
  })
  webContents.on('did-fail-load', (_event, _code, description, validatedURL) => {
    browserLabState.loading = false
    browserLabState.ready = false
    emitBrowserLabEvent({ error: `Falha ao carregar ${validatedURL || 'a página'}: ${description || 'erro desconhecido'}.` })
  })

  browserLabWindow.on('closed', () => {
    browserLabWindow = undefined
  })

  return browserLabWindow
}

function normalizeBrowserLabBounds(bounds) {
  const next = {
    x: Math.max(0, Math.round(Number(bounds?.x) || 0)),
    y: Math.max(0, Math.round(Number(bounds?.y) || 0)),
    width: Math.max(0, Math.round(Number(bounds?.width) || 0)),
    height: Math.max(0, Math.round(Number(bounds?.height) || 0)),
  }
  return next
}

ipcMain.handle('browser-lab:set-bounds', (_event, bounds) => {
  ensureBrowserLabWindow()
  const next = normalizeBrowserLabBounds(bounds)
  browserLabState.bounds = next
  applyBrowserLabWindowBounds()
  return browserLabState
})

ipcMain.handle('browser-lab:navigate', async (_event, url) => {
  const win = ensureBrowserLabWindow()
  const nextURL = String(url || '').trim() || browserLabState.url
  browserLabState.url = nextURL
  browserLabState.ready = false
  await win.webContents.loadURL(nextURL)
  applyBrowserLabWindowBounds()
  return browserLabState
})

ipcMain.handle('browser-lab:back', () => {
  const win = ensureBrowserLabWindow()
  if (win.webContents.canGoBack()) win.webContents.goBack()
})

ipcMain.handle('browser-lab:forward', () => {
  const win = ensureBrowserLabWindow()
  if (win.webContents.canGoForward()) win.webContents.goForward()
})

ipcMain.handle('browser-lab:reload', () => {
  const win = ensureBrowserLabWindow()
  win.webContents.reload()
})

ipcMain.handle('browser-lab:hide', () => {
  if (!browserLabWindow || browserLabWindow.isDestroyed()) return
  browserLabWindow.hide()
})

ipcMain.handle('browser-lab:get-state', () => ({
  ...browserLabState,
  canGoBack: Boolean(browserLabWindow && !browserLabWindow.isDestroyed() && browserLabWindow.webContents.canGoBack()),
  canGoForward: Boolean(browserLabWindow && !browserLabWindow.isDestroyed() && browserLabWindow.webContents.canGoForward()),
}))

ipcMain.handle('browser-lab:set-inspect-mode', async (_event, enabled) => {
  const win = ensureBrowserLabWindow()
  if (!browserLabState.ready) return null
  await win.webContents.executeJavaScript(`${browserLabInspectScript}; window.__dunderiaBrowserLab.setInspectMode(${enabled ? 'true' : 'false'});`)
  return null
})

ipcMain.handle('browser-lab:consume-selection', async () => {
  const win = ensureBrowserLabWindow()
  if (!browserLabState.ready) return null
  return win.webContents.executeJavaScript('window.__dunderiaBrowserLab?.consumeSelection?.() ?? null')
})

app.whenReady().then(() => {
  app.setAppUserModelId('DunderIA')
  Menu.setApplicationMenu(null)
  createTray()
  createWindow().catch((err) => {
    console.error('[dunderia-desktop] failed to start', err)
    app.quit()
  })
})

app.on('window-all-closed', () => {
  if (!tray && process.platform !== 'darwin') app.quit()
})

app.on('activate', () => {
  if (BrowserWindow.getAllWindows().length === 0) {
    createWindow().catch((err) => {
      console.error('[dunderia-desktop] failed to reactivate', err)
    })
  }
})

app.on('before-quit', () => {
  if (brokerProcess) {
    brokerProcess.kill()
    brokerProcess = undefined
  }
})
