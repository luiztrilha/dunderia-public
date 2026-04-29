package team

import (
	"net/http"
)

func (b *Broker) handleWebLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodHead)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")
	_, _ = w.Write([]byte(webLoginHTML))
}

const webLoginHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>WUPHF Office Login</title>
  <style>
    :root {
      color-scheme: light dark;
      --bg: #f6f1e8;
      --ink: #171511;
      --muted: #655f55;
      --line: #d9cec0;
      --panel: #fffaf1;
      --field: #fffdf8;
      --accent: #0a6f68;
      --accent-ink: #ffffff;
      --danger: #a33224;
      --shadow: 0 24px 80px rgba(42, 34, 22, 0.18);
      font-family: ui-serif, Georgia, Cambria, "Times New Roman", Times, serif;
    }

    @media (prefers-color-scheme: dark) {
      :root {
        --bg: #161411;
        --ink: #f7efe2;
        --muted: #c7baaa;
        --line: #40392f;
        --panel: #211d18;
        --field: #181511;
        --accent: #4fb7a9;
        --accent-ink: #0d1716;
        --danger: #ff907f;
        --shadow: 0 24px 80px rgba(0, 0, 0, 0.38);
      }
    }

    * {
      box-sizing: border-box;
    }

    body {
      min-height: 100vh;
      margin: 0;
      color: var(--ink);
      background:
        linear-gradient(90deg, rgba(23, 21, 17, 0.045) 1px, transparent 1px) 0 0 / 44px 44px,
        linear-gradient(0deg, rgba(23, 21, 17, 0.035) 1px, transparent 1px) 0 0 / 44px 44px,
        radial-gradient(circle at 12% 8%, rgba(10, 111, 104, 0.16), transparent 32rem),
        var(--bg);
      display: grid;
      place-items: center;
      padding: 32px 18px;
    }

    main {
      width: min(100%, 940px);
      min-height: 560px;
      display: grid;
      grid-template-columns: 0.95fr 1.05fr;
      border: 1px solid var(--line);
      border-radius: 8px;
      background: var(--panel);
      box-shadow: var(--shadow);
      overflow: hidden;
    }

    .intro {
      padding: clamp(28px, 6vw, 58px);
      border-right: 1px solid var(--line);
      background:
        linear-gradient(135deg, rgba(10, 111, 104, 0.1), transparent 42%),
        repeating-linear-gradient(135deg, rgba(101, 95, 85, 0.1) 0 1px, transparent 1px 15px);
      display: flex;
      flex-direction: column;
      justify-content: space-between;
      gap: 36px;
    }

    .brand {
      display: inline-flex;
      align-items: center;
      gap: 10px;
      font: 700 0.78rem/1 ui-sans-serif, system-ui, sans-serif;
      letter-spacing: 0.12em;
      text-transform: uppercase;
    }

    .mark {
      width: 30px;
      height: 30px;
      border: 2px solid var(--ink);
      border-radius: 50%;
      display: grid;
      place-items: center;
      font-size: 0.72rem;
    }

    h1 {
      max-width: 9ch;
      margin: 0;
      font-size: clamp(3rem, 8vw, 5.9rem);
      line-height: 0.9;
      letter-spacing: 0;
    }

    .intro p {
      max-width: 28rem;
      margin: 0;
      color: var(--muted);
      font: 500 1rem/1.65 ui-sans-serif, system-ui, sans-serif;
    }

    .form-shell {
      padding: clamp(28px, 6vw, 58px);
      display: grid;
      align-content: center;
    }

    form {
      display: grid;
      gap: 18px;
    }

    .field {
      display: grid;
      gap: 8px;
    }

    label {
      font: 800 0.78rem/1 ui-sans-serif, system-ui, sans-serif;
      letter-spacing: 0.08em;
      text-transform: uppercase;
    }

    input {
      width: 100%;
      min-height: 52px;
      border: 1px solid var(--line);
      border-radius: 6px;
      padding: 0 14px;
      background: var(--field);
      color: var(--ink);
      font: 600 1rem/1.2 ui-sans-serif, system-ui, sans-serif;
      outline: 3px solid transparent;
      outline-offset: 1px;
      transition: border-color 140ms ease, outline-color 140ms ease;
    }

    input:focus-visible {
      border-color: var(--accent);
      outline-color: color-mix(in srgb, var(--accent), transparent 72%);
    }

    .hint,
    .status {
      margin: 0;
      color: var(--muted);
      font: 500 0.92rem/1.55 ui-sans-serif, system-ui, sans-serif;
    }

    .status[role="alert"] {
      color: var(--danger);
    }

    button {
      min-height: 54px;
      border: 0;
      border-radius: 6px;
      padding: 0 18px;
      background: var(--accent);
      color: var(--accent-ink);
      cursor: pointer;
      font: 900 0.88rem/1 ui-sans-serif, system-ui, sans-serif;
      letter-spacing: 0.1em;
      text-transform: uppercase;
      transition: transform 140ms ease, filter 140ms ease;
    }

    button:hover {
      filter: brightness(1.04);
      transform: translateY(-1px);
    }

    button:focus-visible {
      outline: 3px solid color-mix(in srgb, var(--accent), transparent 62%);
      outline-offset: 3px;
    }

    button:disabled {
      cursor: wait;
      opacity: 0.72;
      transform: none;
    }

    @media (max-width: 760px) {
      body {
        align-items: start;
      }

      main {
        grid-template-columns: 1fr;
        min-height: auto;
      }

      .intro {
        min-height: 280px;
        border-right: 0;
        border-bottom: 1px solid var(--line);
      }

      h1 {
        max-width: 8ch;
      }
    }
  </style>
