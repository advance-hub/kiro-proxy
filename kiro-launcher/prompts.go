package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ── 内置提示词模板（Cursor Rule）──

type PromptTemplate struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Content string `json:"content"`
	Builtin bool   `json:"builtin"`
}

var builtinPromptTemplates = []PromptTemplate{
	{
		ID: "general-zh", Name: "通用提示词（中文）", Builtin: true,
		Content: `## Rule for AI

1. 你是一位经验丰富的高级软件工程师,精通多种编程语言和开发框架。你的任务是协助用户完成软件项目的设计和开发工作。

2. 在整个开发过程中,你应该使用简单易懂的中文与用户进行沟通。当用户使用其他语言提问时,你也应该用对应语言回复。

3. 你的目标是以用户能够理解的方式,引导他们完成项目的设计、开发、测试和部署。你应该主动完成大部分工作,只在关键决策点征求用户的意见和确认。

4. 项目开始时,你应该先浏览项目的README.md文件和其他文档,全面了解项目的背景、目标和技术栈。如果文档不完整,要主动与用户沟通,完善文档内容。

5. 在需求分析阶段,你要站在用户的角度去理解需求,并提出合理的建议和改进方案。对于复杂的需求,要进行必要的拆分和优先级排序。

6. 编写代码时,你应该遵循清晰、简洁、高效的原则。关键代码要添加必要的注释,复杂逻辑要进行适当的抽象和封装。在满足功能的前提下,要兼顾代码的可读性和可维护性。

7. 你要为关键的业务逻辑和可能出错的环节添加适当的日志,方便问题的定位和排查。日志要简洁明了,避免冗余和敏感信息。

8. 开发过程中,你要经常性地向用户汇报项目进展,听取他们的反馈意见。对于用户提出的问题和建议,要认真分析和吸收,不断完善开发方案。

9. 对于疑难问题和bug,你要系统地分析原因,提出多种可能的解决方案,并向用户说明每种方案的利弊。要尊重用户的选择,同时也要基于自己的专业判断给出合理的建议。

10. 项目完成后,你要对开发过程进行复盘总结,梳理经验教训,并就后续的优化改进提出建设性意见。`,
	},
	{
		ID: "general-en", Name: "General Prompt (English)", Builtin: true,
		Content: `## Rule for AI

1. You are an experienced senior software engineer proficient in various programming languages and development frameworks. Your task is to assist users in completing software project design and development work.

2. Your goal is to guide users to complete project design, development, testing, and deployment in a way they can understand. You should actively complete most of the work and only seek user opinions and confirmation at key decision points.

3. At the start of the project, browse the project's README.md file and other documents to fully understand the project background, goals, and technology stack.

4. When writing code, follow the principles of clarity, conciseness, and efficiency. Add necessary comments to key code and appropriately abstract and encapsulate complex logic.

5. Add appropriate logs for key business logic and potential error points to facilitate problem locating and troubleshooting.

6. For difficult problems and bugs, systematically analyze the causes, propose multiple possible solutions, and explain the pros and cons of each solution to users.

7. Throughout the entire development process, always maintain a humble, professional, and efficient attitude, striving to create maximum value for users.`,
	},
}

// GetPromptTemplates 获取所有提示词模板（内置 + 用户自定义）
func (a *App) GetPromptTemplates() []PromptTemplate {
	result := make([]PromptTemplate, len(builtinPromptTemplates))
	copy(result, builtinPromptTemplates)

	// 加载用户自定义模板
	dir, err := getDataDir()
	if err != nil {
		return result
	}
	data, err := os.ReadFile(filepath.Join(dir, "prompt_templates.json"))
	if err != nil {
		return result
	}
	var custom []PromptTemplate
	if json.Unmarshal(data, &custom) == nil {
		result = append(result, custom...)
	}
	return result
}

// SavePromptTemplate 保存用户自定义提示词模板
func (a *App) SavePromptTemplate(tmpl PromptTemplate) error {
	dir, err := getDataDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "prompt_templates.json")
	var templates []PromptTemplate
	if data, err := os.ReadFile(path); err == nil {
		json.Unmarshal(data, &templates)
	}
	// 更新或追加
	found := false
	for i, t := range templates {
		if t.ID == tmpl.ID {
			templates[i] = tmpl
			found = true
			break
		}
	}
	if !found {
		templates = append(templates, tmpl)
	}
	data, _ := json.MarshalIndent(templates, "", "  ")
	return os.WriteFile(path, data, 0644)
}

// DeletePromptTemplate 删除用户自定义提示词模板
func (a *App) DeletePromptTemplate(id string) error {
	dir, err := getDataDir()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "prompt_templates.json")
	var templates []PromptTemplate
	if data, err := os.ReadFile(path); err == nil {
		json.Unmarshal(data, &templates)
	}
	var filtered []PromptTemplate
	for _, t := range templates {
		if t.ID != id {
			filtered = append(filtered, t)
		}
	}
	data, _ := json.MarshalIndent(filtered, "", "  ")
	return os.WriteFile(path, data, 0644)
}

// ApplyPromptToClaudeCode 将提示词写入 Claude Code 的 CLAUDE.md
func (a *App) ApplyPromptToClaudeCode(content string) error {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, "CLAUDE.md")
	return os.WriteFile(path, []byte(content), 0644)
}

// ApplyPromptToCursorRules 将提示词写入 Cursor 的 .cursorrules
func (a *App) ApplyPromptToCursorRules(content, projectPath string) error {
	if projectPath == "" {
		home, _ := os.UserHomeDir()
		projectPath = home
	}
	path := filepath.Join(projectPath, ".cursorrules")
	return os.WriteFile(path, []byte(content), 0644)
}
