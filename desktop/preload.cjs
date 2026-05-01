const { contextBridge, ipcRenderer } = require('electron')

contextBridge.exposeInMainWorld('dunderiaDesktop', {
  isDesktop: true,
  runtime: 'electron',
  browserLab: {
    setBounds: (bounds) => ipcRenderer.invoke('browser-lab:set-bounds', bounds),
    navigate: (url) => ipcRenderer.invoke('browser-lab:navigate', url),
    back: () => ipcRenderer.invoke('browser-lab:back'),
    forward: () => ipcRenderer.invoke('browser-lab:forward'),
    reload: () => ipcRenderer.invoke('browser-lab:reload'),
    hide: () => ipcRenderer.invoke('browser-lab:hide'),
    setInspectMode: (enabled) => ipcRenderer.invoke('browser-lab:set-inspect-mode', enabled),
    consumeSelection: () => ipcRenderer.invoke('browser-lab:consume-selection'),
    getState: () => ipcRenderer.invoke('browser-lab:get-state'),
    onEvent: (callback) => {
      const listener = (_event, payload) => callback(payload)
      ipcRenderer.on('browser-lab:event', listener)
      return () => ipcRenderer.removeListener('browser-lab:event', listener)
    },
  },
})
