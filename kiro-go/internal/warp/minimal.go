package warp

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"time"
)

// 手工编码 protobuf - 参考 all-2-api 的 warp-service.js

func encVarint(v uint64) []byte {
	var out []byte
	for v >= 0x80 {
		out = append(out, byte(v&0x7F)|0x80)
		v >>= 7
	}
	out = append(out, byte(v))
	return out
}

func encField(fieldNo int, wireType int, payload []byte) []byte {
	tag := encVarint(uint64((fieldNo << 3) | wireType))
	return append(tag, payload...)
}

func encString(fieldNo int, s string) []byte {
	b := []byte(s)
	length := encVarint(uint64(len(b)))
	payload := append(length, b...)
	return encField(fieldNo, 2, payload)
}

func encBytes(fieldNo int, b []byte) []byte {
	length := encVarint(uint64(len(b)))
	payload := append(length, b...)
	return encField(fieldNo, 2, payload)
}

func encMessage(fieldNo int, payload []byte) []byte {
	length := encVarint(uint64(len(payload)))
	data := append(length, payload...)
	return encField(fieldNo, 2, data)
}

func encVarintField(fieldNo int, v uint64) []byte {
	return encField(fieldNo, 0, encVarint(v))
}

func encFixed32(fieldNo int, v uint32) []byte {
	b := make([]byte, 4)
	binary.LittleEndian.PutUint32(b, v)
	return encField(fieldNo, 5, b)
}

// BuildMinimalWarpRequest 构建极简化的 Warp protobuf 请求
// 完全参考 all-2-api 的 buildRequestBody 函数
func BuildMinimalWarpRequest(query string) []byte {
	workingDir := "/tmp"
	homeDir := "/tmp"

	now := time.Now()
	ts := now.Unix()
	nanos := int64(now.Nanosecond())

	// field1: empty string
	field1 := encString(1, "")

	// field2: Input context
	pathInfo := bytes.Join([][]byte{
		encString(1, workingDir),
		encString(2, homeDir),
	}, nil)

	osInfo := encMessage(1, encFixed32(9, 0x534F6361)) // 特殊的 OS 标识

	shellInfo := bytes.Join([][]byte{
		encString(1, "zsh"),
		encString(2, "5.9"),
	}, nil)

	tsInfo := bytes.Join([][]byte{
		encVarintField(1, uint64(ts)),
		encVarintField(2, uint64(nanos)),
	}, nil)

	field2_1 := bytes.Join([][]byte{
		encMessage(1, pathInfo),
		encMessage(2, osInfo),
		encMessage(3, shellInfo),
		encMessage(4, tsInfo),
	}, nil)

	queryContent := bytes.Join([][]byte{
		encString(1, query),
		encString(3, ""),
		encVarintField(4, 1),
	}, nil)

	field2_6 := encMessage(1, encMessage(1, queryContent))

	field2Content := bytes.Join([][]byte{
		encMessage(1, field2_1),
		encMessage(6, field2_6),
	}, nil)

	// field3: Settings
	modelConfig := bytes.Join([][]byte{
		encString(1, "auto-efficient"),
		encString(4, "cli-agent-auto"),
	}, nil)

	// 固定的 capabilities 字节序列（从 all-2-api 复制）
	caps := []byte{0x06, 0x07, 0x0C, 0x08, 0x09, 0x0F, 0x0E, 0x00, 0x0B, 0x10, 0x0A, 0x14, 0x11, 0x13, 0x12, 0x02, 0x03, 0x01, 0x0D}
	caps2 := []byte{0x0A, 0x14, 0x06, 0x07, 0x0C, 0x02, 0x01}

	field3Content := bytes.Join([][]byte{
		encMessage(1, modelConfig),
		encVarintField(2, 1),
		encVarintField(3, 1),
		encVarintField(4, 1),
		encVarintField(6, 1),
		encVarintField(7, 1),
		encVarintField(8, 1),
		encBytes(9, caps),
		encVarintField(10, 1),
		encVarintField(11, 1),
		encVarintField(12, 1),
		encVarintField(13, 1),
		encVarintField(14, 1),
		encVarintField(15, 1),
		encVarintField(16, 1),
		encVarintField(17, 1),
		encVarintField(21, 1),
		encBytes(22, caps2),
		encVarintField(23, 1),
	}, nil)

	// field4: Metadata (logging entries)
	entrypoint := bytes.Join([][]byte{
		encString(1, "entrypoint"),
		encMessage(2, encMessage(3, encString(1, "USER_INITIATED"))),
	}, nil)

	autoResume := bytes.Join([][]byte{
		encString(1, "is_auto_resume_after_error"),
		encMessage(2, encVarintField(4, 0)),
	}, nil)

	autoDetect := bytes.Join([][]byte{
		encString(1, "is_autodetected_user_query"),
		encMessage(2, encVarintField(4, 1)),
	}, nil)

	field4Content := bytes.Join([][]byte{
		encMessage(2, entrypoint),
		encMessage(2, autoResume),
		encMessage(2, autoDetect),
	}, nil)

	// 组合所有字段
	return bytes.Join([][]byte{
		field1,
		encMessage(2, field2Content),
		encMessage(3, field3Content),
		encMessage(4, field4Content),
	}, nil)
}

// BuildWarpRequestFromMessages 从 Claude 消息构建极简 Warp 请求
func BuildWarpRequestFromMessages(messages []json.RawMessage) []byte {
	// 提取最后一条用户消息作为查询
	var query string
	for i := len(messages) - 1; i >= 0; i-- {
		var msg struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if json.Unmarshal(messages[i], &msg) == nil && msg.Role == "user" {
			// 尝试解析为字符串
			var textStr string
			if json.Unmarshal(msg.Content, &textStr) == nil {
				query = textStr
				break
			}
			// 尝试解析为数组
			var blocks []map[string]interface{}
			if json.Unmarshal(msg.Content, &blocks) == nil {
				for _, block := range blocks {
					if blockType, _ := block["type"].(string); blockType == "text" {
						if text, ok := block["text"].(string); ok {
							query = text
							break
						}
					}
				}
				if query != "" {
					break
				}
			}
		}
	}

	if query == "" {
		query = "你好"
	}

	return BuildMinimalWarpRequest(query)
}
