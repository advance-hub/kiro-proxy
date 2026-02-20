package warp

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	pb "kiro-go/internal/warp/pb"

	"google.golang.org/protobuf/types/known/structpb"
)

// claudeToWarpTool Claude 工具名 → Warp ToolType
var claudeToWarpTool = map[string]pb.ToolType{
	"Bash":               pb.ToolType_RUN_SHELL_COMMAND,
	"Read":               pb.ToolType_READ_FILES,
	"Write":              pb.ToolType_APPLY_FILE_DIFFS,
	"Edit":               pb.ToolType_APPLY_FILE_DIFFS,
	"Grep":               pb.ToolType_GREP,
	"Glob":               pb.ToolType_FILE_GLOB_V2,
	"SearchCodebase":     pb.ToolType_SEARCH_CODEBASE,
	"Task":               pb.ToolType_SUBAGENT,
	"Subagent":           pb.ToolType_SUBAGENT,
	"WebFetch":           pb.ToolType_CALL_MCP_TOOL,
	"WebSearch":          pb.ToolType_CALL_MCP_TOOL,
	"web_search":         pb.ToolType_CALL_MCP_TOOL,
	"TodoWrite":          pb.ToolType_CALL_MCP_TOOL,
	"TodoRead":           pb.ToolType_CALL_MCP_TOOL,
	"ReadDocuments":      pb.ToolType_READ_DOCUMENTS,
	"EditDocuments":      pb.ToolType_EDIT_DOCUMENTS,
	"CreateDocuments":    pb.ToolType_CREATE_DOCUMENTS,
	"WriteToShell":       pb.ToolType_WRITE_TO_LONG_RUNNING_SHELL_COMMAND,
	"ReadShellOutput":    pb.ToolType_READ_SHELL_COMMAND_OUTPUT,
	"Plan":               pb.ToolType_SUGGEST_PLAN,
	"SuggestPlan":        pb.ToolType_SUGGEST_PLAN,
	"UseComputer":        pb.ToolType_USE_COMPUTER,
	"computer":           pb.ToolType_USE_COMPUTER,
	"ReadMCPResource":    pb.ToolType_READ_MCP_RESOURCE,
	"CallMCPTool":        pb.ToolType_CALL_MCP_TOOL,
	"ReadSkill":          pb.ToolType_READ_SKILL,
	"Skill":              pb.ToolType_READ_SKILL,
	"OpenCodeReview":     pb.ToolType_OPEN_CODE_REVIEW,
	"RequestComputerUse": pb.ToolType_REQUEST_COMPUTER_USE,
}

// GetWarpSupportedTools 从 Claude tools 定义获取 Warp 支持的工具类型列表
func GetWarpSupportedTools(claudeTools []json.RawMessage) []pb.ToolType {
	if len(claudeTools) == 0 {
		return []pb.ToolType{
			pb.ToolType_RUN_SHELL_COMMAND, pb.ToolType_READ_FILES, pb.ToolType_APPLY_FILE_DIFFS,
			pb.ToolType_GREP, pb.ToolType_FILE_GLOB_V2,
		}
	}
	seen := make(map[pb.ToolType]bool)
	for _, raw := range claudeTools {
		var tool struct {
			Name string `json:"name"`
		}
		if json.Unmarshal(raw, &tool) != nil {
			continue
		}
		if tt, ok := claudeToWarpTool[tool.Name]; ok {
			seen[tt] = true
		} else if strings.HasPrefix(tool.Name, "mcp__") {
			seen[pb.ToolType_CALL_MCP_TOOL] = true
		}
	}
	result := make([]pb.ToolType, 0, len(seen))
	for tt := range seen {
		result = append(result, tt)
	}
	return result
}

