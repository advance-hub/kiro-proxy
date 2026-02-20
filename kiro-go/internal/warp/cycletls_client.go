package warp

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/Danny-Dasilva/CycleTLS/cycletls"
)

// CycleTLSClient 使用 CycleTLS 模拟 Chrome 浏览器的 TLS 指纹
type CycleTLSClient struct {
	client cycletls.CycleTLS
}

// NewCycleTLSClient 创建新的 CycleTLS 客户端
func NewCycleTLSClient() *CycleTLSClient {
	return &CycleTLSClient{
		client: cycletls.Init(),
	}
}

// Do 执行 HTTP 请求，模拟 Chrome 的 TLS 指纹
func (c *CycleTLSClient) Do(req *http.Request) (*http.Response, error) {
	// 读取请求体
	var bodyBytes []byte
	if req.Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
		req.Body.Close()
	}

	// 构建 headers
	headers := make(map[string]string)
	for k, v := range req.Header {
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	fmt.Printf("[CycleTLS] 发送请求: %s %s (body: %d bytes)\n", req.Method, req.URL.String(), len(bodyBytes))
	fmt.Printf("[CycleTLS] JA3: 771,4866-4865-4867-49196-49195-52393-49200-49199-52392,11-45-5-0-43-10-35-16-23-13-51-65281,4588-29-23-24,0\n")

	// 使用 CycleTLS 发送请求，使用 warp2api-full 验证过的 JA3 指纹
	resp, err := c.client.Do(req.URL.String(), cycletls.Options{
		Body:            string(bodyBytes),
		Headers:         headers,
		UserAgent:       headers["User-Agent"],
		Ja3:             "771,4866-4865-4867-49196-49195-52393-49200-49199-52392,11-45-5-0-43-10-35-16-23-13-51-65281,4588-29-23-24,0", // Warp 验证通过的 JA3
		DisableRedirect: true,
		Timeout:         120,
		OrderAsProvided: true,
	}, req.Method)

	if err != nil {
		fmt.Printf("[CycleTLS] 请求失败: %v\n", err)
		return nil, fmt.Errorf("cycletls request failed: %w", err)
	}

	fmt.Printf("[CycleTLS] 响应状态: %d\n", resp.Status)

	// 转换为标准 http.Response
	httpResp := &http.Response{
		Status:     fmt.Sprintf("%d %s", resp.Status, http.StatusText(resp.Status)),
		StatusCode: resp.Status,
		Proto:      "HTTP/1.1",
		ProtoMajor: 1,
		ProtoMinor: 1,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(resp.Body)),
		Request:    req,
	}

	// 复制响应头
	for k, v := range resp.Headers {
		httpResp.Header.Set(k, v)
	}

	return httpResp, nil
}

// Close 关闭 CycleTLS 客户端
func (c *CycleTLSClient) Close() {
	c.client.Close()
}

// DoWithBody 执行带 body 的 POST 请求
func (c *CycleTLSClient) DoWithBody(url string, headers map[string]string, body []byte) (*http.Response, error) {
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return c.Do(req)
}
