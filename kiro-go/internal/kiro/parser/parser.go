package parser

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
)

// AWS Event Stream 消息格式:
// ┌──────────────┬──────────────┬──────────────┬──────────┬──────────┬───────────┐
// │ Total Length │ Header Length│ Prelude CRC  │ Headers  │ Payload  │ Msg CRC   │
// │   (4 bytes)  │   (4 bytes)  │   (4 bytes)  │ (变长)    │ (变长)    │ (4 bytes) │
// └──────────────┴──────────────┴──────────────┴──────────┴──────────┴───────────┘

const (
	PreludeSize    = 12
	MinMessageSize = PreludeSize + 4 // 16
	MaxMessageSize = 16 * 1024 * 1024
)

// crc32IEEE 使用 IEEE 多项式 (与 CRC_32_ISO_HDLC 相同)
var crc32Table = crc32.MakeTable(crc32.IEEE)

func calcCRC32(data []byte) uint32 {
	return crc32.Checksum(data, crc32Table)
}

// ── Header 解析 ──

const (
	HeaderTypeBoolTrue  = 0
	HeaderTypeBoolFalse = 1
	HeaderTypeByte      = 2
	HeaderTypeShort     = 3
	HeaderTypeInteger   = 4
	HeaderTypeLong      = 5
	HeaderTypeByteArray = 6
	HeaderTypeString    = 7
	HeaderTypeTimestamp = 8
	HeaderTypeUUID      = 9
)

// Headers AWS Event Stream 消息头部
type Headers map[string]string

func (h Headers) MessageType() string { return h[":message-type"] }
func (h Headers) EventType() string   { return h[":event-type"] }
func (h Headers) ContentType() string { return h[":content-type"] }
func (h Headers) ExceptionType() string { return h[":exception-type"] }

func parseHeaders(data []byte, headerLen int) (Headers, error) {
	headers := make(Headers)
	offset := 0

	for offset < headerLen {
		if offset >= len(data) {
			break
		}
		nameLen := int(data[offset])
		offset++
		if nameLen == 0 || offset+nameLen > len(data) {
			return headers, fmt.Errorf("invalid header name length: %d", nameLen)
		}
		name := string(data[offset : offset+nameLen])
		offset += nameLen

		if offset >= len(data) {
			return headers, fmt.Errorf("unexpected end of header data")
		}
		valueType := data[offset]
		offset++

		// 解析值，只保留 String 类型的值（我们只需要 :message-type, :event-type 等）
		switch valueType {
		case HeaderTypeBoolTrue, HeaderTypeBoolFalse:
			// 无额外字节
			if valueType == HeaderTypeBoolTrue {
				headers[name] = "true"
			} else {
				headers[name] = "false"
			}
		case HeaderTypeByte:
			offset += 1
		case HeaderTypeShort:
			offset += 2
		case HeaderTypeInteger:
			offset += 4
		case HeaderTypeLong, HeaderTypeTimestamp:
			offset += 8
		case HeaderTypeByteArray:
			if offset+2 > len(data) {
				return headers, fmt.Errorf("incomplete byte array header")
			}
			vLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
			offset += 2 + vLen
		case HeaderTypeString:
			if offset+2 > len(data) {
				return headers, fmt.Errorf("incomplete string header")
			}
			vLen := int(binary.BigEndian.Uint16(data[offset : offset+2]))
			offset += 2
			if offset+vLen > len(data) {
				return headers, fmt.Errorf("incomplete string value")
			}
			headers[name] = string(data[offset : offset+vLen])
			offset += vLen
		case HeaderTypeUUID:
			offset += 16
		default:
			return headers, fmt.Errorf("unknown header value type: %d", valueType)
		}
	}
	return headers, nil
}

// ── Frame 解析 ──

// Frame 解析后的 AWS Event Stream 消息帧
type Frame struct {
	Headers Headers
	Payload []byte
}

func (f *Frame) MessageType() string { return f.Headers.MessageType() }
func (f *Frame) EventType() string   { return f.Headers.EventType() }

func (f *Frame) PayloadJSON(v interface{}) error {
	return json.Unmarshal(f.Payload, v)
}

func (f *Frame) PayloadString() string {
	return string(f.Payload)
}

// ParseFrame 尝试从缓冲区解析一个完整的帧
// 返回: frame, consumed bytes, error
// frame==nil && err==nil 表示数据不足
func ParseFrame(buf []byte) (*Frame, int, error) {
	if len(buf) < PreludeSize {
		return nil, 0, nil // 数据不足
	}

	totalLen := int(binary.BigEndian.Uint32(buf[0:4]))
	headerLen := int(binary.BigEndian.Uint32(buf[4:8]))
	preludeCRC := binary.BigEndian.Uint32(buf[8:12])

	if totalLen < MinMessageSize {
		return nil, 0, fmt.Errorf("message too small: %d < %d", totalLen, MinMessageSize)
	}
	if totalLen > MaxMessageSize {
		return nil, 0, fmt.Errorf("message too large: %d > %d", totalLen, MaxMessageSize)
	}
	if len(buf) < totalLen {
		return nil, 0, nil // 数据不足，等待更多数据
	}

	// 验证 Prelude CRC
	actualPreludeCRC := calcCRC32(buf[0:8])
	if actualPreludeCRC != preludeCRC {
		return nil, 0, fmt.Errorf("prelude CRC mismatch: expected %d, got %d", preludeCRC, actualPreludeCRC)
	}

	// 验证 Message CRC
	msgCRC := binary.BigEndian.Uint32(buf[totalLen-4 : totalLen])
	actualMsgCRC := calcCRC32(buf[0 : totalLen-4])
	if actualMsgCRC != msgCRC {
		return nil, 0, fmt.Errorf("message CRC mismatch: expected %d, got %d", msgCRC, actualMsgCRC)
	}

	// 解析 Headers
	headersStart := PreludeSize
	headersEnd := headersStart + headerLen
	if headersEnd > totalLen-4 {
		return nil, 0, fmt.Errorf("header length exceeds message boundary")
	}

	headers, err := parseHeaders(buf[headersStart:headersEnd], headerLen)
	if err != nil {
		return nil, 0, fmt.Errorf("parse headers: %w", err)
	}

	// 提取 Payload
	payloadStart := headersEnd
	payloadEnd := totalLen - 4
	payload := make([]byte, payloadEnd-payloadStart)
	copy(payload, buf[payloadStart:payloadEnd])

	return &Frame{Headers: headers, Payload: payload}, totalLen, nil
}

// ── Decoder (流式解码器) ──

// Decoder AWS Event Stream 流式解码器
type Decoder struct {
	buf []byte
}

func NewDecoder() *Decoder {
	return &Decoder{buf: make([]byte, 0, 8192)}
}

// Feed 向解码器提供新数据
func (d *Decoder) Feed(data []byte) {
	d.buf = append(d.buf, data...)
}

// Decode 尝试解析所有可用的帧
func (d *Decoder) Decode() ([]*Frame, error) {
	var frames []*Frame
	for {
		frame, consumed, err := ParseFrame(d.buf)
		if err != nil {
			// 跳过错误帧，尝试恢复
			if len(d.buf) > 0 {
				d.buf = d.buf[1:] // 跳过一个字节尝试恢复
			}
			continue
		}
		if frame == nil {
			break // 数据不足
		}
		frames = append(frames, frame)
		d.buf = d.buf[consumed:]
	}
	return frames, nil
}
