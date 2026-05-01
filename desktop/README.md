# DunderIA Desktop

Electron shell for DunderIA tools that need embedded Chromium, especially Browser Lab.
It also keeps a small tray menu available for opening the app, reloading the web
view, and jumping straight to Runtime Doctor.

Run from this directory after installing dependencies:

```powershell
npm install
npm run start
```

Environment knobs:

- `MAESTRIA_WEB_URL`: web UI URL to load. Defaults to `http://127.0.0.1:7891`.
- `MAESTRIA_BROKER_EXE`: broker executable path. Defaults to `..\wuphf.exe`.
- `MAESTRIA_DESKTOP_NO_BROKER=1`: skip auto-starting the broker.

The legacy `DUNDERIA_*` variables still work as compatibility aliases.
