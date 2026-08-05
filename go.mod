module github.com/pdparchitect/rook

// @note pinned to a stdlib-patched Go. Advisories land against the standard
// library regularly and govulncheck gates CI, so this tracks the current patch
// release rather than a floating minor.
go 1.26.5

require (
	github.com/joho/godotenv v1.5.1
	github.com/openzot/openzot v0.7.0
	github.com/spf13/pflag v1.0.5
)

require gopkg.in/yaml.v3 v3.0.1

require (
	github.com/dlclark/regexp2/v2 v2.5.1 // indirect
	github.com/tiktoken-go/tokenizer v0.8.1 // indirect
)
