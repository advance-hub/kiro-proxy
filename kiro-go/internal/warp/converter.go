package warp

import (
	"encoding/json"
	"strings"
	"time"

	pb "kiro-go/internal/warp/pb"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// BuildWarpRequest 将 Claude API 请求转换为 Warp protobuf 请求
func BuildWarpRequest(model string, messages []json.RawMessage, system string, tools []json.RawMessage, workingDir, homeDir string) []byte {
	if workingDir == "" {
		workingDir = "/tmp"
	}
	if homeDir == "" {
		homeDir = "/tmp"
	}

	taskID := uuid.New().String()
	convID := uuid.New().String()

	// 构建 InputContext
	inputContext := buildInputContext(workingDir, homeDir)

	// 添加 system 作为 project_rules
	if system != "" {
		inputContext.ProjectRules = []*pb.InputContext_ProjectRules{{
			RootPath: workingDir,
			ActiveRuleFiles: []*pb.FileContent{{
				FilePath: ".claude/rules.md",
				Content:  system,
			}},
		}}
	}

	// 转换消息
	taskMessages, userInputs := convertClaudeMessages(messages, inputContext)

	// 构建 Task
	task := &pb.Task{
		Id:          taskID,
		Description: "",
		Messages:    taskMessages,
		Summary:     "",
	}

	// 构建 Settings
	supportedTools := GetWarpSupportedTools(tools)
	settings := buildSettings(model, supportedTools)

	// 构建请求
	req := &pb.Request{
		TaskContext: &pb.Request_TaskContext{
			Tasks:        []*pb.Task{task},
			ActiveTaskId: taskID,
		},
		Input: &pb.Request_Input{
			Context: inputContext,
			Type: &pb.Request_Input_UserInputs_{
				UserInputs: &pb.Request_Input_UserInputs{
					Inputs: userInputs,
				},
			},
		},
		Settings: settings,
		Metadata: &pb.Request_Metadata{
			ConversationId: convID,
			Logging:        buildLoggingEntries(),
		},
	}

	data, err := proto.Marshal(req)
	if err != nil {
		return nil
	}
	return data
}

func buildInputContext(workingDir, homeDir string) *pb.InputContext {
	now := time.Now()
	return &pb.InputContext{
		Directory: &pb.InputContext_Directory{
			Pwd:  workingDir,
			Home: homeDir,
		},
		OperatingSystem: &pb.InputContext_OperatingSystem{
			Platform: "macOS",
		},
		Shell: &pb.InputContext_Shell{
			Name:    "zsh",
			Version: "5.9",
		},
		CurrentTime: &timestamppb.Timestamp{
			Seconds: now.Unix(),
			Nanos:   int32(now.Nanosecond()),
		},
	}
}

func buildSettings(model string, supportedTools []pb.ToolType) *pb.Request_Settings {
	return &pb.Request_Settings{
		ModelConfig: &pb.Request_Settings_ModelConfig{
			Base:     model,
			Planning: "",
			Coding:   "",
		},
		RulesEnabled:                       true,
		WebContextRetrievalEnabled:         false,
		SupportsParallelToolCalls:          true,
		UseAnthropicTextEditorTools:        false,
		PlanningEnabled:                    false,
		WarpDriveContextEnabled:            false,
		SupportsCreateFiles:                true,
		SupportedTools:                     supportedTools,
		SupportsLongRunningCommands:        true,
		ShouldPreserveFileContentInHistory: false,
		SupportsTodosUi:                    true,
		SupportsLinkedCodeBlocks:           true,
	}
}

func buildLoggingEntries() []*pb.Request_Metadata_LoggingEntry {
	strVal := func(s string) *structpb.Value {
		return &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: s}}
	}
	numVal := func(n float64) *structpb.Value {
		return &structpb.Value{Kind: &structpb.Value_NumberValue{NumberValue: n}}
	}
	boolVal := func(b bool) *structpb.Value {
		return &structpb.Value{Kind: &structpb.Value_BoolValue{BoolValue: b}}
	}
	_ = numVal // suppress unused if not needed

	return []*pb.Request_Metadata_LoggingEntry{
		{Key: "entrypoint", Value: strVal("USER_INITIATED")},
		{Key: "is_auto_resume_after_error", Value: boolVal(false)},
		{Key: "is_autodetected_user_query", Value: boolVal(true)},
	}
}

