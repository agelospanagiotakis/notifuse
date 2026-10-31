# e-Pragma fork of Notifuse

This is a **fork of [Notifuse/notifuse](https://github.com/Notifuse/notifuse)** used to run the
newsletter platform at `newsletters.e-pragma.gr` and the blog at `blog.e-pragma.gr`.

| | |
|---|---|
| Base release | **upstream `v40.0`** (`7cf0bfde` / tag `v40.0`) |
| Our branch | `epragma` |
| Our release tag | `v40.0-el.1` — `<upstream base>-el.<our build>` |
| Image | `ghcr.io/agelospanagiotakis/notifuse-epragma:v40.0-el.1` (`linux/amd64`) |

## What this fork changes

Five commits on top of v40.0, all in `epragma`:

1. **Greek notification centre** — `el` added to the SPA's 21 strings and its language selector.
   Upstream ships 11 languages and Greek is not one of them, so subscribers clicking a
   confirm/unsubscribe link got English.
2. **e-Pragma branding** — palette for the notification centre and the blog theme, taken from
   www.e-pragma.gr (`#ff7a1a`, `#4051b5`, `#12141f`, Roboto).
3. **Password sign-in** — bcrypt against a new `users.password_hash` column, alongside the
   existing magic codes. `POST /api/user.signin` accepts an optional `password` and returns a
   token; `POST /api/user.setPassword` sets or changes it. Accounts without a password keep the
   code flow unchanged.
4. **Console catalogs** — the strings above extracted into the lingui catalogs, without which
   they render as ids instead of text.
5. **CI** — `.github/workflows/image.yml`, building `linux/amd64` and publishing to GHCR.

Everything else is upstream, untouched.

## Why it is pinned to v40.0

Upstream is already at **v42.1**. Upgrading is not free:

| Version | Licensed Features in the BSL |
|---|---|
| v40.0 (ours) | 5 |
| **v42.1** | **6** — adds *"recording an audit log of administrative actions"* |

The licence is **Business Source License 1.1**, not AGPL: production use is allowed, but each
"Licensed Feature" needs a paid key. v39.x and earlier remain AGPL-3.0 forever; v40.0 becomes
AGPL on **2030-09-04**. `web_analytics_sdk/` is separately AGPL-3.0.

Current compliance of the e-Pragma deployment (verified 2026-10-31): **2 of 3 workspaces**, no
customised permission sets, no SES tenant, no SSO, **0 of 13 templates** with language variants.
The binding constraint is the **three-workspace** ceiling.

## Building

CI builds on tags; nothing is built on a laptop. The reasons are recorded here because each one
cost time:

* `ubuntu-latest` is x86_64 — building on Apple silicon produced an arm64 image that died with
  `exec format error` on the server.
* the build needs several GB and ~30 GB of cache.
* **a fork's `GITHUB_TOKEN` is read-only by default**, which fails the registry login. This repo
  sets *Settings → Actions → General → Workflow permissions → Read and write*.
* the package must belong to the fork. `ghcr.io/agelospanagiotakis/notifuse` stayed bound to the
  repository that first created it, so this fork publishes to `…/notifuse-epragma` instead.

```sh
git tag -a v40.0-el.2 -m "…"
git push origin v40.0-el.2        # CI builds and publishes
```

## Keeping up with upstream

```sh
git fetch upstream --tags
git checkout main && git reset --hard upstream/main && git push origin main   # mirror
git checkout epragma
git rebase upstream/v41.0                     # or whichever tag you are moving to
```

Check `LICENSE` at the new tag **before** rebasing — that is how the sixth Licensed Feature was
discovered — and re-read the "Licensed Features" list, because the set changes between releases.

## Upstream contributions

`CLA.md` is a one-way grant: contributing gives Notifuse the right to relicense the contribution
on any terms. The Greek notification-centre translation is the one change worth offering upstream
(it is 21 strings and a language entry); password auth and our branding stay here.

## What consumes this image

`agelospanagiotakis/epragma-mailer` → `Dockerfile`:
`FROM ghcr.io/agelospanagiotakis/notifuse-epragma:v40.0-el.1`, deployed by Coolify as app 34 on
server3. Coolify builds with `--pull`, so the tag must exist in the registry before a deploy.
