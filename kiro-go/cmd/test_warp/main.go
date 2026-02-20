package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"time"
	"crypto/tls"
	"golang.org/x/net/http2"

	"kiro-go/internal/warp"
)

func main() {
	token := os.Getenv("WARP_TOKEN")

	messages := []json.RawMessage{
		json.RawMessage(`{"role":"user","content":"Say hi in one word"}`),
	}
	protoBody := warp.BuildWarpRequest("claude-4-sonnet", messages, "", nil, "/tmp", "/tmp")

	grpcBody := make([]byte, 5+len(protoBody))
	grpcBody[0] = 0
	binary.BigEndian.PutUint32(grpcBody[1:5], uint32(len(protoBody)))
	copy(grpcBody[5:], protoBody)

	jar, _ := cookiejar.New(nil)
	transport := &http2.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
	client := &http.Client{Timeout: 120 * time.Second, Transport: transport, Jar: jar}

	// Step 1: Call /client/login to get session cookie
	fmt.Println("=== Step 1: /client/login ===")
	loginReq, _ := http.NewRequest("POST", "https://app.warp.dev/client/login", nil)
	loginReq.Header.Set("Authorization", "Bearer "+token)
	loginReq.Header.Set("x-warp-client-id", "warp-app")
	loginReq.Header.Set("x-warp-client-version", "v0.2026.02.11.08.23.stable_02")
	loginResp, err := client.Do(loginReq)
	if err != nil {
		fmt.Printf("Login error: %v\n", err)
		return
	}
	loginResp.Body.Close()
	fmt.Printf("Login: HTTP %d, cookies: %v\n", loginResp.StatusCode, jar.Cookies(loginReq.URL))

	// Step 2: /ai/multi-agent with cookie
	fmt.Println("\n=== Step 2: /ai/multi-agent with cookie ===")
	req, _ := http.NewRequest("POST", "https://app.warp.dev/ai/multi-agent", bytes.NewReader(grpcBody))
	req.Header.Set("Content-Type", "application/grpc")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("x-warp-client-id", "warp-app")
	req.Header.Set("x-warp-client-version", "v0.2026.02.11.08.23.stable_02")
	req.Header.Set("x-warp-os-category", "macOS")
	req.Header.Set("x-warp-os-name", "macOS")
	req.Header.Set("x-warp-os-version", "15.7.2")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	defer resp.Body.Close()

	grpcStatus := resp.Header.Get("Grpc-Status")
	grpcMsg := resp.Header.Get("Grpc-Message")
	fmt.Printf("HTTP %d, grpc-status: %s\n", resp.StatusCode, grpcStatus)
	
	if grpcStatus == "0" || grpcStatus == "" {
		// Success! Read response
		fmt.Println("SUCCESS! Reading gRPC stream...")
		buf := make([]byte, 8192)
		totalRead := 0
		for {
			n, err := resp.Body.Read(buf[totalRead:])
			if n > 0 {
				totalRead += n
				fmt.Printf("Read %d bytes (total: %d)\n", n, totalRead)
				// Parse gRPC frames from accumulated buffer
				pos := 0
				for pos+5 <= totalRead {
					msgLen := int(binary.BigEndian.Uint32(buf[pos+1:pos+5]))
					if pos+5+msgLen > totalRead {
						break // incomplete frame
					}
					msgData := buf[pos+5:pos+5+msgLen]
					events := warp.ParseWarpResponseEvent(msgData)
					for _, ev := range events {
						text := ev.Text
						if len(text) > 100 { text = text[:100] }
						fmt.Printf("  Event: type=%s", ev.Type)
						if text != "" { fmt.Printf(" text=%q", text) }
						fmt.Println()
					}
					pos += 5+msgLen
				}
			}
			if err != nil {
				if err != io.EOF {
					fmt.Printf("Read error: %v\n", err)
				}
				break
			}
		}
		fmt.Printf("Total: %d bytes\n", totalRead)
	} else {
		if len(grpcMsg) > 100 { grpcMsg = grpcMsg[:100] }
		fmt.Printf("grpc-message: %s\n", grpcMsg)
	}
}
