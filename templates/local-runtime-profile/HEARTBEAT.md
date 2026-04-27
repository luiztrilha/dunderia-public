# HEARTBEAT.md

Read `heartbeat-state.json`. Run only the most overdue check that is currently in window.

Checks:
- `gateway`: every 30 min, 06:00-23:00. Confirm `Relatorios/OpenClaw/openclaw-runtime.json` still shows gateway `ok=true`.
- `swarm`: every 30 min, 06:00-23:00. Check `Swarm/supervisor-status.md` for blocked, failed, or stale execution.
- `tasks`: every 2 hours, 06:00-23:00. Check backlog/task health only if something looks stalled or noisy, and confirm whether any benchmark follow-up in `Relatorios/OpenClaw/benchmark-followups.json` is due or overdue.
- `memory`: every 24 hours, 08:00-22:00. Check whether durable operational decisions from recent work should be promoted to `MEMORY.md`.

Process:
1. Load timestamps from `heartbeat-state.json`.
2. Ignore checks outside their time window.
3. Pick only the most overdue remaining check.
4. Run the minimum read-only verification needed.
5. If nothing is actionable, update that check timestamp and reply `HEARTBEAT_OK`.
6. If something is actionable, report only the concrete issue and recommended next step, then update the timestamp if the check itself completed.

Rules:
- Do not rerun every check in one heartbeat.
- Do not repeat old tasks from prior chats.
- Prefer local artifacts and runtime state over inference.
- DR rapido: `D:\Repos\Relatorios\OpenClaw\playbook-recuperacao-openclaw-pos-interrupcao.md`
