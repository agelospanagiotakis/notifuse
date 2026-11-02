# FORK_TODO — screens and import features

What we want our fork to do that it does not do yet, in the order it should be built.
The order is not preference, it is dependency: each group below needs the one above it
to exist, and building them out of order means building the same thing twice.

Two rules for everything here:

1. **Do not send email.** Every import path creates drafts and records. The only tool
   that sends is the one a person presses, in the console, on a campaign they wrote.
2. **Do not invent data.** If a field cannot be known (a past campaign's real open rate,
   a subscriber's consent date), leave it empty and say so in the UI, rather than
   filling it with something plausible.

---

## 1. Archive screen (console) — the smallest useful loop

The archive exists in the API and has no UI, so changing its title, its password or
turning it off currently means SQL on the server. That is the wrong shape for a feature
meant to be handed to an operator.

- [ ] **Settings → Campaign archive**: enable toggle, title, Subscribe URL, basic-auth
      user and password (or "public"), and a live link to `/archive/<workspace>/`.
- [ ] **Per-campaign publish/unpublish**: a campaign is archived by a row existing in
      `campaign_archive`; today only the import script writes rows. Needs a table view
      with a search box and a "remove from archive" action.
- [ ] **Preview link** on each archived campaign, opening the public URL.
- [ ] **Show the gate state** at the top of the archive list page while it is behind
      basic auth, so the reader knows why they were asked for a password.

**Depends on:** nothing. Smallest change with the highest operational payoff.
**Touches:** `console/src/pages/`, `internal/http/archive_handler.go` (admin routes).

## 2. Import wizard (Mailchimp) — one screen instead of four scripts

Today: a Python file on the server, a `.env` with an API key, a manual `--workspace`
argument, and a log to read afterwards. The logic works; the interface is a shell.

- [ ] **Step 1 — credentials**: Mailchimp API key, validated by a real call
      (`GET /3.0/`), stored encrypted per workspace rather than in `/root/.mailchimp.env`.
- [ ] **Step 2 — inventory**: show what is there before importing anything — campaigns by
      year, templates, audiences, subscriber counts, total asset bytes. This is the step
      that stops a 2 GB surprise.
- [ ] **Step 3 — choose what to import**: checkboxes for *templates*, *assets*,
      *archive entries*, *broadcast history*, *audiences*. Each with its own consequence
      spelled out (assets = a copy in your object store, audiences = consent question).
- [ ] **Step 4 — run**: progress per campaign, resumable, and a failure list with a
      retry button. The server-side script already does this; the screen only has to
      report it.
- [ ] **Step 5 — report**: what was created, what was skipped and why, what needs a human.

**Depends on:** 1 (so the archive has somewhere to point).
**Note:** the importer's own resilience is not optional here — a wizard that dies at
campaign 101 of 182 is worse than a script, because the user is watching.

## 3. Imported content browser — make 182 foreign campaigns usable

- [ ] **Campaigns list with filters**: year, has-errors, missing assets, already archived.
- [ ] **"Re-import this one"** action per campaign, and a bulk retry for the failure list.
- [ ] **Asset health**: a column showing whether every image in the template resolves,
      so the old `gallery.mailchimp.com` holes are visible instead of surprising a reader.
- [ ] **Bulk edit names/tags**: imported names are truncated to 32 bytes at a word
      boundary; the ones that matter deserve a better name.

**Depends on:** 2.

## 4. Broadcast history — the internal record

- [ ] **One draft broadcast per imported campaign**, with its original send date, so the
      console shows what was sent and when rather than only what it looked like.
- [ ] **Backfilled statistics are marked as absent**, not as zero. A campaign that
      Notifuse never sent has no opens, and a screen that shows `0 opens` is lying.
- [ ] **Link each broadcast to its template version**, which is what the archive already
      does — the same pointer, two uses.

**Depends on:** 2. This is the phase that makes the console a record instead of a library.

## 5. Audiences and consent — the one that needs a decision, not code

- [ ] **Audience import screen** with the three questions surfaced in the UI:
      double opt-in or not, how to dedupe the same person across audiences, and what
      consent basis is being carried over.
- [ ] **Do not import unsubscribed contacts as subscribed.** Mailchimp's status must map
      to ours exactly, or the first send mails people who opted out — the single most
      expensive mistake this project could make.
- [ ] **Tags and segments** as labels, not as sendable lists, until consent is settled.

**Depends on:** a decision from the owner. Deliberately last of the import work.

## 6. Templates — small things that add up

- [ ] **Column filters** (category, channel, "has MJML source") and **sorting by date**.
- [ ] **Grid view with thumbnails**: 182 campaigns are impossible to browse as rows, and a
      thumbnail is rendered from the same compile the preview already does.
- [ ] **Duplicate as MJML/HTML**: imported campaigns are a useful starting point for a new
      newsletter; today the only path is to copy and paste.

**Depends on:** nothing, but low value until 2 and 3 exist.

## 7. Explicitly out of scope

Written down so nobody re-opens it by accident.

- **Automations**: Mailchimp journeys do not map onto Notifuse automations without a
  design, and a half-migrated automation is a live risk.
- **A/B test results**: the data does not survive the export; importing a winner is
  inventing history.
- **Sending imported campaigns**: never. The archive is a reading surface.
- **Upstream v42.1**: adds a sixth Licensed Feature. An upgrade is a licence decision
  first, read `LICENSE` at the target tag.

---

## Order in one line

**Archive screen → import wizard → imported-content browser → broadcast history →
audiences (needs a decision) → template niceties.**
