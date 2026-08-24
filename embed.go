// Package rook bundles Rook's built-in skill catalog into the executable. The
// binary ships lean: the only embedded skill is a catalog that tells the agent
// where external skill collections live and how to fetch them into the local
// skills directory. Substantive skills are not compiled in - they are cloned on
// demand and loaded from disk, so the collection can grow and change without a
// rook release.
package rook

import "embed"

// SkillsFS holds the contents of the skills/ directory baked into the binary at
// build time - the bootstrap catalog, not a skill library. Each top-level
// subdirectory is one skill containing a SKILL.md front-matter document.
// Consume it via fs.Sub(SkillsFS, "skills") and agent.LoadSkillsFromFS; the
// agent fetches everything else itself.
//
//go:embed skills
var SkillsFS embed.FS

// ExampleConfigYAML is the commented starter config baked into the binary, used
// by `rook config` to seed ~/.config/rook/config.yaml when it does not exist yet.
//
//go:embed configs/rook.example.yaml
var ExampleConfigYAML []byte
