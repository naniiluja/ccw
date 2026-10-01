package httpapi

import (
	"compress/gzip"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// maxV1Body bounds a /v1 request, before and after it is decompressed. A call
// carries whole conversations and screenshots, so the bound is generous: it is
// there to keep one request from filling the memory, not to shape a prompt.
var maxV1Body int64 = 50 << 20

// readBody reads a /v1 request body and undoes a gzip or deflate Content-Encoding,
// so everything after it sees the JSON. It reports the status to answer with when
// the body cannot be used.
func readBody(w http.ResponseWriter, r *http.Request) ([]byte, int, error) {
	raw := http.MaxBytesReader(w, r.Body, maxV1Body)
	defer r.Body.Close()
	var src io.Reader = raw
	switch enc := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Encoding"))); enc {
	case "", "identity":
	case "gzip", "x-gzip":
		zr, err := gzip.NewReader(raw)
		if err != nil {
			return nil, http.StatusBadRequest, errors.New("cannot read a gzip body")
		}
		defer zr.Close()
		src = zr
	case "deflate":
		zr, err := zlib.NewReader(raw)
		if err != nil {
			return nil, http.StatusBadRequest, errors.New("cannot read a deflate body")
		}
		defer zr.Close()
		src = zr
	default:
		return nil, http.StatusUnsupportedMediaType, fmt.Errorf("Content-Encoding %q is not supported; send the body as plain JSON, gzip or deflate", enc)
	}
	// The limit applies to what the body becomes as well: a small compressed
	// body can inflate to far more than it is.
	body, err := io.ReadAll(io.LimitReader(src, maxV1Body+1))
	if err == nil && int64(len(body)) > maxV1Body {
		err = &http.MaxBytesError{Limit: maxV1Body}
	}
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return nil, http.StatusRequestEntityTooLarge, fmt.Errorf("the request is larger than %d MB", maxV1Body>>20)
		}
		return nil, http.StatusBadRequest, errors.New("cannot read body")
	}
	// What goes upstream is the decoded JSON, so the header that described the
	// wire form must not follow it.
	r.Header.Del("Content-Encoding")
	return body, 0, nil
}
