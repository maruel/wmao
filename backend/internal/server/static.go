// Precompressed static file handler for embedded frontend assets.
//
// At build time, each file in dist/ gets .br, .zst, and .gz siblings at
// maximum compression. This handler serves the best precompressed variant
// the client accepts, falling back to the original.
package server

import (
	"io"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"path/filepath"
	"strconv"
	"strings"
)

// encodingVariant maps an Accept-Encoding token to a file suffix.
type encodingVariant struct {
	encoding string // e.g. "zstd"
	suffix   string // e.g. ".zst"
}

// Ordered by preference: best first.
var staticEncodings = []encodingVariant{
	{"zstd", ".zst"},
	{"br", ".br"},
	{"gzip", ".gz"},
}

// newStaticHandler returns an http.HandlerFunc that serves precompressed
// static files from dist with SPA fallback to index.html.
func newStaticHandler(dist fs.FS) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == "/" {
			p = "/index.html"
		}
		clean := strings.TrimPrefix(path.Clean(p), "/")

		// SPA fallback: if the file doesn't exist, serve index.html.
		if _, err := fs.Stat(dist, clean); err != nil {
			clean = "index.html"
		}

		ct := mime.TypeByExtension(filepath.Ext(clean))
		if ct == "" {
			ct = "application/octet-stream"
		}

		accepted := parseAcceptEncoding(r.Header.Get("Accept-Encoding"))

		// Try precompressed variants in preference order.
		for _, v := range staticEncodings {
			if !accepted[v.encoding] {
				continue
			}
			name := clean + v.suffix
			f, err := dist.Open(name)
			if err != nil {
				continue
			}
			stat, err := f.Stat()
			if err != nil {
				_ = f.Close()
				continue
			}
			w.Header().Set("Content-Type", ct)
			w.Header().Set("Content-Encoding", v.encoding)
			w.Header().Set("Content-Length", strconv.FormatInt(stat.Size(), 10))
			w.Header().Set("Vary", "Accept-Encoding")
			setStaticCacheControl(w, clean)
			// embed.FS files implement io.ReadSeeker.
			http.ServeContent(w, r, clean, stat.ModTime(), f.(io.ReadSeeker))
			_ = f.Close()
			return
		}

		// Fallback: serve uncompressed original.
		f, err := dist.Open(clean)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		stat, err := f.Stat()
		if err != nil {
			_ = f.Close()
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Vary", "Accept-Encoding")
		setStaticCacheControl(w, clean)
		http.ServeContent(w, r, clean, stat.ModTime(), f.(io.ReadSeeker))
		_ = f.Close()
	}
}

// setStaticCacheControl sets Cache-Control for static assets. Hashed
// filenames under assets/ are immutable; everything else (index.html,
// favicon) must not be cached so deploys take effect immediately.
func setStaticCacheControl(w http.ResponseWriter, clean string) {
	if strings.HasPrefix(clean, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
}

// parseAcceptEncoding returns the set of encodings the client accepts.
func parseAcceptEncoding(header string) map[string]bool {
	accepted := make(map[string]bool)
	for part := range strings.SplitSeq(header, ",") {
		enc := strings.TrimSpace(part)
		// Strip quality parameter (e.g. "gzip;q=0.5").
		if i := strings.IndexByte(enc, ';'); i >= 0 {
			enc = enc[:i]
		}
		if enc != "" {
			accepted[enc] = true
		}
	}
	return accepted
}
