// Package rook bundles Rook's built-in security skill library into the
// executable. The skills/ directory is embedded at compile time so the
// program ships as a single self-contained binary with no external skill
// files to manage or lose.
package rook

import "embed"

// SkillsFS holds the contents of the skills/ directory baked into the binary
// at build time. Each top-level subdirectory is one skill containing a
// SKILL.md front-matter document. Consume it via fs.Sub(SkillsFS, "skills")
// and agent.LoadSkillsFromFS.
//
//go:embed skills
var SkillsFS embed.FS

// ExampleConfigYAML is the commented starter config baked into the binary, used
// by `rook config` to seed ~/.config/rook/config.yaml when it does not exist yet.
//
//go:embed configs/rook.example.yaml
var ExampleConfigYAML []byte
