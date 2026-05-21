package router

import (
	"bytes"
	"net/http"
)

// ResponseRecorder 捕获 HTTP 响应，以便我们可以检查并在需要时重试
type ResponseRecorder struct {
	Code      int
	HeaderMap http.Header
	Body      *bytes.Buffer
}

func NewResponseRecorder() *ResponseRecorder {
	return &ResponseRecorder{
		Code:      200,
		HeaderMap: make(http.Header),
		Body:      new(bytes.Buffer),
	}
}

func (rw *ResponseRecorder) Header() http.Header {
	return rw.HeaderMap
}

func (rw *ResponseRecorder) Write(buf []byte) (int, error) {
	return rw.Body.Write(buf)
}

func (rw *ResponseRecorder) WriteHeader(code int) {
	if rw.Code == 200 { // 仅设置一次，类似实际 http.ResponseWriter
		rw.Code = code
	}
}
