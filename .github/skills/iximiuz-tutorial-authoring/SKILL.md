---
name: iximiuz-tutorial-authoring
description: 'Create and edit tutorials for https://labs.iximiuz.com. Use when asked to author an iximiuz Labs tutorial, prepare tutorial front matter, define playground tabs and tasks, and structure markdown with iximiuz MDC components such as image-box, details-box, hint-box, simple-task, and user-input-task.'
---

# Iximiuz Tutorial Authoring

Create production-ready tutorial content for iximiuz Labs, based on platform rules and local project examples.

## When to Use This Skill

- User asks to create a tutorial for iximiuz Labs
- User asks to prepare or fix `index.md` for tutorial content
- User needs front matter with playground/tasks/challenges metadata
- User needs iximiuz MDC components (`image-box`, `details-box`, `hint-box`, `tabbed`, `card`, `grid`, `simple-task`, `user-input-task`)
- User wants to convert existing docs into an interactive labs tutorial

## Canonical References

- Platform sample/tutorial rules: `https://labs.iximiuz.com/tutorials/sample-tutorial`
- Local working example: `docs/iximiuz/deep-in-memory-tutorial-ac080898/index.md`

## Required Content Structure

Use this layout for each tutorial:

```text
<tutorial-name>/
├── index.md
├── <optional helper files>
└── __static__/
    ├── cover.png
    └── other-images.png
```

Rules:

- `index.md` starts with YAML front matter and `kind: tutorial`
- `__static__` is public; never store secrets there
- Non-static helper files can live next to `index.md` for author-only use

## Front Matter Checklist

Minimum required fields:

```yaml
---
kind: tutorial
title: <10-120 chars>
description: |
  <up to 500 chars>
categories:
  - linux
tagz:
  - <tag>
createdAt: YYYY-MM-DD
cover: __static__/cover.png
---
```

Use and validate:

- `categories`: max 2 from the platform list (linux, networking, containers, kubernetes, programming, observability, security, ci-cd)
- `tagz`: max 5
- `createdAt`: required (`YYYY-MM-DD` or datetime variant)
- `updatedAt`: optional
- `playground`, `tasks`, `challenges`, `tutorials`, `skill-paths`, `playgrounds`: optional and useful for interactive content

## Authoring Workflow

1. Define tutorial scope
- Topic, target audience, expected outcomes
- What user can do after finishing

2. Create front matter first
- Fill required metadata
- Attach a playground if the tutorial has hands-on steps
- Define init/helper/regular tasks when progress checks are needed

3. Write content in learning flow
- Explain context and goals
- Add short command blocks and expected outcomes
- Use section-based progression with clear transitions

4. Add interactive navigation
- Use `:tab{...}` links to guide users between playground tabs
- Use `:http-port{...}` when users need web UI access

5. Add validation tasks
- `init` tasks for environment setup
- Regular tasks with `::simple-task` or `::user-input-task` in markdown
- Use `needs`, `hintcheck`, and `failcheck` for progressive guidance

6. Add visuals and side content
- Use `::image-box` for diagrams and screenshots
- Use `::details-box` and `::hint-box` for optional or staged guidance
- Use `::remark-box` for warnings and important constraints

7. Validate and harden
- Ensure all commands are reproducible in playground
- Verify tabs/machines names match front matter
- Ensure each user-facing task has matching UI block
- Ensure no sensitive info is inside `__static__`

## Tutorial Writing Standards

- Keep sections concise and actionable
- Prefer command + explanation + expected result pattern
- Use explicit machine/tab references for every command
- Minimize theory-only paragraphs without runnable steps
- Keep examples deterministic and testable

## Task Patterns

### Init task in front matter

```yaml
tasks:
  init_setup:
    init: true
    machine: dev-machine
    user: laborant
    run: |
      echo "setup"
```

### Regular verification task + UI block

```yaml
tasks:
  verify_file_exists:
    machine: dev-machine
    run: |
      test -f /tmp/example.txt
```

```md
::simple-task
---
:tasks: tasks
:name: verify_file_exists
---
#active
Create `/tmp/example.txt` on `dev-machine`.

#completed
Great, file detected.
::
```

## Recommended Final QA

- Front matter fields valid and complete
- Playground starts and tasks execute
- No broken image paths from `__static__`
- No unresolved references in cards (`challenges.*`, `tutorials.*`, etc.)
- Narrative is understandable without author-only context

## Local Example Mapping

When creating tutorials in this repository, align with style and structure from:

- `docs/iximiuz/deep-in-memory-tutorial-ac080898/index.md`

Use it as a reference for:

- Full front matter including playground tabs and tasks
- Command-oriented sections
- Use of `image-box`, `details-box`, and `hint-box`

## Optional CLI Workflow (for publication)

```bash
labctl auth login
labctl content pull tutorial <tutorial-name>
labctl content push -fw tutorial <tutorial-name>
```

Use live push during editing to validate rendering and playground behavior continuously.