// convertClaudeMessages 转换 Claude 消息数组为 Warp 格式
func convertClaudeMessages(messages []json.RawMessage, inputContext *pb.InputContext) (taskMessages []*pb.Message, userInputs []*pb.Request_Input_UserInputs_UserInput) {
	toolCallMap := make(map[string]string) // tool_use_id → tool_name

	for i, raw := range messages {
		var msg struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		}
		if json.Unmarshal(raw, &msg) != nil {
			continue
		}
		isLast := i == len(messages)-1

		if msg.Role == "user" {
			tm, ui := convertUserMessage(msg.Content, inputContext, isLast, toolCallMap)
			taskMessages = append(taskMessages, tm...)
			userInputs = append(userInputs, ui...)
		} else if msg.Role == "assistant" {
			tm := convertAssistantMessage(msg.Content, toolCallMap)
			taskMessages = append(taskMessages, tm...)
		}
	}
	return
}

func convertUserMessage(content json.RawMessage, inputContext *pb.InputContext, isLast bool, toolCallMap map[string]string) (taskMessages []*pb.Message, userInputs []*pb.Request_Input_UserInputs_UserInput) {
	// 尝试解析为字符串
	var textStr string
	if json.Unmarshal(content, &textStr) == nil {
		userQuery := &pb.Request_Input_UserQuery{
			Query: textStr,
		}
		if isLast {
			userInputs = append(userInputs, &pb.Request_Input_UserInputs_UserInput{
				Input: &pb.Request_Input_UserInputs_UserInput_UserQuery{
					UserQuery: userQuery,
				},
			})
		} else {
			taskMessages = append(taskMessages, &pb.Message{
				Id: uuid.New().String(),
				Message: &pb.Message_UserQuery_{
					UserQuery: &pb.Message_UserQuery{
						Query:   textStr,
						Context: inputContext,
					},
				},
			})
		}
		return
	}

	// 解析为数组
	var blocks []map[string]interface{}
	if json.Unmarshal(content, &blocks) != nil {
		return
	}

	var textContent string
	var toolResults []*pb.Request_Input_UserInputs_UserInput

	for _, block := range blocks {
		blockType, _ := block["type"].(string)
		if blockType == "text" {
			text, _ := block["text"].(string)
			textContent += text
		} else if blockType == "tool_result" {
			toolUseID, _ := block["tool_use_id"].(string)
			toolName := toolCallMap[toolUseID]
			if toolName == "" {
				toolName = "Bash"
			}
			resultContent := extractToolResultContent(block)
			isError, _ := block["is_error"].(bool)
			warpResult := ClaudeToolResultToWarpToolCallResult(toolUseID, toolName, resultContent, isError)
			toolResults = append(toolResults, &pb.Request_Input_UserInputs_UserInput{
				Input: &pb.Request_Input_UserInputs_UserInput_ToolCallResult{
					ToolCallResult: warpResult,
				},
			})
		}
	}

	if textContent != "" {
		userQuery := &pb.Request_Input_UserQuery{
			Query: textContent,
		}
		if isLast && len(toolResults) == 0 {
			userInputs = append(userInputs, &pb.Request_Input_UserInputs_UserInput{
				Input: &pb.Request_Input_UserInputs_UserInput_UserQuery{
					UserQuery: userQuery,
				},
			})
		} else {
			taskMessages = append(taskMessages, &pb.Message{
				Id: uuid.New().String(),
				Message: &pb.Message_UserQuery_{
					UserQuery: &pb.Message_UserQuery{
						Query:   textContent,
						Context: inputContext,
					},
				},
			})
		}
	}

	if len(toolResults) > 0 {
		if isLast {
			userInputs = append(userInputs, toolResults...)
		} else {
			for _, tr := range toolResults {
				tcr := tr.GetToolCallResult()
				taskMessages = append(taskMessages, &pb.Message{
					Id: uuid.New().String(),
					Message: &pb.Message_ToolCallResult_{
						ToolCallResult: convertInputToolCallResultToMessageToolCallResult(tcr),
					},
				})
			}
		}
	}

	return
}

