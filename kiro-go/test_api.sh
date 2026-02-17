#!/bin/bash
# kiro-go API 测试集合
# 用法: ./test_api.sh [SERVER_URL] [API_KEY]
# 示例: ./test_api.sh http://117.72.183.248:13000 act-RYKG-DVA4-1NLF-XT3D
#       ./test_api.sh http://127.0.0.1:13000 kiro-proxy-123

SERVER="${1:-http://117.72.183.248:13000}"
KEY="${2:-act-RYKG-DVA4-1NLF-XT3D}"
PASS=0
FAIL=0
TIMEOUT=60

green() { echo -e "\033[32m$1\033[0m"; }
red()   { echo -e "\033[31m$1\033[0m"; }

run_test() {
    local name="$1"
    local expected="$2"
    local result="$3"
    
    if echo "$result" | grep -qi "$expected"; then
        green "✅ $name"
        PASS=$((PASS+1))
    else
        red "❌ $name"
        echo "   期望包含: $expected"
        echo "   实际结果: $(echo "$result" | head -c 200)"
        FAIL=$((FAIL+1))
    fi
}

echo "=========================================="
echo "  kiro-go API 测试集合"
echo "  服务器: $SERVER"
echo "=========================================="
echo ""

# ── 场景 1: 简单对话（非流式）──
echo "--- 场景 1: 简单对话（非流式）---"
R=$(curl -s --max-time $TIMEOUT "$SERVER/v1/chat/completions" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"claude-sonnet-4-5-20250929","messages":[{"role":"user","content":"say ok"}],"max_tokens":10,"stream":false}')
run_test "简单非流式对话" "chat.completion" "$R"

# ── 场景 2: 流式对话 ──
echo "--- 场景 2: 流式对话 ---"
R=$(curl -s --max-time $TIMEOUT "$SERVER/v1/chat/completions" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"claude-sonnet-4-5-20250929","stream":true,"messages":[{"role":"user","content":"say ok"}],"max_tokens":10}')
run_test "流式对话" "chat.completion.chunk" "$R"
run_test "流式 DONE 标记" "DONE" "$R"

# ── 场景 3: 多轮对话 ──
echo "--- 场景 3: 多轮对话 ---"
R=$(curl -s --max-time $TIMEOUT "$SERVER/v1/chat/completions" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"claude-sonnet-4-5-20250929","stream":false,"messages":[
    {"role":"user","content":"What is 2+2?"},
    {"role":"assistant","content":"4"},
    {"role":"user","content":"Multiply that by 3"}
  ],"max_tokens":20}')
run_test "多轮对话上下文" "12" "$R"

# ── 场景 4: Tool calls + tool results（非流式）──
echo "--- 场景 4: Tool calls + tool results ---"
R=$(curl -s --max-time $TIMEOUT "$SERVER/v1/chat/completions" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"claude-sonnet-4-5-20250929","stream":false,"messages":[
    {"role":"user","content":"Read /tmp/test.txt"},
    {"role":"assistant","content":null,"tool_calls":[{"id":"t1","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"/tmp/test.txt\"}"}}]},
    {"role":"tool","tool_call_id":"t1","content":"HELLO_WORLD_123"},
    {"role":"user","content":"What did the file say?"}
  ],"tools":[{"type":"function","function":{"name":"read_file","description":"Read a file","parameters":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}}}],"max_tokens":50}')
run_test "Tool result 内容传递" "HELLO_WORLD_123" "$R"

# ── 场景 5: 多次 tool_calls ──
echo "--- 场景 5: 多次 tool_calls ---"
R=$(curl -s --max-time $TIMEOUT "$SERVER/v1/chat/completions" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"claude-sonnet-4-5-20250929","stream":false,"messages":[
    {"role":"user","content":"Read a.txt and b.txt"},
    {"role":"assistant","content":null,"tool_calls":[
      {"id":"t1","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"a.txt\"}"}},
      {"id":"t2","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"b.txt\"}"}}
    ]},
    {"role":"tool","tool_call_id":"t1","content":"AAA"},
    {"role":"tool","tool_call_id":"t2","content":"BBB"},
    {"role":"user","content":"Combine them with a dash"}
  ],"tools":[{"type":"function","function":{"name":"read_file","description":"Read a file","parameters":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}}}],"max_tokens":50}')
