module github.com/chatbotkit/rook

go 1.21

// @note pin the build toolchain to a stdlib-patched Go (GO-2026-5037 /
// GO-2026-5039 are fixed in go1.26.4). Bump as future stdlib advisories land.
toolchain go1.26.4

require (
	github.com/chatbotkit/go-sdk v0.4.0
	github.com/joho/godotenv v1.5.1
	github.com/spf13/pflag v1.0.5
)

require gopkg.in/yaml.v3 v3.0.1
