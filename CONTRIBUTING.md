# Contributing

## Local checks

Use the same credential-isolated checks required by CI:

```bash
make fmt-check
make mod-check
make vet
make test-race
```

`make fmt` and `make tidy` are developer conveniences that may modify the
working tree. The `fmt-check` and `mod-check` targets are read-only assertions
for the resulting state (although `mod-check` runs `go mod tidy` before
reporting any module diff).

Build the release-compatible native binary with an explicit version:

```bash
make build VERSION=ci-local
./asana-cli --version
```

Cross-build the four published targets with:

```bash
for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do
  GOOS=${target%/*} GOARCH=${target#*/} CGO_ENABLED=0 \
    go build -trimpath \
      -ldflags "-s -w -X github.com/thaodangspace/asana-cli/cli.version=ci-local" \
      ./cmd/asana-cli
done
```

Build the documentation site from the committed lockfile with:

```bash
npm --prefix docs ci
npm --prefix docs run build
```

The documentation build uses the Node.js version declared in `.node-version`.

## Release snapshots

Pull requests run the `release-snapshot` job. It validates `.goreleaser.yaml`,
builds without publishing, and checks the stable artifact contract:

```text
asana-cli_<version>_darwin_amd64.tar.gz
asana-cli_<version>_darwin_arm64.tar.gz
asana-cli_<version>_linux_amd64.tar.gz
asana-cli_<version>_linux_arm64.tar.gz
checksums.txt
```

The job also executes the Linux amd64 binary and validates the generated
Homebrew formula. Its short-retention `release-snapshot-debug` artifact is for
debugging only and is not a release distribution.

The checks are intentionally fail-closed: configuration errors fail at
`goreleaser check`, build/linker and formula errors fail during the snapshot,
mutating hooks fail the clean-tree check, and missing targets or checksums fail
the artifact verifier.

## Tests

Tests use an in-process HTTP server and do not need an Asana token or network
access. Keep credentials explicitly cleared when invoking Go tests manually:

```bash
env ASANA_CONFIG=/dev/null ASANA_ACCESS_TOKEN= \
  ASANA_DEFAULT_WORKSPACE= ASANA_API_BASE= go test -count=1 ./...
```
