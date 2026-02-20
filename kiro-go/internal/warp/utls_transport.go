package warp

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

// newUTLSTransport 创建使用标准 TLS 但参数更真实的 HTTP Transport
func newUTLSTransport() *http.Transport {
	return &http.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			// 建立 TCP 连接
			dialer := &net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}

			conn, err := dialer.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}

			// 使用标准 TLS
			tlsConn := tls.Client(conn, &tls.Config{
				ServerName: extractHostname(addr),
				MinVersion: tls.VersionTLS12,
				MaxVersion: tls.VersionTLS12, // 限制到 TLS 1.2
				CipherSuites: []uint16{
					tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
					tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
					tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
					tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
				},
				CurvePreferences: []tls.CurveID{
					tls.X25519,
					tls.CurveP256,
					tls.CurveP384,
				},
				// 禁用 HTTP/2 协商
				NextProtos:         []string{"http/1.1"},
				InsecureSkipVerify: false,
			})

			// 执行 TLS 握手
			if err := tlsConn.Handshake(); err != nil {
				conn.Close()
				return nil, err
			}

			return tlsConn, nil
		},
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		ForceAttemptHTTP2:   false, // 强制 HTTP/1.1
		// 禁用 HTTP/2
		TLSNextProto: nil,
	}
}

// extractHostname 从 "host:port" 中提取 hostname
func extractHostname(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}
