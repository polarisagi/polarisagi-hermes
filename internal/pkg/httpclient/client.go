package httpclient

import (
	"context"
	"net"
	"net/http"
	"time"
)

// SharedTransport 全局共享的 HTTP Transport，避免高并发下 TCP 连接膨胀
var SharedTransport = &http.Transport{
	MaxIdleConns:          200,
	MaxIdleConnsPerHost:   50,
	MaxConnsPerHost:       0, // 0 = 不限并发连接，仅 idle 池有上限
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
	ResponseHeaderTimeout: 600 * time.Second,
	ForceAttemptHTTP2:     true,
	DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
		dialer := &net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}
		return dialer.DialContext(ctx, network, addr)
	},
}

// Client 全局统一的 HTTP 客户端
var Client = &http.Client{
	Timeout:   600 * time.Second,
	Transport: SharedTransport,
}