// ClaudeToolUseToWarpToolCall 将 Claude tool_use 转换为 Warp ToolCall proto
func ClaudeToolUseToWarpToolCall(id, name string, input map[string]interface{}) *pb.Message_ToolCall {
	tc := &pb.Message_ToolCall{ToolCallId: id}

	switch name {
	case "Bash":
		cmd, _ := input["command"].(string)
		tc.Tool = &pb.Message_ToolCall_RunShellCommand_{
			RunShellCommand: &pb.Message_ToolCall_RunShellCommand{
				Command:    cmd,
				IsReadOnly: isReadOnlyCommand(cmd),
				IsRisky:    isRiskyCommand(cmd),
			},
		}

	case "Read":
		filePath, _ := input["file_path"].(string)
		file := &pb.Message_ToolCall_ReadFiles_File{Name: filePath}
		if offset, ok := input["offset"].(float64); ok {
			if limit, ok2 := input["limit"].(float64); ok2 {
				file.LineRanges = []*pb.FileContentLineRange{{
					Start: uint32(offset),
					End:   uint32(offset + limit),
				}}
			}
		}
		tc.Tool = &pb.Message_ToolCall_ReadFiles_{
			ReadFiles: &pb.Message_ToolCall_ReadFiles{
				Files: []*pb.Message_ToolCall_ReadFiles_File{file},
			},
		}

	case "Write":
		filePath, _ := input["file_path"].(string)
		content, _ := input["content"].(string)
		tc.Tool = &pb.Message_ToolCall_ApplyFileDiffs_{
			ApplyFileDiffs: &pb.Message_ToolCall_ApplyFileDiffs{
				Summary: fmt.Sprintf("Create %s", filePath),
				NewFiles: []*pb.Message_ToolCall_ApplyFileDiffs_NewFile{{
					FilePath: filePath,
					Content:  content,
				}},
			},
		}

	case "Edit":
		filePath, _ := input["file_path"].(string)
		oldStr, _ := input["old_string"].(string)
		newStr, _ := input["new_string"].(string)
		tc.Tool = &pb.Message_ToolCall_ApplyFileDiffs_{
			ApplyFileDiffs: &pb.Message_ToolCall_ApplyFileDiffs{
				Summary: fmt.Sprintf("Edit %s", filePath),
				Diffs: []*pb.Message_ToolCall_ApplyFileDiffs_FileDiff{{
					FilePath: filePath,
					Search:   oldStr,
					Replace:  newStr,
				}},
			},
		}

	case "Grep":
		pattern, _ := input["pattern"].(string)
		path, _ := input["path"].(string)
		tc.Tool = &pb.Message_ToolCall_Grep_{
			Grep: &pb.Message_ToolCall_Grep{
				Queries: []string{pattern},
				Path:    path,
			},
		}

	case "Glob":
		pattern, _ := input["pattern"].(string)
		path, _ := input["path"].(string)
		tc.Tool = &pb.Message_ToolCall_FileGlobV2_{
			FileGlobV2: &pb.Message_ToolCall_FileGlobV2{
				Patterns:  []string{pattern},
				SearchDir: path,
			},
		}

	case "SearchCodebase":
		query, _ := input["query"].(string)
		tc.Tool = &pb.Message_ToolCall_SearchCodebase_{
			SearchCodebase: &pb.Message_ToolCall_SearchCodebase{
				Query: query,
			},
		}

	case "Task", "Subagent":
		payload, _ := json.Marshal(input)
		taskID, _ := input["task_id"].(string)
		if taskID == "" {
			taskID = id
		}
		tc.Tool = &pb.Message_ToolCall_Subagent_{
			Subagent: &pb.Message_ToolCall_Subagent{
				TaskId:  taskID,
				Payload: string(payload),
			},
		}

	case "ReadMCPResource":
		uri, _ := input["uri"].(string)
		serverID, _ := input["server_id"].(string)
		tc.Tool = &pb.Message_ToolCall_ReadMcpResource{
			ReadMcpResource: &pb.Message_ToolCall_ReadMCPResource{
				Uri:      uri,
				ServerId: serverID,
			},
		}

	case "ReadSkill", "Skill":
		skillPath, _ := input["skill_path"].(string)
		skillName, _ := input["skill_name"].(string)
		tc.Tool = &pb.Message_ToolCall_ReadSkill_{
			ReadSkill: &pb.Message_ToolCall_ReadSkill{
				SkillPath: skillPath,
				SkillName: skillName,
			},
		}

	default:
		// MCP 工具或未知工具
		argsStruct, _ := structpb.NewStruct(input)
		tc.Tool = &pb.Message_ToolCall_CallMcpTool{
			CallMcpTool: &pb.Message_ToolCall_CallMCPTool{
				Name: name,
				Args: argsStruct,
			},
		}
	}

	return tc
}

