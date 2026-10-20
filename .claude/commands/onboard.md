---
description: Onboard to this codebase - architecture, schema, request tracing, system design interview prep, and a seeded FEATURES.md ledger
argument-hint: "[optional focus area, e.g. auth, payments, database]"
---

Analyse this repository and explain it the way a senior engineer would to a new teammate. Then seed `FEATURES.md`.

## Parameters

- Target: the current workspace directory.
- Focus area: $ARGUMENTS. If given, spend most of the depth on that module or flow and keep the rest to a short overview. If empty, cover the whole system evenly.

## Ground rules

- Read-only, except for creating or extending `FEATURES.md`. Do not edit code, config or other docs, and do not commit.
- Never open `.env` or other private files, and never print a secret. `.env.example` and similar templates are fine.
- Do not read every file. Start from manifests, config and entry points, then read only what a claim needs.
- Every file path, function name and command you state must come from a file you read or a command you ran. If you did not verify it, say so or leave it out.
- If `CLAUDE.md`, `README.md` or `docs/` already describe the system, read them first and build on them. Report where the code disagrees with them.

## Phase 1: Blueprint and stack

1. Language and runtime: build files and manifests (`package.json`, `go.mod`, `Cargo.toml`, `pyproject.toml`, `pom.xml`, `Gemfile`, ...). Note the language version and the few dependencies that shape the design.
2. Entry points: where the process starts and what feeds it (HTTP server, CLI, worker daemon, scheduler, message consumer).
3. Environment and infrastructure: `.env.example`, Dockerfile, compose files, CI workflows, deploy configs. List the config knobs that change behaviour (modes, limits, feature flags), not every variable.
4. How to build, run, and test, including how to run one test and what services tests need. Confirm each command against the Makefile, scripts or CI before stating it.

## Phase 2: Architecture and data

1. Topology: monolith, modular monolith, services, or an event-driven pipeline. Say which processes exist, what each owns, and how they talk.
2. Data and schema: find migrations, ORM models or schemas. Summarise the core entities, relationships, keys and indexes, and which constraints the code depends on (uniqueness, leases, full-text indexes). Note retention or pruning rules.
3. Trace one core path end to end, naming the real file and function at every hop:
   `Router/Handler` -> `Auth/Validation` -> `Business logic` -> `Storage/Cache` -> `Response`.
   If the system has a second important path (a background job, a write path, a build pipeline), trace it too, briefly.
4. Boundaries and trust: where untrusted input enters, how it is validated or escaped, who can call what, and where authentication or authorisation lives (or that it does not).

## Phase 3: Production patterns and interview prep

Use plain, language-neutral terms.

- **Caching:** what is cached, where (in-process, Redis, CDN, browser), its lifetime, and how it is invalidated.
- **Statelessness and scaling:** can several instances run behind a load balancer? Where does shared state live? What is single-instance by design?
- **Resilience:** timeouts, retries and backoff, rate limits, circuit breakers, graceful shutdown, crash recovery, idempotency, fallbacks, dead-letter handling. Only list what you found in code.
- **Observability:** logs, metrics, health checks, and what is missing.
- **Security and cost limits:** secrets handling, SSRF or injection surfaces, resource caps, free-tier or memory constraints that shaped the design.
- **System design interview prep:**
  - Name the classic interview question this system answers (for example "Design a web crawler" or "Design an idempotent payment engine").
  - Trade-offs: what was prioritised (consistency vs availability, simplicity vs horizontal scale, cost vs freshness), and what breaks first at 10x traffic or data, with the specific file or limit that breaks.
  - Three interview questions with model answers, each grounded in a named file or behaviour of this repo, not generic theory.

## Phase 4: Bootstrap FEATURES.md

1. Look for `FEATURES.md` in the project root.
   - Missing: create it with the heading `# Features & Architecture Ledger` and one line saying it is maintained by `/ledger-update`.
   - Present: do not overwrite. Add only subsystems that are missing, and fix an entry only when you have verified it is wrong.
2. Seed 5 to 15 core subsystems, one entry each, in this template. Group by what a maintainer would think of as a feature, not by directory.

```markdown
### [Feature / Subsystem Name]
- **Status**: Active
- **Last Updated**: YYYY-MM-DD (short commit hash)
- **Entry Points / Key Files**:
  - `path/to/entry.ext`: router or handler
  - `path/to/service.ext`: business logic
  - `path/to/schema.ext`: data model
- **Implementation Summary**: Plain-language description of how it works internally (2 to 5 sentences).
- **Data & Schema Dependencies**: Tables, collections, external APIs, queues, or caches touched.
- **Production Trade-Offs**: Known edge cases, locking and idempotency rules, caching behaviour, scaling limits.
```

3. Use today's date and `git rev-parse --short HEAD`. Outside git, write the date only.
4. Confirm every path in an entry exists before writing it.

## Final response

Reply with these sections, in this order, and keep each tight:

1. Stack and how to run it
2. Architecture and data model
3. The traced request path
4. Production patterns (caching, scaling, resilience)
5. Interview prep (question, trade-offs, 3 Q&As)
6. `FEATURES.md`: created or extended, with the list of entries
7. Gaps: anything you could not verify, and risks a new maintainer should know

If `CLAUDE.md` is missing or clearly out of date, say so and offer to run `/init`. Do not run it yourself.