run_test "多 tool_calls 合并" "AAA-BBB" "$R"

# ── 场景 6: 模型主动 tool_calls（非流式）──
echo "--- 场景 6: 模型主动 tool_calls ---"
R=$(curl -s --max-time $TIMEOUT "$SERVER/v1/chat/completions" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"claude-sonnet-4-5-20250929","stream":false,"messages":[
    {"role":"user","content":"Please read /tmp/hello.txt"}
  ],"tools":[{"type":"function","function":{"name":"read_file","description":"Read contents of a file","parameters":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}}}],"max_tokens":200}')
run_test "模型主动 tool_calls" "tool_calls" "$R"
run_test "finish_reason=tool_calls" "\"finish_reason\":\"tool_calls\"" "$R"

# ── 场景 7: 流式 tool_calls ──
echo "--- 场景 7: 流式 tool_calls ---"
R=$(curl -s --max-time $TIMEOUT "$SERVER/v1/chat/completions" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"claude-sonnet-4-5-20250929","stream":true,"messages":[
    {"role":"user","content":"Read /tmp/hello.txt"}
  ],"tools":[{"type":"function","function":{"name":"read_file","description":"Read a file","parameters":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}}}],"max_tokens":200}')
run_test "流式 tool_calls" "read_file" "$R"
run_test "流式 finish_reason" "tool_calls" "$R"

# ── 场景 8: 空 content assistant + 连续 tool messages ──
echo "--- 场景 8: 空 content + 连续 tool ---"
R=$(curl -s --max-time $TIMEOUT "$SERVER/v1/chat/completions" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"claude-sonnet-4-5-20250929","stream":false,"messages":[
    {"role":"user","content":"List /tmp and /var"},
    {"role":"assistant","content":"","tool_calls":[
      {"id":"t1","type":"function","function":{"name":"ls","arguments":"{\"path\":\"/tmp\"}"}},
      {"id":"t2","type":"function","function":{"name":"ls","arguments":"{\"path\":\"/var\"}"}}
    ]},
    {"role":"tool","tool_call_id":"t1","content":"a.txt b.txt"},
    {"role":"tool","tool_call_id":"t2","content":"log run"},
    {"role":"user","content":"How many items total?"}
  ],"tools":[{"type":"function","function":{"name":"ls","description":"List dir","parameters":{"type":"object","properties":{"path":{"type":"string"}},"required":["path"]}}}],"max_tokens":100}')
run_test "空 content + 连续 tool" "chat.completion" "$R"

# ── 场景 9: Thinking 模型 ──
echo "--- 场景 9: Thinking 模型 ---"
R=$(curl -s --max-time $TIMEOUT "$SERVER/v1/chat/completions" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"claude-sonnet-4-5-20250929-thinking","stream":true,"messages":[{"role":"user","content":"What is 99*101?"}],"max_tokens":2000}')
run_test "Thinking 模型响应" "chat.completion.chunk" "$R"

# ── 场景 10: Anthropic 原生 API ──
echo "--- 场景 10: Anthropic /v1/messages ---"
R=$(curl -s --max-time $TIMEOUT "$SERVER/v1/messages" \
  -H "x-api-key: $KEY" \
  -H "Content-Type: application/json" \
  -H "anthropic-version: 2023-06-01" \
  -d '{"model":"claude-sonnet-4-5-20250929","max_tokens":20,"messages":[{"role":"user","content":"say ok"}]}')
run_test "Anthropic API" "\"type\":\"message\"" "$R"

# ── 场景 11: 错误处理 - 无效模型 ──
echo "--- 场景 11: 错误处理 ---"
R=$(curl -s --max-time 10 "$SERVER/v1/chat/completions" \
  -H "Authorization: Bearer $KEY" \
  -H "Content-Type: application/json" \
  -d '{"model":"invalid-model-xyz","messages":[{"role":"user","content":"hi"}],"max_tokens":10}')
run_test "无效模型报错" "error" "$R"

# ── 汇总 ──
echo ""
echo "=========================================="
echo "  测试结果: $(green "$PASS 通过") / $(red "$FAIL 失败") / $((PASS+FAIL)) 总计"
echo "=========================================="

exit $FAIL
