# Security Policy

## Supported versions

Solo is developed on `main`. Security fixes are applied to the latest minor
release. Older minors may receive fixes at the maintainer's discretion.

| Version | Supported           |
| ------- | ------------------- |
| latest  | ✅                  |
| older   | ⚠️ best effort      |

## Reporting a vulnerability

**Please do not file public GitHub issues for suspected vulnerabilities.**

Send a private report to:

**`fredal_zhu@outlook.com`**

Include in your report:

- A clear description of the issue and its impact
- Steps to reproduce, or a proof-of-concept
- Affected versions / commits
- Any known workarounds

You should hear back within **7 days**. If you don't, feel free to follow up
on the same thread.

## What to expect

- **Acknowledgement** within a few days of the report
- **Triage** — confirm the issue, scope the impact, decide on a fix
- **Patch** — a fix is developed and tested privately
- **Disclosure** — once a fix is shipped, a public advisory is published with
  credit to the reporter (unless you prefer to stay anonymous)

Solo is a small, solo-maintained project. There is no formal SLA. The goal is
to acknowledge quickly, fix honestly, and disclose responsibly.

## Scope

In scope:

- Authentication and authorization bypass
- Cross-tenant data exposure (channels, DMs, tasks, attachments)
- Remote code execution, including the agent daemon's tool execution surface
- SQL injection, SSRF, or path traversal in the API
- Dependency vulnerabilities with a real exploit path against Solo

Out of scope:

- Issues that require a malicious or compromised local user
- Theoretical issues without a concrete attack path
- "Self-XSS" through content the user controls and renders themselves
- Missing security headers on a development-only setup

## Recognition

Thanks to the people who report vulnerabilities responsibly.