// WarpToolCallToClaudeToolUse 将 Warp ToolCall proto 解析为 Claude tool_use
func WarpToolCallToClaudeToolUse(tc *pb.Message_ToolCall) (id, name string, input map[string]interface{}) {
	if tc == nil {
		return "", "", nil
	}
	id = tc.ToolCallId
	input = make(map[string]interface{})

	switch t := tc.Tool.(type) {
	case *pb.Message_ToolCall_RunShellCommand_:
		name = "Bash"
		input["command"] = t.RunShellCommand.GetCommand()

	case *pb.Message_ToolCall_ReadFiles_:
		name = "Read"
		if files := t.ReadFiles.GetFiles(); len(files) > 0 {
			input["file_path"] = files[0].GetName()
			if ranges := files[0].GetLineRanges(); len(ranges) > 0 {
				input["offset"] = float64(ranges[0].GetStart())
				input["limit"] = float64(ranges[0].GetEnd() - ranges[0].GetStart())
			}
		}

	case *pb.Message_ToolCall_ApplyFileDiffs_:
		afd := t.ApplyFileDiffs
		if nf := afd.GetNewFiles(); len(nf) > 0 {
			name = "Write"
			input["file_path"] = nf[0].GetFilePath()
			input["content"] = nf[0].GetContent()
		} else if diffs := afd.GetDiffs(); len(diffs) > 0 {
			name = "Edit"
			input["file_path"] = diffs[0].GetFilePath()
			input["old_string"] = normalizeIndent(stripLineNumbers(diffs[0].GetSearch()), 2)
			input["new_string"] = normalizeIndent(stripLineNumbers(diffs[0].GetReplace()), 2)
		}

	case *pb.Message_ToolCall_SearchCodebase_:
		name = "SearchCodebase"
		input["query"] = t.SearchCodebase.GetQuery()

	case *pb.Message_ToolCall_Grep_:
		name = "Grep"
		if qs := t.Grep.GetQueries(); len(qs) > 0 {
			input["pattern"] = qs[0]
		}
		input["path"] = t.Grep.GetPath()

	case *pb.Message_ToolCall_FileGlob_:
		name = "Glob"
		if ps := t.FileGlob.GetPatterns(); len(ps) > 0 {
			input["pattern"] = ps[0]
		}
		input["path"] = t.FileGlob.GetPath()

	case *pb.Message_ToolCall_FileGlobV2_:
		name = "Glob"
		if ps := t.FileGlobV2.GetPatterns(); len(ps) > 0 {
			input["pattern"] = ps[0]
		}
		input["path"] = t.FileGlobV2.GetSearchDir()

	case *pb.Message_ToolCall_ReadMcpResource:
		name = "ReadMCPResource"
		input["uri"] = t.ReadMcpResource.GetUri()
		input["server_id"] = t.ReadMcpResource.GetServerId()

	case *pb.Message_ToolCall_CallMcpTool:
		name = t.CallMcpTool.GetName()
		if name == "" {
			name = "mcp__unknown"
		}
		if args := t.CallMcpTool.GetArgs(); args != nil {
			input = args.AsMap()
		}

	case *pb.Message_ToolCall_WriteToLongRunningShellCommand_:
		name = "WriteToShell"
		input["input"] = string(t.WriteToLongRunningShellCommand.GetInput())
		input["command_id"] = t.WriteToLongRunningShellCommand.GetCommandId()

	case *pb.Message_ToolCall_Subagent_:
		name = "Task"
		payload := t.Subagent.GetPayload()
		if payload != "" {
			json.Unmarshal([]byte(payload), &input)
		}
		input["task_id"] = t.Subagent.GetTaskId()

	case *pb.Message_ToolCall_ReadDocuments_:
		name = "ReadDocuments"

	case *pb.Message_ToolCall_EditDocuments_:
		name = "EditDocuments"

	case *pb.Message_ToolCall_CreateDocuments_:
		name = "Write"
		if docs := t.CreateDocuments.GetNewDocuments(); len(docs) > 0 {
			input["content"] = docs[0].GetContent()
			input["file_path"] = docs[0].GetTitle()
		}

	case *pb.Message_ToolCall_ReadShellCommandOutput_:
		name = "ReadShellOutput"
		input["command_id"] = t.ReadShellCommandOutput.GetCommandId()

	case *pb.Message_ToolCall_UseComputer_:
		name = "computer"
		input["action_summary"] = t.UseComputer.GetActionSummary()

	case *pb.Message_ToolCall_ReadSkill_:
		name = "Skill"
		input["skill_path"] = t.ReadSkill.GetSkillPath()
		input["skill_name"] = t.ReadSkill.GetSkillName()

	case *pb.Message_ToolCall_RequestComputerUse_:
		name = "RequestComputerUse"
		input["task_summary"] = t.RequestComputerUse.GetTaskSummary()

	default:
		name = "unknown"
	}

	return
}

