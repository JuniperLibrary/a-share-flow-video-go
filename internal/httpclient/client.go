// Package httpclient 提供全项目共享的 HTTP 客户端。
//
// 背景：此前多数调用点直接写 &http.Client{Timeout: ...}，这会连带创建一个新的
// http.Transport。而 Transport 才是连接池的持有者，每次请求都新建一个意味着
// TCP/TLS 握手无法复用，高频调用时会堆积大量 TIME_WAIT 端口——macOS 上尤其明显，
// 项目为此不得不在连续 LLM 调用之间加 sleep 节流来规避端口耗尽。
//
// 这里统一复用同一个 Transport，各调用点只按自身语义设置单次请求总超时。
package httpclient

import (
	"net"
	"net/http"
	"time"
)

// sharedTransport 是全局唯一的连接池持有者，被所有客户端复用。
var sharedTransport = &http.Transport{
	Proxy: http.ProxyFromEnvironment,
	DialContext: (&net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}).DialContext,
	MaxIdleConns: 100,
	// 默认值仅为 2；对东财这类单域名接口，并发请求稍多就会被强制新建连接。
	MaxIdleConnsPerHost:   16,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ResponseHeaderTimeout: 20 * time.Second,
	ExpectContinueTimeout: 2 * time.Second,
}

// New 返回复用共享连接池的客户端，timeout 为单次请求的总超时。
func New(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: sharedTransport,
		Timeout:   timeout,
	}
}
