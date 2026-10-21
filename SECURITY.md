# Security Policy

Visper is an end-to-end encrypted chat, so security reports are taken seriously.

## Supported versions

Only the latest commit on `main` is supported. Fixes are not backported.

## Reporting a vulnerability

**Please do not open a public issue.** Instead, report it privately through
[GitHub Security Advisories](https://github.com/HilthonTT/Visper/security/advisories/new).

Include:

- the affected component (`api`, `api-sdk`, `cli`, `proxy`, `ai`, `web`),
- a description of the issue and its impact,
- steps or a minimal proof of concept to reproduce it.

You can expect an acknowledgement within a few days. Once a fix is available,
the advisory will be published with credit to the reporter unless you prefer
to stay anonymous.

## Scope

In scope: the encryption scheme, room access control, authentication/session
handling, file upload/storage, rate limiting and the reverse proxy.

Out of scope: denial of service through sheer traffic volume, issues in
third-party dependencies that are already publicly known (Dependabot tracks
those), and the default development credentials in `docker-compose.yml` /
`.env.example`, which must be changed for any real deployment.
