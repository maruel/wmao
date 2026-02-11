package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func testFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":          {Data: []byte("<html>hello</html>")},
		"index.html.br":       {Data: []byte("br-index")},
		"index.html.zst":      {Data: []byte("zst-index")},
		"index.html.gz":       {Data: []byte("gz-index")},
		"favicon.ico":         {Data: []byte("icon")},
		"assets/app.js":       {Data: []byte("console.log('hi')")},
		"assets/app.js.br":    {Data: []byte("br-app")},
		"assets/app.js.zst":   {Data: []byte("zst-app")},
		"assets/app.js.gz":    {Data: []byte("gz-app")},
		"assets/style.css":    {Data: []byte("body{}")},
		"assets/style.css.br": {Data: []byte("br-css")},
	}
}

func TestStaticHandler(t *testing.T) {
	h := newStaticHandler(testFS())

	t.Run("PrecompressedZstd", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/assets/app.js", http.NoBody)
		req.Header.Set("Accept-Encoding", "zstd, br, gzip")
		w := httptest.NewRecorder()
		h(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		if got := w.Header().Get("Content-Encoding"); got != "zstd" {
			t.Errorf("Content-Encoding = %q, want %q", got, "zstd")
		}
		if got := w.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
			t.Errorf("Content-Type = %q, want %q", got, "text/javascript; charset=utf-8")
		}
		if got := w.Body.String(); got != "zst-app" {
			t.Errorf("body = %q, want %q", got, "zst-app")
		}
	})

	t.Run("PrecompressedBrotli", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/assets/app.js", http.NoBody)
		req.Header.Set("Accept-Encoding", "br")
		w := httptest.NewRecorder()
		h(w, req)

		if got := w.Header().Get("Content-Encoding"); got != "br" {
			t.Errorf("Content-Encoding = %q, want %q", got, "br")
		}
		if got := w.Body.String(); got != "br-app" {
			t.Errorf("body = %q, want %q", got, "br-app")
		}
	})

	t.Run("PrecompressedGzip", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/assets/app.js", http.NoBody)
		req.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()
		h(w, req)

		if got := w.Header().Get("Content-Encoding"); got != "gzip" {
			t.Errorf("Content-Encoding = %q, want %q", got, "gzip")
		}
		if got := w.Body.String(); got != "gz-app" {
			t.Errorf("body = %q, want %q", got, "gz-app")
		}
	})

	t.Run("FallbackOriginal", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/favicon.ico", http.NoBody)
		w := httptest.NewRecorder()
		h(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		if got := w.Header().Get("Content-Encoding"); got != "" {
			t.Errorf("Content-Encoding = %q, want empty", got)
		}
		if got := w.Body.String(); got != "icon" {
			t.Errorf("body = %q, want %q", got, "icon")
		}
	})

	t.Run("SPAFallback", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/some/deep/route", http.NoBody)
		req.Header.Set("Accept-Encoding", "br")
		w := httptest.NewRecorder()
		h(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		if got := w.Header().Get("Content-Encoding"); got != "br" {
			t.Errorf("Content-Encoding = %q, want %q", got, "br")
		}
		if got := w.Body.String(); got != "br-index" {
			t.Errorf("body = %q, want %q", got, "br-index")
		}
	})

	t.Run("VaryHeader", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/favicon.ico", http.NoBody)
		w := httptest.NewRecorder()
		h(w, req)

		if got := w.Header().Get("Vary"); got != "Accept-Encoding" {
			t.Errorf("Vary = %q, want %q", got, "Accept-Encoding")
		}
	})

	t.Run("CacheControlAssets", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/assets/app.js", http.NoBody)
		w := httptest.NewRecorder()
		h(w, req)

		if got := w.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
			t.Errorf("Cache-Control = %q, want immutable", got)
		}
	})

	t.Run("CacheControlRoot", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		w := httptest.NewRecorder()
		h(w, req)

		if got := w.Header().Get("Cache-Control"); got != "no-cache" {
			t.Errorf("Cache-Control = %q, want %q", got, "no-cache")
		}
	})

	t.Run("PreferenceOrder", func(t *testing.T) {
		// Only br variant for style.css (no zst or gz).
		req := httptest.NewRequest(http.MethodGet, "/assets/style.css", http.NoBody)
		req.Header.Set("Accept-Encoding", "zstd, br, gzip")
		w := httptest.NewRecorder()
		h(w, req)

		if got := w.Header().Get("Content-Encoding"); got != "br" {
			t.Errorf("Content-Encoding = %q, want %q", got, "br")
		}
		if got := w.Body.String(); got != "br-css" {
			t.Errorf("body = %q, want %q", got, "br-css")
		}
	})

	t.Run("RootServesIndex", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", http.NoBody)
		w := httptest.NewRecorder()
		h(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		body, _ := io.ReadAll(w.Body)
		if string(body) != "<html>hello</html>" {
			t.Errorf("body = %q, want index.html content", string(body))
		}
	})
}

func TestParseAcceptEncoding(t *testing.T) {
	tests := []struct {
		header string
		want   map[string]bool
	}{
		{"gzip, br", map[string]bool{"gzip": true, "br": true}},
		{"zstd;q=1.0, gzip;q=0.5", map[string]bool{"zstd": true, "gzip": true}},
		{"", map[string]bool{}},
		{"identity", map[string]bool{"identity": true}},
	}
	for _, tt := range tests {
		t.Run(tt.header, func(t *testing.T) {
			got := parseAcceptEncoding(tt.header)
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("parseAcceptEncoding(%q)[%q] = %v, want %v", tt.header, k, got[k], v)
				}
			}
			if len(got) != len(tt.want) {
				t.Errorf("parseAcceptEncoding(%q) has %d entries, want %d", tt.header, len(got), len(tt.want))
			}
		})
	}
}
