---
name: skill-catalog
description: Start here. An index of external security-skill collections and how to install them. Rook ships with no hunting skills of its own - read this to learn where skills live, how to fetch a collection into the local skills directory, and how rook then loads it. Consult it before an audit whenever you need methodology or vulnerability-class playbooks you do not already have on disk.
---

# Skill catalog

Rook ships lean. The binary carries this catalog and nothing else: no hunting
playbooks, no methodology, no reporting templates. Substantive security skills
live in external collections you fetch on demand. This keeps the binary small
and the skills current - a collection is improved with a `git push`, not a rook
release.

## Where skills live

Rook loads skills from a single directory:

    ~/.config/rook/skills/

Each skill is a subdirectory holding a `SKILL.md`:

    ~/.config/rook/skills/<skill-name>/SKILL.md

Rook rescans that directory every turn, so a skill added while you work -
including one you clone yourself - is available on your next step, no restart
needed. A skill on disk overrides an embedded one of the same name.

## Installing a collection

If `~/.config/rook/skills/` is empty or missing, populate it with the `shell`
tool before you start hunting.

### Claude-BugHunter

A broad public library of bug-bounty methodology and per-vulnerability-class
hunting playbooks - SQLi/NoSQLi, XSS, SSRF, IDOR, OAuth, SAML, GraphQL, cloud
misconfiguration, business logic, and more - each built from real public
reports and CVE research.

- Repository: https://github.com/elementalsouls/Claude-BugHunter
- Author: Sachin Sharma. Licensed MIT; its content keeps its own license.

Clone it into the skills directory:

    mkdir -p ~/.config/rook/skills
    git clone https://github.com/elementalsouls/Claude-BugHunter ~/.config/rook/skills/claude-bughunter

## How to work with skills

1. Read this catalog to see which collections exist.
2. Check whether the skills directory is already populated: `ls ~/.config/rook/skills/`.
3. If it is empty and a collection above fits your objective, clone it with `shell`.
4. Browse what arrived and read the skills relevant to the task, then follow them:

       ls -R ~/.config/rook/skills/
       # read a relevant SKILL.md with the `read` tool

If a collection lays its skills out as `<name>/SKILL.md` directly under the
skills directory, rook loads and advertises them automatically after the clone -
they appear in your available-skills list on the next turn. If a collection
nests them differently (as the clone above does, under `claude-bughunter/`),
that is fine: read the files you need directly with `read`. You do not need rook
to list a skill for you to use it - the filesystem is right there.
