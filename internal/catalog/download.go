package catalog

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// statusError reports a non-2xx HTTP response.
type statusError struct {
	code int
	url  string
	msg  string
}

func (e *statusError) Error() string {
	if e.msg != "" {
		return fmt.Sprintf("%s: %s (%s)", e.url, http.StatusText(e.code), e.msg)
	}
	return fmt.Sprintf("%s: %s", e.url, http.StatusText(e.code))
}

func isNotFound(err error) bool {
	var se *statusError
	return err != nil && errors.As(err, &se) && se.code == http.StatusNotFound
}

// get performs a GET request and returns the body, erroring on a
// non-2xx response. Bodies are capped at 8 MiB, which covers every
// endpoint used here except the WowInterface filelist (streamed
// separately in wowinterface.go).
func get(ctx context.Context, client *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "wowfix")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, &statusError{code: resp.StatusCode, url: url, msg: strings.TrimSpace(string(body))}
	}
	return body, nil
}

// downloadFile streams url into dest, reporting progress through
// progress (done bytes, total bytes; total may be 0 when unknown).
func downloadFile(ctx context.Context, client *http.Client, url, dest string, progress func(done, total int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "wowfix")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &statusError{code: resp.StatusCode, url: url, msg: resp.Status}
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer func() { _ = out.Close() }()

	total := resp.ContentLength
	if total < 0 {
		total = 0
	}
	var w io.Writer = out
	if progress != nil {
		w = &progressWriter{w: out, progress: progress, total: total}
	}
	if _, err := io.Copy(w, resp.Body); err != nil {
		return err
	}
	if progress != nil && total > 0 {
		progress(total, total) // final flush so UIs reach 100%
	}
	return nil
}

// progressWriter forwards written bytes to the progress callback.
type progressWriter struct {
	w        io.Writer
	progress func(done, total int64)
	total    int64
	done     int64
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	p.done += int64(n)
	if p.progress != nil {
		p.progress(p.done, p.total)
	}
	return n, err
}