</head>
<body>
  <main>
    <section class="intro" aria-labelledby="login-title">
      <div class="brand"><span class="mark" aria-hidden="true">W</span> WUPHF Office</div>
      <h1 id="login-title">Local office access.</h1>
      <p>Connect this browser to the broker running on this machine. The access key stays in local browser storage.</p>
    </section>

    <section class="form-shell" aria-label="Sign in">
      <form id="login-form" autocomplete="off">
        <div class="field">
          <label for="access-key">Broker access key</label>
          <input id="access-key" name="access_key" type="password" required spellcheck="false" autocomplete="current-password" aria-describedby="access-key-help">
          <p id="access-key-help" class="hint">Use the key from the local broker, or let this page detect it automatically.</p>
        </div>

        <input id="broker-url" name="broker_url" type="hidden">
        <p id="status" class="status" aria-live="polite">Checking local broker session...</p>
        <button id="submit" type="submit">Enter office</button>
      </form>
    </section>
  </main>

  <script>
    const form = document.querySelector("#login-form");
    const tokenInput = document.querySelector("#access-key");
    const brokerInput = document.querySelector("#broker-url");
    const status = document.querySelector("#status");
    const button = document.querySelector("#submit");

    const nextPath = (() => {
      const value = new URLSearchParams(window.location.search).get("next") || "/";
      return value.startsWith("/") && !value.startsWith("//") ? value : "/";
    })();

    async function detectLocalSession() {
      try {
        const response = await fetch("/api-token", { cache: "no-store" });
        if (!response.ok) throw new Error("Token endpoint returned " + response.status);
        const data = await response.json();
        tokenInput.value = data.token || "";
        brokerInput.value = data.broker_url || "";
        status.textContent = tokenInput.value ? "Local broker detected. Review and continue." : "Enter the broker access key to continue.";
      } catch (error) {
        status.textContent = "Enter the broker access key to continue.";
      }
    }

    form.addEventListener("submit", (event) => {
      event.preventDefault();
      const token = tokenInput.value.trim();
      if (!token) {
        status.setAttribute("role", "alert");
        status.textContent = "Broker access key is required.";
        tokenInput.focus();
        return;
      }

      button.disabled = true;
      window.localStorage.setItem("wuphf.brokerToken", token);
      window.localStorage.setItem("wuphf.authenticated", "true");
      if (brokerInput.value) {
        window.localStorage.setItem("wuphf.brokerURL", brokerInput.value);
      }
      window.location.assign(nextPath);
    });

    detectLocalSession();
  </script>
</body>
</html>
`
