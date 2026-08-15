# Mnemo — Setup Guide (for AI Agents)

You are being asked to set up **Mnemo**, a Git-native memory server for AI
agents, inside a project repository. This guide is written for you — an AI
agent — so you can complete the setup without a human doing manual steps.

**Source of truth:** this file lives in the Mnemo repository. The setup is
fully local: everything is stored inside the target project repo, and binary
artifacts are gitignored so they never get committed.

---

## Ask the user before starting (mandatory)

Do not touch anything until the human answers these. Especially important for
**existing** projects, where conventions may already exist:

1. **New project or existing one?** Existing: check for an existing
   `.memory_config.yaml`, `.mcp.json`, and `.gitignore` conventions first —
   merge, never overwrite.
2. **How should memory be saved?** Default Mnemo layout (`.mnemo/` +
   `.memory_config.yaml`) or custom paths? Which categories fit the project?
3. **What categories/tags should be allowed?** Propose defaults, get approval —
   never invent tags.
4. **Binary location?** Default: saved at `.mnemo/mnemo` inside the
   project (gitignored). Alternative: on PATH.
5. **Commit strategy?** Separate memory commits, or bundled with feature work?
6. **Existing project extras:** where should the `mnemo` entry merge into
   `.mcp.json`, and does the team already have an agent rules file
   (`AGENTS.md`/`CLAUDE.md`) to update instead of creating `agent.md`?

Record the answers; they are item 0 of your final report.

---

## What you're building

In the target project you will create:

```
project/
├── .memory_config.yaml    # Mnemo config — COMMIT ME
├── .mnemo/           # memory directory
│   ├── memory_log.jsonl   # append-only audit log — COMMIT ME
│   ├── docs/              # markdown memory docs — COMMIT ME
│   ├── index.db           # semantic index — IGNORED (rebuildable)
│   ├── index.db-shm       # SQLite WAL sidecar — IGNORED
│   ├── index.db-wal       # SQLite WAL sidecar — IGNORED
│   ├── mnemo              # the executable binary — IGNORED
│   └── models/            # embedding model — IGNORED (redownloads)
├── .mcp.json              # MCP client registration (if supported)
├── agent.md               # copied from AGENT-TEMPLATE.md — COMMIT ME
└── .gitignore             # gets entries appended by --init
```

**Commit the log, docs, config, and agent.md. Never commit index.db or its
-shm/-wal sidecars, the models directory, or the mnemo binary.**

---

## Prerequisites

Check, in order:

1. **Go 1.26+** available? → `go version`. If yes, build from source (step A).
2. **Prebuilt binary** reachable? → `which mnemo`. If yes, skip to step B.
3. Both missing → **stop and report**: the human must install Go or provide a
   release binary. Do not fake-install.

## Step A — Install the binary

```sh
# Installs as the binary "ai-shared-memory" (module base name)
go install github.com/Guuzzeji/ai-shared-memory@latest
```

If `go install` fails (no network, module proxy blocked), report it and stop —
do not attempt a workaround build.

Verify the binary runs:

```sh
ai-shared-memory --help
```

If `ai-shared-memory` is not on your PATH after install (common with custom
GOPATH), note the install path from `go env GOPATH` and use that absolute path
in step D. Alternative: the project-local build `go build -o .mnemo/mnemo .`
keeps the `mnemo` name.

## Step B — Initialize the target project

```sh
cd <project-root>
mnemo --init
```

`--init` creates `.memory_config.yaml`, `.mnemo/` + `.mnemo/docs/`,
and appends these lines to `.gitignore`:

```
.mnemo/index.db
.mnemo/index.db-shm
.mnemo/index.db-wal
.mnemo/mnemo
.mnemo/models/
.sisyphus/
```

**Verify the scaffold:**

```sh
ls -la .mnemo/          # must show docs/ (and later log + db)
cat .memory_config.yaml      # paths must point into .mnemo/
grep -E "mnemo" .gitignore   # ignore entries present
```

If the project **already has** `.memory_config.yaml`, `--init` will refuse.
That means Mnemo is already set up — skip to step C and verify instead of
re-initializing.

## Step C — Tailor the config to the project

Edit `.memory_config.yaml`:

- `taxonomy.allowed_categories` — replace with categories that fit this
  project (e.g. `[api, frontend, infra, data]`). This is a **closed set**:
  agents can only tag memory with these.
- `taxonomy.key_terms` — optional query synonyms. Add project jargon
  (product names, internal abbreviations).

Do **not** change `dimensions` or `model_repo` — they must match the index.

## Step D — Register the MCP server

Add a project-scoped MCP config so agents in this repo can use Mnemo. The
cross-client convention is a `.mcp.json` at the project root:

```json
{
  "mcpServers": {
    "mnemo": {
      "command": "<absolute path to mnemo binary>",
      "args": []
    }
  }
}
```

If the project already has a `.mcp.json` (or `.cursor/mcp.json`), **merge**
the `mnemo` entry into it — do not overwrite existing servers.

If the client does not support `.mcp.json`, report the binary path to the
human with instructions to register it manually (e.g.
`claude mcp add --scope project mnemo -- /abs/path/mnemo`).

## Step E — Copy the agent rules file

Copy the agent usage template from the Mnemo repo into the project root:

```sh
cp <path-to-mnemo-repo>/AGENT-TEMPLATE.md <project-root>/agent.md
```

(Or `AGENTS.md` if the project's tooling auto-loads that name.)

## Step F — Verify end-to-end

1. Start the server once so it downloads the model and builds the index
   (first boot may take minutes; allow it):

   ```sh
   mnemo --config .memory_config.yaml
   ```
   Stop it after startup completes (Ctrl-C). If the model download fails, the
   server still runs in degraded mode — that is expected, not an error.

2. Confirm the index was created:

   ```sh
   ls .mnemo/index.db
   ```

3. If you have MCP tool access in this session, confirm the tools are
   registered (`semantic_search`, `append_memory`, `get_memory`,
   `list_memories`).

## Step G — Commit

Stage and commit (following the project's existing commit style):

```sh
git add .memory_config.yaml .mnemo .gitignore .mcp.json agent.md
git commit -m "feat: add Mnemo AI memory system"
```

**Do NOT `git add`** `.mnemo/index.db` or `.mnemo/models/` — verify
`git status` shows them untracked/ignored before committing.

---

## Guardrails (mandatory)

- **Everything local.** No cloud services, no external memory stores.
- **Binaries stay out of git.** `index.db`, `index.db-shm`, `index.db-wal`,
  `models/`, and the `mnemo` binary (`.mnemo/mnemo`) are ignored. If
  `git status` shows them as staged, unstage.
- **Do not edit `.mnemo/` files by hand** during setup beyond what this
  guide says. The log and docs are written by the Mnemo MCP tools at runtime.
- **Do not invent tags** in the config; only use categories the human approves.
- **If anything in steps A–F fails**, stop and report the exact error. Do not
  improvise alternative setups.

---

## Report format

When done, report:

0. Answers to the setup questions (new/existing, paths, categories, binary
   location, commit strategy, merge targets)
1. Binary location (or build method)
2. What was created in the project (files + gitignore entries)
3. Whether `.mcp.json` was merged/created or the human must register manually
4. First-boot status: index built, or degraded mode (model unavailable)
5. Anything that needs a human decision (categories chosen, agent.md name)