---
description: Sync FEATURES.md with recent code changes - new features, schema updates, refactors, removals
argument-hint: "[optional feature name, change summary, or git range like main..HEAD]"
---

Inspect what changed in the repository and update the living `FEATURES.md` ledger to match.

## Parameters

- Hint: $ARGUMENTS. It can be a feature name (match it against existing entries), a short change summary, or a git range such as `main..HEAD` or `HEAD~3`. If empty, use the working tree and, when the tree is clean, the last commit.

## Ground rules

- Edit only `FEATURES.md`. Do not change code, and do not commit or push. Leave that to the user.
- Document behaviour you verified by reading the changed code, not what a commit message claims.
- Never write a secret, token or private URL into the ledger.
- Keep entries short and factual. The ledger describes what the system does and where, not how the diff went.

## Protocol

1. **Detect changes.**
   - Run `git status --short`, `git diff HEAD` and `git diff --staged`. Untracked files appear in status only, so read them too.
   - Tree clean, or a range was given: use `git diff <range>` or `git show --stat HEAD`.
   - Outside git: use files modified in this session.
   - Skip changes with no behavioural or architectural effect: formatting, comments, typos, and test-only edits that add no new rule. Say so if nothing is left.
   - Treat these as ledger-worthy even when small: schema or migration changes, new or changed endpoints, config or env knobs, new background jobs, changed limits or retention, new dependencies, deploy or CI changes.

2. **Open `FEATURES.md`.** If it does not exist, create it with the heading `# Features & Architecture Ledger`, then add entries for the changed subsystems only, and suggest `/onboard` to seed the rest.

3. **Match and update.** For each changed area:
   - Belongs to an existing entry: update that entry in place. Set `Status: Modified`, refresh file paths, summary, dependencies and trade-offs, and set `Last Updated` to today's date and `git rev-parse --short HEAD`. If the entry is already `Modified` or `Active`, do not stack history: rewrite it to describe the current state.
   - New capability: append a new entry with `Status: Active`.
   - Code removed or replaced: set `Status: Deprecated`, say what replaces it, and keep the entry. Never delete an entry. If a file path in an entry no longer exists, fix or annotate it.
   - A change spans several subsystems: update each one. Do not invent a catch-all entry.

4. **Verify before saving.** Every path in a touched entry must exist (or be marked removed). Every table, column, endpoint or env var named must appear in the code. An uncommitted change is stamped with the current HEAD hash plus the words `(uncommitted)`, so the stamp is not mistaken for a committed state.

5. **Entry template.**

```markdown
### [Feature / Subsystem Name]
- **Status**: Active | Modified | Deprecated
- **Last Updated**: YYYY-MM-DD (short commit hash)
- **Entry Points / Key Files**:
  - `path/to/entry.ext`: router or handler
  - `path/to/service.ext`: business logic
  - `path/to/schema.ext`: data model
- **Implementation Summary**: Plain-language description of how it works internally (2 to 5 sentences).
- **Data & Schema Dependencies**: Tables, collections, external APIs, queues, or caches touched.
- **Production Trade-Offs**: Edge cases, locking and idempotency behaviour, caching rules, scaling limits, known gaps.
```

## Final response

Report in a few lines: entries added, modified and deprecated (names only), changes you skipped as non-behavioural and why, and anything you could not verify. Do not paste the whole ledger.
