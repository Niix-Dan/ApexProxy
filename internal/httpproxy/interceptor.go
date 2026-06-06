package httpproxy

import (
	"io"
	"net/http"
)

type bodyReadCounter struct {
	io.ReadCloser
	bytesRead int64
}

func (b *bodyReadCounter) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	b.bytesRead += int64(n)
	return n, err
}

type responseWriterInterceptor struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
}

func (w *responseWriterInterceptor) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *responseWriterInterceptor) Write(b []byte) (int, error) {
	n, err := w.ResponseWriter.Write(b)
	w.bytesWritten += int64(n)
	return n, err
}

func (w *responseWriterInterceptor) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