// ClaudeToolResultToWarpToolCallResult 将 Claude tool_result 转换为 Warp Request.Input.ToolCallResult
func ClaudeToolResultToWarpToolCallResult(toolUseID, toolName, content string, isError bool) *pb.Request_Input_ToolCallResult {
	result := &pb.Request_Input_ToolCallResult{ToolCallId: toolUseID}

	switch toolName {
	case "Bash":
		exitCode := int32(0)
		if isError {
			exitCode = 1
		}
		result.Result = &pb.Request_Input_ToolCallResult_RunShellCommand{
			RunShellCommand: &pb.RunShellCommandResult{
				Command: "",
				Result: &pb.RunShellCommandResult_CommandFinished{
					CommandFinished: &pb.ShellCommandFinished{
						Output:   content,
						ExitCode: exitCode,
					},
				},
			},
		}

	case "Read":
		if isError {
			result.Result = &pb.Request_Input_ToolCallResult_ReadFiles{
				ReadFiles: &pb.ReadFilesResult{
					Result: &pb.ReadFilesResult_Error_{
						Error: &pb.ReadFilesResult_Error{Message: content},
					},
				},
			}
		} else {
			result.Result = &pb.Request_Input_ToolCallResult_ReadFiles{
				ReadFiles: &pb.ReadFilesResult{
					Result: &pb.ReadFilesResult_TextFilesSuccess_{
						TextFilesSuccess: &pb.ReadFilesResult_TextFilesSuccess{
							Files: []*pb.FileContent{{FilePath: "", Content: content}},
						},
					},
				},
			}
		}

	case "Write", "Edit":
		if isError {
			result.Result = &pb.Request_Input_ToolCallResult_ApplyFileDiffs{
				ApplyFileDiffs: &pb.ApplyFileDiffsResult{
					Result: &pb.ApplyFileDiffsResult_Error_{
						Error: &pb.ApplyFileDiffsResult_Error{Message: content},
					},
				},
			}
		} else {
			result.Result = &pb.Request_Input_ToolCallResult_ApplyFileDiffs{
				ApplyFileDiffs: &pb.ApplyFileDiffsResult{
					Result: &pb.ApplyFileDiffsResult_Success_{
						Success: &pb.ApplyFileDiffsResult_Success{},
					},
				},
			}
		}

	case "Grep":
		if isError {
			result.Result = &pb.Request_Input_ToolCallResult_Grep{
				Grep: &pb.GrepResult{
					Result: &pb.GrepResult_Error_{
						Error: &pb.GrepResult_Error{Message: content},
					},
				},
			}
		} else {
			result.Result = &pb.Request_Input_ToolCallResult_Grep{
				Grep: &pb.GrepResult{
					Result: &pb.GrepResult_Success_{
						Success: &pb.GrepResult_Success{},
					},
				},
			}
		}

	case "Glob":
		if isError {
			result.Result = &pb.Request_Input_ToolCallResult_FileGlobV2{
				FileGlobV2: &pb.FileGlobV2Result{
					Result: &pb.FileGlobV2Result_Error_{
						Error: &pb.FileGlobV2Result_Error{Message: content},
					},
				},
			}
		} else {
			result.Result = &pb.Request_Input_ToolCallResult_FileGlobV2{
				FileGlobV2: &pb.FileGlobV2Result{
					Result: &pb.FileGlobV2Result_Success_{
						Success: &pb.FileGlobV2Result_Success{},
					},
				},
			}
		}

	case "SearchCodebase":
		if isError {
			result.Result = &pb.Request_Input_ToolCallResult_SearchCodebase{
				SearchCodebase: &pb.SearchCodebaseResult{
					Result: &pb.SearchCodebaseResult_Error_{
						Error: &pb.SearchCodebaseResult_Error{Message: content},
					},
				},
			}
		} else {
			result.Result = &pb.Request_Input_ToolCallResult_SearchCodebase{
				SearchCodebase: &pb.SearchCodebaseResult{
					Result: &pb.SearchCodebaseResult_Success_{
						Success: &pb.SearchCodebaseResult_Success{},
					},
				},
			}
		}

	case "Task", "Subagent":
		result.Result = &pb.Request_Input_ToolCallResult_CallMcpTool{
			CallMcpTool: &pb.CallMCPToolResult{
				Result: &pb.CallMCPToolResult_Success_{
					Success: &pb.CallMCPToolResult_Success{
						Results: []*pb.CallMCPToolResult_Success_Result{{
							Result: &pb.CallMCPToolResult_Success_Result_Text_{
								Text: &pb.CallMCPToolResult_Success_Result_Text{Text: content},
							},
						}},
					},
				},
			},
		}

	default:
		// MCP 工具
		if isError {
			result.Result = &pb.Request_Input_ToolCallResult_CallMcpTool{
				CallMcpTool: &pb.CallMCPToolResult{
					Result: &pb.CallMCPToolResult_Error_{
						Error: &pb.CallMCPToolResult_Error{Message: content},
					},
				},
			}
		} else {
			result.Result = &pb.Request_Input_ToolCallResult_CallMcpTool{
				CallMcpTool: &pb.CallMCPToolResult{
					Result: &pb.CallMCPToolResult_Success_{
						Success: &pb.CallMCPToolResult_Success{
							Results: []*pb.CallMCPToolResult_Success_Result{{
								Result: &pb.CallMCPToolResult_Success_Result_Text_{
									Text: &pb.CallMCPToolResult_Success_Result_Text{Text: content},
								},
							}},
						},
					},
				},
			}
		}
	}

	return result
}

