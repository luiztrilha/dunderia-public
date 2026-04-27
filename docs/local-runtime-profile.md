# Local Runtime Profile

DunderIA treats a working agent setup as a local runtime profile: the reusable
parts of a machine that make agents behave consistently without turning a
private computer into a public artifact.

The profile has three layers:

- `skills`: executable agent capabilities, each with its own `SKILL.md` and
  supporting references or scripts.
- `orientation`: Markdown guidance for agents, commands, prompts, READMEs and
  operating notes that shape behavior but are not themselves installable skills.
- `config`: restorable settings, shell profiles, lock files and sanitized tool
  configuration.

This split keeps publication and backup decisions simple. Skills can be shared
when their content is reusable. Orientation can be reviewed as documentation.
Config must be sanitized before it is published.

## Public Starter Kit

The public starter profile lives in:

```text
templates/starter-kit/
```

It contains a sanitized Codex/agent profile, validated skills, prompts, rules
and an installer script. It is meant to help a new DunderIA workspace start with
a useful baseline while still forcing local credentials and private state to be
created on the new machine.

## Private Backup Shape

For personal backup, keep a private snapshot with this shape:

```text
local-runtime/
  config/
  orientation/
  skills/
```

The private snapshot can include machine-specific preferences and local
orientation, but it should still exclude volatile or secret-bearing files.

## Never Publish

Do not publish:

- auth files, OAuth credentials, API keys, session files or tokens
- database files, logs, shell history, model caches or local telemetry
- private repository paths, customer names, channel history or task receipts
- live office state such as `company.json`, `broker-state.json` or direct
  message history
- encrypted vault passphrases or files that explain how to decrypt a private
  recovery bundle

## Safe Publication Checklist

Before making a runtime profile public:

```powershell
rg -n "api_key|token|secret|password|credential|Authorization|Bearer|BEGIN .*PRIVATE|client_secret|refresh_token" .
rg -n "C:\\\\Users|D:\\\\|auth\\.json|company\\.json|broker-state|application_default_credentials" .
git status --short
```

Every match should be reviewed manually. Some matches are expected in docs that
describe exclusions, but no match should reveal a real credential, private path
or live state.

## Restore Rule

Restore public profile files selectively. A public profile should bootstrap a
workspace, not overwrite a user's existing runtime blindly.

Recommended order:

1. Install DunderIA.
2. Run `wuphf init`.
3. Review `templates/starter-kit/EXCLUSIONS.md`.
4. Install the starter kit with `templates/starter-kit/install-profile.ps1`.
5. Add credentials locally through the target tool's normal login flow.
