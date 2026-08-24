package version

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIsDev(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{"dev", true},
		{"", true},
		{"v0.1.0", false},
		{"0.1.0", false},
	}
	for _, tt := range tests {
		Version = tt.version
		if got := IsDev(); got != tt.want {
			t.Errorf("IsDev() with Version=%q = %v, want %v", tt.version, got, tt.want)
		}
	}
	Version = "dev" // reset
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"v1.2.3", "1.2.3"},
		{"1.2.3", "1.2.3"},
		{" v0.1.0 ", "0.1.0"},
	}
	for _, tt := range tests {
		if got := normalize(tt.input); got != tt.want {
			t.Errorf("normalize(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestIsNewer(t *testing.T) {
	tests := []struct {
		current string
		latest  string
		want    bool
	}{
		{"v0.1.0", "v0.2.0", true},
		{"v0.2.0", "v0.1.0", false},
		{"v0.1.0", "v0.1.0", false},
		{"v0.9.0", "v0.10.0", true},
		{"v1.0.0", "v0.9.0", false},
		{"v1.0.0", "v1.0.1", true},
		{"v1.9.9", "v2.0.0", true},
		{"", "v0.1.0", false},
		{"v0.1.0", "", false},
		{"invalid", "v1.0.0", false},
	}
	for _, tt := range tests {
		if got := IsNewer(tt.current, tt.latest); got != tt.want {
			t.Errorf("IsNewer(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.want)
		}
	}
}

func TestParseSemver(t *testing.T) {
	if got := parseSemver("1.2.3"); got == nil || got[0] != 1 || got[1] != 2 || got[2] != 3 {
		t.Errorf("parseSemver(1.2.3) = %v", got)
	}
	if got := parseSemver("1.2.3-beta.1"); got == nil || got[2] != 3 {
		t.Errorf("parseSemver with pre-release suffix = %v", got)
	}
	for _, bad := range []string{"", "1", "1.2", "1.2.beta"} {
		if got := parseSemver(bad); got != nil {
			t.Errorf("parseSemver(%q) = %v, want nil", bad, got)
		}
	}
}

func TestCheckSkipsDevVersion(t *testing.T) {
	Version = "dev"
	result, err := Check()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Fatal("expected nil result for dev version")
	}
}

func TestFormatUpdateNotice(t *testing.T) {
	if got := FormatUpdateNotice(nil); got != "" {
		t.Errorf("expected empty string for nil result, got %q", got)
	}

	r := &CheckResult{Current: "v0.1.0", Latest: "v0.1.0", Outdated: false}
	if got := FormatUpdateNotice(r); got != "" {
		t.Errorf("expected empty string when not outdated, got %q", got)
	}

	r = &CheckResult{
		Current:   "v0.1.0",
		Latest:    "v0.2.0",
		UpdateURL: "https://github.com/pdparchitect/rook/releases/tag/v0.2.0",
		Outdated:  true,
	}
	notice := FormatUpdateNotice(r)
	for _, want := range []string{"v0.1.0", "v0.2.0", "https://github.com/pdparchitect/rook", InstallCommand} {
		if !strings.Contains(notice, want) {
			t.Errorf("notice should contain %q, got: %q", want, notice)
		}
	}
}

func TestInstallCommandPointsAtReleaseAsset(t *testing.T) {
	for _, want := range []string{"curl", releaseRepo, "releases/latest/download/install.sh"} {
		if !strings.Contains(InstallCommand, want) {
			t.Errorf("InstallCommand should contain %q, got: %q", want, InstallCommand)
		}
	}
}

// LatestRelease queries the GitHub API. A server returning a release tag yields
// it; a server returning a non-200 status is an error.
func TestLatestReleaseParsesTag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.github+json")
		fmt.Fprint(w, `{"tag_name":"v0.7.0","html_url":"https://github.com/pdparchitect/rook/releases/tag/v0.7.0"}`)
	}))
	defer server.Close()

	original := releaseAPI
	releaseAPI = server.URL
	defer func() { releaseAPI = original }()

	tag, url, err := LatestRelease()
	if err != nil {
		t.Fatalf("LatestRelease: %v", err)
	}

	if tag != "v0.7.0" {
		t.Errorf("tag = %q, want v0.7.0", tag)
	}

	if url != "https://github.com/pdparchitect/rook/releases/tag/v0.7.0" {
		t.Errorf("url = %q", url)
	}
}

// A non-200 response is an error, not a silent empty result.
func TestLatestReleaseRejectsNon200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	original := releaseAPI
	releaseAPI = server.URL
	defer func() { releaseAPI = original }()

	if _, _, err := LatestRelease(); err == nil {
		t.Error("a 403 response must be an error")
	}
}

// A malformed JSON body is an error.
func TestLatestReleaseRejectsBadJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "not json")
	}))
	defer server.Close()

	original := releaseAPI
	releaseAPI = server.URL
	defer func() { releaseAPI = original }()

	if _, _, err := LatestRelease(); err == nil {
		t.Error("malformed JSON must be an error")
	}
}

// Check against a real (test) server: a non-dev version queries the API and
// reports whether it is outdated.
func TestCheckReportsOutdated(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.github+json")
		fmt.Fprint(w, `{"tag_name":"v0.7.0","html_url":"https://github.com/pdparchitect/rook/releases/tag/v0.7.0"}`)
	}))
	defer server.Close()

	original := releaseAPI
	releaseAPI = server.URL
	defer func() { releaseAPI = original }()

	Version = "v0.6.0"
	defer func() { Version = "dev" }()

	result, err := Check()
	if err != nil {
		t.Fatalf("Check: %v", err)
	}

	if result == nil {
		t.Fatal("expected a non-nil result for a non-dev version")
	}

	if !result.Outdated {
		t.Error("v0.6.0 should be outdated against v0.7.0")
	}

	if result.Latest != "v0.7.0" {
		t.Errorf("latest = %q, want v0.7.0", result.Latest)
	}
}

// Check reports not-outdated when the current version matches the latest.
func TestCheckReportsCurrent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/vnd.github+json")
		fmt.Fprint(w, `{"tag_name":"v0.6.0","html_url":"https://github.com/pdparchitect/rook/releases/tag/v0.6.0"}`)
	}))
	defer server.Close()

	original := releaseAPI
	releaseAPI = server.URL
	defer func() { releaseAPI = original }()

	Version = "v0.6.0"
	defer func() { Version = "dev" }()

	result, err := Check()
	if err != nil {
		t.Fatalf("Check: %v", err)
	}

	if result.Outdated {
		t.Error("v0.6.0 should not be outdated against v0.6.0")
	}
}

// Check surfaces a server error rather than swallowing it.
func TestCheckSurfacesNetworkError(t *testing.T) {
	Version = "v0.6.0"
	defer func() { Version = "dev" }()

	original := releaseAPI
	releaseAPI = "http://127.0.0.1:0/this-does-not-exist"
	defer func() { releaseAPI = original }()

	if _, err := Check(); err == nil {
		t.Error("a network error must be surfaced, not swallowed")
	}
}
