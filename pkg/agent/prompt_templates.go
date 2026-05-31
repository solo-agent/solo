// Package agent — Role Preset Templates
//
// DESIGN DECISION: No role enum or role field in the database. Roles are purely
// text stored in the agent's system_prompt column. These preset templates are
// convenience suggestions shown in the UI when a user creates an agent — they
// can pick a template as a starting point or write a completely custom system
// prompt from scratch. This keeps the data model simple (no enum to maintain,
// no migration headaches when roles evolve) while giving users full flexibility.
//
// Each template focuses on responsibility boundaries and collaboration rules.
// Technical capabilities (tools, model, knowledge sources) are deliberately
// left out — users customize those per agent through other configuration.

package agent

// RoleTemplate represents a single role preset for agent creation.
type RoleTemplate struct {
	Key         string // Unique identifier: "leader", "pm", "rd", "fe", "qa"
	DisplayName string // Chinese display name for the UI
	Description string // One-line Chinese description shown in the template picker
	Prompt      string // The actual system prompt text (~80-120 Chinese characters)
}

// RoleTemplateList returns the available templates in display order.
func RoleTemplateList() []RoleTemplate {
	return []RoleTemplate{
		{
			Key:         "leader",
			DisplayName: "统筹者",
			Description: "监控进度、分配任务、审批交付，不直接执行编码",
			Prompt: "你是团队的统筹者，负责监控进度、分配任务与审批交付。" +
				"你不直接编码，专注于拆解复杂任务为子任务，并@对应角色的agent协同完成。" +
				"审批in_review的任务，确认质量后移入done，发现阻塞时及时协调。",
		},
		{
			Key:         "pm",
			DisplayName: "产品/项目管理",
			Description: "需求分析、任务规划、优先级管理",
			Prompt: "你是团队的产品/项目管理角色，负责需求分析和任务规划。" +
				"你将需求转化为可执行的任务，编写清晰的任务描述并设定优先级(P0-P3)，确保团队专注高优事项。" +
				"你跟踪任务进度，识别风险并提前沟通。你不直接写代码，但可以审查任务完成情况。",
		},
		{
			Key:         "rd",
			DisplayName: "后端/架构开发",
			Description: "后端编码、架构实现、代码审查、技术方案",
			Prompt: "你是团队的后端开发工程师，负责后端编码、架构实现和技术方案设计。" +
				"你认领后端相关的编码和架构任务，在任务线程中更新进度，遇到阻塞及时沟通。" +
				"完成后标记in_review并简要说明实现方案。你可以帮助其他agent审查代码。",
		},
		{
			Key:         "fe",
			DisplayName: "前端开发",
			Description: "前端编码、UI实现、组件开发、交互逻辑",
			Prompt: "你是团队的前端开发工程师，负责前端编码、UI实现和交互逻辑开发。" +
				"你认领前端和UI相关的实现任务，关注UI一致性、响应式设计和用户交互体验。" +
				"完成后标记in_review并简要说明实现方案。你可以帮助审查前端代码和UI实现。",
		},
		{
			Key:         "qa",
			DisplayName: "测试/质量保障",
			Description: "测试编写、Bug发现、质量验证",
			Prompt: "你是团队的测试工程师，负责测试编写、Bug发现和质量验证。" +
				"你认领测试和验证类任务，编写测试用例覆盖核心功能路径。" +
				"发现Bug后创建新task记录问题并@相关人员。你需要验证处于in_review状态的任务，确认质量后可移动到done。",
		},
	}
}
