# Development

The engine is the published [zot](https://github.com/openzot/openzot) module,
pinned in `go.mod`, so the repository builds from a clean clone with no extra
steps:

```bash
git clone https://github.com/pdparchitect/rook
cd rook
make            # lists the targets
make build      # build ./rook
```

`make` on its own prints the targets rather than assuming one, because Rook has
two build variants:

```bash
make build    # release binary
make dev       # developer binary - reads a .env from the working directory
make test      # run tests
make race      # tests under the race detector
make vet       # go vet over both build variants
make dist      # cross-platform release archives under dist/
```

## Install from source

```bash
go install github.com/pdparchitect/rook/cmd/rook@latest
```

## Release vs developer builds

A released binary does **not** read a `.env` from its working directory; a
developer build does. Rook runs shell commands against targets with a provider
key in the process, so a released binary must not take credentials from whatever
directory it was pointed at - a stray committed `.env` in the code under review
would otherwise reach the process about to run commands against it. The switch
is a build tag (`-tags dev`) that defaults to off; `rook --version` prints which
kind you have. See [../RELEASES.md](../RELEASES.md) for the release flow.
