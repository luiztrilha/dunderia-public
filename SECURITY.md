# Security Policy

## Supported Versions

Security fixes are handled on a best-effort basis for:

- the latest tagged release
- `main`, when the fix has not been released yet

Older tags and unpublished forks should not be assumed to receive coordinated fixes.

## Reporting a Vulnerability

Please do not post vulnerability details in public GitHub issues, pull requests, or Discord.

Preferred path:

1. Use GitHub's private `Report a vulnerability` flow for this repository if it is enabled.
2. If that flow is not available, open a minimal public issue without exploit details and ask for a private reporting path.

When you report an issue, include:

- affected version or commit
- impact and attack surface
- reproduction steps
- whether credentials, tokens, or local files are exposed

## What To Expect

- Best-effort triage. There is no formal SLA yet.
- A request for reproduction details or a reduced test case if needed.
- A public fix note after the issue is understood and a safe patch path exists.

## Scope Notes

DunderIA is local-first. Many risks are operational rather than hosted-service risks. In particular, reports are especially helpful for:

- credential handling
- command execution boundaries
- broker or web auth gaps
- unsafe logging of sensitive content
- integration surfaces such as Telegram, One, Composio, and optional Nex paths

## Handling Secrets

If your report involves tokens, API keys, chat contents, or private files:

- redact them before sharing
- rotate exposed credentials before filing the report
- avoid posting screenshots or logs with live sensitive content
