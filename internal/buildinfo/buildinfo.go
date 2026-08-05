// Package buildinfo reports which kind of binary this is.
//
// Rook is an autonomous offensive-security agent: it runs shell commands
// against targets with a provider key in the same process. The difference
// between a developer's build and a released one is therefore a security
// boundary, not a convenience. The one thing that turns on it is whether a
// `.env` in the working directory is read - handy while developing, and
// unacceptable in a released binary, because pointing Rook at a target's
// checkout would otherwise be enough to load credentials out of it.
//
// The switch is a build tag (`-tags dev`) that defaults to off. A release built
// without the version ldflags still looks like a release; a build that forgets
// `-tags dev` merely loses a developer convenience. The failure mode of
// forgetting has to be the safe one.
//
// Named buildinfo rather than build because a bare `build` directory is caught
// by the monorepo's root .gitignore.
package buildinfo
