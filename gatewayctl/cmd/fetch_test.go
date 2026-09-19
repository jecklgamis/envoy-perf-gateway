package cmd

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchHTTPFileSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/config/cds.yaml" {
			t.Errorf("got path %s, want /config/cds.yaml", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret" {
			t.Errorf("got Authorization %q, want %q", got, "Bearer secret")
		}
		w.Write([]byte("resources: []\n"))
	}))
	defer srv.Close()

	body, found, err := fetchHTTPFile(srv.URL, "secret", "cds.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("got found=false, want true")
	}
	if string(body) != "resources: []\n" {
		t.Errorf("got body %q", body)
	}
}

func TestFetchHTTPFileNotFoundIsNotAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	body, found, err := fetchHTTPFile(srv.URL, "", "runtime.yaml")
	if err != nil {
		t.Fatalf("expected nil error for a 404, got %v", err)
	}
	if found {
		t.Error("got found=true for a 404, want false")
	}
	if body != nil {
		t.Errorf("got non-nil body %q for a 404", body)
	}
}

func TestFetchHTTPFileUnauthorizedIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	_, found, err := fetchHTTPFile(srv.URL, "wrong-token", "cds.yaml")
	if err == nil {
		t.Fatal("expected an error for a 401, got nil")
	}
	if found {
		t.Error("got found=true for a 401, want false")
	}
}

func TestFetchHTTPFileOmitsAuthHeaderWhenTokenEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := r.Header["Authorization"]; ok {
			t.Error("expected no Authorization header when apiToken is empty")
		}
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	if _, found, err := fetchHTTPFile(srv.URL, "", "cds.yaml"); err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
}

func TestFetchHTTPFileTrimsTrailingSlashFromServerURL(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	if _, _, err := fetchHTTPFile(srv.URL+"/", "", "lds.yaml"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := "/config/lds.yaml"
	if gotPath != want {
		t.Errorf("got path %q, want %q (trailing slash on server URL must not produce //)", gotPath, want)
	}
}

func TestFetchRemoteFileErrorsWhenNoModeConfigured(t *testing.T) {
	t.Setenv("CONFIG_SOURCE_KIND", "") // resolveMode() checks this before appSettings.Mode
	prev := appSettings
	appSettings.Mode = ""
	t.Cleanup(func() { appSettings = prev })

	_, _, err := fetchRemoteFile("cds.yaml")
	if err == nil {
		t.Fatal("expected an error when no mode is configured, got nil")
	}
}

func TestFetchRemoteFileErrorsOnUnsupportedMode(t *testing.T) {
	t.Setenv("CONFIG_SOURCE_KIND", "") // resolveMode() checks this before appSettings.Mode
	prev := appSettings
	appSettings.Mode = "ftp"
	t.Cleanup(func() { appSettings = prev })

	_, _, err := fetchRemoteFile("cds.yaml")
	if err == nil {
		t.Fatal("expected an error for an unsupported mode, got nil")
	}
	got := err.Error()
	wantSubstr := "unsupported mode: ftp"
	if !strings.Contains(got, wantSubstr) {
		t.Errorf("got error %q, want it to contain %q", got, wantSubstr)
	}
}
