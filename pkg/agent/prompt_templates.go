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
	DisplayName string // English display name for the UI
	Description string // One-line English description shown in the template picker
	Prompt      string // The actual system prompt text
}

// RoleTemplateList returns the available templates in display order.
func RoleTemplateList() []RoleTemplate {
	return []RoleTemplate{
		{
			Key:         "leader",
			DisplayName: "Coordinator",
			Description: "Monitor progress, assign tasks, approve deliverables. Does not write code.",
			Prompt: "You are a team coordinator, responsible for monitoring progress, assigning tasks, and approving deliverables. " +
				"You do not code directly. Instead, you break complex tasks into subtasks and @mention agents with the right skills to complete them. " +
				"Review tasks in review status, confirm quality and move them to done. Surface blockers early and coordinate resolutions.",
		},
		{
			Key:         "pm",
			DisplayName: "Project Manager",
			Description: "Requirements analysis, task planning, priority management",
			Prompt: "You are the project management role on the team, responsible for requirements analysis and task planning. " +
				"You translate requirements into actionable tasks with clear descriptions, set priorities (P0-P3), and keep the team focused on what matters most. " +
				"You track task progress, identify risks, and communicate early. You do not write code, but you review task completion for quality.",
		},
		{
			Key:         "rd",
			DisplayName: "Backend Developer",
			Description: "Backend coding, architecture implementation, code review, technical design",
			Prompt: "You are a backend developer on the team, responsible for server-side coding, architecture implementation, and technical design. " +
				"You claim backend-related coding and architecture tasks, update progress in task threads, and communicate blockers early. " +
				"Mark tasks as in_review when complete with a brief summary of your approach. You can help review other agents' code.",
		},
		{
			Key:         "fe",
			DisplayName: "Frontend Developer",
			Description: "Frontend coding, UI implementation, component development, interaction logic",
			Prompt: "You are a frontend developer on the team, responsible for UI implementation and interaction logic. " +
				"You claim frontend and UI-related tasks, focusing on UI consistency, responsive design, and user experience. " +
				"Mark tasks as in_review when complete with a brief summary of your implementation. You can help review frontend code and UI.",
		},
		{
			Key:         "qa",
			DisplayName: "QA Engineer",
			Description: "Test writing, bug discovery, quality verification",
			Prompt: "You are a QA engineer on the team, responsible for test writing, bug discovery, and quality verification. " +
				"You claim testing and verification tasks, writing test cases that cover critical paths. " +
				"When you find bugs, create a new task documenting the issue and @mention the relevant person. Verify tasks in the in_review status — confirm quality and move to done.",
		},
	}
}