func convertAssistantMessage(content json.RawMessage, toolCallMap map[string]string) (taskMessages []*pb.Message) {
	// 尝试字符串
	var textStr string
	if json.Unmarshal(content, &textStr) == nil {
		taskMessages = append(taskMessages, &pb.Message{
			Id: uuid.New().String(),
			Message: &pb.Message_AgentOutput_{
				AgentOutput: &pb.Message_AgentOutput{Text: textStr},
			},
		})
		return
	}

	// 数组
	var blocks []map[string]interface{}
	if json.Unmarshal(content, &blocks) != nil {
		return
	}

	for _, block := range blocks {
		blockType, _ := block["type"].(string)
		if blockType == "text" {
			text, _ := block["text"].(string)
			taskMessages = append(taskMessages, &pb.Message{
				Id: uuid.New().String(),
				Message: &pb.Message_AgentOutput_{
					AgentOutput: &pb.Message_AgentOutput{Text: text},
				},
			})
		} else if blockType == "tool_use" {
			id, _ := block["id"].(string)
			name, _ := block["name"].(string)
			input, _ := block["input"].(map[string]interface{})
			if input == nil {
				input = make(map[string]interface{})
			}
			toolCallMap[id] = name
			toolCall := ClaudeToolUseToWarpToolCall(id, name, input)
			taskMessages = append(taskMessages, &pb.Message{
				Id: uuid.New().String(),
				Message: &pb.Message_ToolCall_{
					ToolCall: toolCall,
				},
			})
		}
	}
	return
}

func extractToolResultContent(block map[string]interface{}) string {
	c := block["content"]
	switch v := c.(type) {
	case string:
		return v
	case []interface{}:
		var parts []string
		for _, item := range v {
			if m, ok := item.(map[string]interface{}); ok {
				if text, ok := m["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
	}
	return "(empty result)"
}

// convertInputToolCallResultToMessageToolCallResult converts Request.Input.ToolCallResult to Message.ToolCallResult
func convertInputToolCallResultToMessageToolCallResult(input *pb.Request_Input_ToolCallResult) *pb.Message_ToolCallResult {
	if input == nil {
		return nil
	}
	result := &pb.Message_ToolCallResult{
		ToolCallId: input.ToolCallId,
	}
	// Map each result type
	switch r := input.Result.(type) {
	case *pb.Request_Input_ToolCallResult_RunShellCommand:
		result.Result = &pb.Message_ToolCallResult_RunShellCommand{RunShellCommand: r.RunShellCommand}
	case *pb.Request_Input_ToolCallResult_ReadFiles:
		result.Result = &pb.Message_ToolCallResult_ReadFiles{ReadFiles: r.ReadFiles}
	case *pb.Request_Input_ToolCallResult_ApplyFileDiffs:
		result.Result = &pb.Message_ToolCallResult_ApplyFileDiffs{ApplyFileDiffs: r.ApplyFileDiffs}
	case *pb.Request_Input_ToolCallResult_Grep:
		result.Result = &pb.Message_ToolCallResult_Grep{Grep: r.Grep}
	case *pb.Request_Input_ToolCallResult_FileGlobV2:
		result.Result = &pb.Message_ToolCallResult_FileGlobV2{FileGlobV2: r.FileGlobV2}
	case *pb.Request_Input_ToolCallResult_CallMcpTool:
		result.Result = &pb.Message_ToolCallResult_CallMcpTool{CallMcpTool: r.CallMcpTool}
	case *pb.Request_Input_ToolCallResult_SearchCodebase:
		result.Result = &pb.Message_ToolCallResult_SearchCodebase{SearchCodebase: r.SearchCodebase}
	}
	return result
}