// ── 辅助函数 ──

var readOnlyPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^ls\b`),
	regexp.MustCompile(`^cat\b`),
	regexp.MustCompile(`^head\b`),
	regexp.MustCompile(`^tail\b`),
	regexp.MustCompile(`^grep\b`),
	regexp.MustCompile(`^find\b`),
	regexp.MustCompile(`^pwd\b`),
	regexp.MustCompile(`^echo\b`),
	regexp.MustCompile(`^wc\b`),
	regexp.MustCompile(`^tree\b`),
	regexp.MustCompile(`^which\b`),
	regexp.MustCompile(`^whoami\b`),
	regexp.MustCompile(`^date\b`),
	regexp.MustCompile(`^uname\b`),
	regexp.MustCompile(`^git\s+(status|log|diff|show|branch|remote|tag)\b`),
}

func isReadOnlyCommand(cmd string) bool {
	cmd = strings.TrimSpace(cmd)
	for _, p := range readOnlyPatterns {
		if p.MatchString(cmd) {
			return true
		}
	}
	return false
}

var riskyPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\brm\s+.*\*`),
	regexp.MustCompile(`\bsudo\b`),
	regexp.MustCompile(`\bchmod\s+777\b`),
	regexp.MustCompile(`\bmkfs\b`),
	regexp.MustCompile(`\bdd\b`),
	regexp.MustCompile(`\bcurl\b.*\|\s*(ba)?sh`),
	regexp.MustCompile(`\bwget\b.*\|\s*(ba)?sh`),
	regexp.MustCompile(`\bkillall\b`),
	regexp.MustCompile(`\bshutdown\b`),
	regexp.MustCompile(`\breboot\b`),
}

