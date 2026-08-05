//go:build dev

package buildinfo

// Dev reports whether this is a developer build. True only under `-tags dev`.
const Dev = true

// Kind names the build, for the version line.
const Kind = "dev"