func isRiskyCommand(cmd string) bool {
	for _, p := range riskyPatterns {
		if p.MatchString(cmd) {
			return true
		}
	}
	return false
}

// stripLineNumbers 去除行号前缀（参考 warp-tool-mapper.js）
func stripLineNumbers(s string) string {
	if s == "" {
		return s
	}
	lineNumRe1 := regexp.MustCompile(`^\s*\d+[|→]\s?(.*)$`)
	lineNumRe2 := regexp.MustCompile(`^\s*\d+\t(.*)$`)
	lineNumRe3 := regexp.MustCompile(`^(\d+):(.*)$`)

	lines := strings.Split(s, "\n")
	for i, line := range lines {
		if m := lineNumRe1.FindStringSubmatch(line); m != nil {
			lines[i] = m[1]
		} else if m := lineNumRe2.FindStringSubmatch(line); m != nil {
			lines[i] = m[1]
		} else if m := lineNumRe3.FindStringSubmatch(line); m != nil {
			lines[i] = m[2]
		}
	}
	return strings.Join(lines, "\n")
}

// detectIndentStyle 检测字符串的缩进风格
func detectIndentStyle(s string) (char byte, size int) {
	if s == "" {
		return ' ', 2
	}
	lines := strings.Split(s, "\n")
	tabCount := 0
	spaceCounts := make(map[int]int)

	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" || trimmed == line {
			continue
		}
		indent := line[:len(line)-len(trimmed)]
		if strings.Contains(indent, "\t") {
			tabCount++
		} else {
			l := len(indent)
			spaceCounts[l]++
		}
	}

	totalSpaces := 0
	for _, c := range spaceCounts {
		totalSpaces += c
	}
	if tabCount > totalSpaces {
		return '\t', 1
	}

	allIndents := make([]int, 0, len(spaceCounts))
	for l := range spaceCounts {
		allIndents = append(allIndents, l)
	}
	if len(allIndents) == 0 {
		return ' ', 2
	}

	gcd := func(a, b int) int {
		for b != 0 {
			a, b = b, a%b
		}
		return a
	}
	base := allIndents[0]
	for _, v := range allIndents[1:] {
		base = gcd(base, v)
	}
	if base <= 1 {
		base = 2
	}
	return ' ', base
}

// normalizeIndent 规范化缩进（将单空格缩进转为双空格等）
func normalizeIndent(s string, targetIndent int) string {
	if s == "" || targetIndent <= 0 {
		return s
	}
	srcChar, srcSize := detectIndentStyle(s)
	if srcChar == ' ' && srcSize == targetIndent {
		return s
	}

	lines := strings.Split(s, "\n")
	for i, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" {
			continue
		}
		indent := line[:len(line)-len(trimmed)]
		var level int
		if srcChar == '\t' {
			level = strings.Count(indent, "\t")
		} else {
			spaceCount := len(strings.ReplaceAll(indent, "\t", ""))
			if srcSize > 0 {
				level = (spaceCount + srcSize/2) / srcSize
			}
		}
		lines[i] = strings.Repeat(" ", level*targetIndent) + trimmed
	}
	return strings.Join(lines, "\n")
}
