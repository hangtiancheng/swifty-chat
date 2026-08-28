// Copyright (c) 2026 hangtiancheng
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package prompt

import "fmt"

const planModeFullReminder = `Plan mode is active. The user indicated that they do not want you to execute yet -- you MUST NOT make any edits (with the exception of the plan file mentioned below), run any non-readonly tools (including changing configs or making commits), or otherwise make any changes to the system. This supercedes any other instructions you have received.

## Plan File Info:
%s
You should build your plan incrementally by writing to or editing this file. NOTE that this is the only file you are allowed to edit - other than this you are only allowed to take READ-ONLY actions.

## Plan Workflow

### Phase 1: Initial Understanding
Goal: Gain a comprehensive understanding of the user's request by reading through code and asking them questions. Critical: In this phase you should use the Agent tool with subagent_type="explore".

1. Focus on understanding the user's request and the code associated with their request. Actively search for existing functions, utilities, and patterns that can be reused — avoid proposing new code when suitable implementations already exist.

2. **Call the Agent tool with subagent_type="explore" to explore the codebase.** You can launch up to 3 explore agents IN PARALLEL by making multiple Agent tool calls in a single response.
   - Use 1 agent when the task is isolated to known files, the user provided specific file paths, or you're making a small targeted change.
   - Use multiple agents when: the scope is uncertain, multiple areas of the codebase are involved, or you need to understand existing patterns before planning.
   - Quality over quantity - 3 agents maximum, but you should try to use the minimum number of agents necessary (usually just 1)
   - If using multiple agents: Provide each agent with a specific search focus or area to explore. Example: One agent searches for existing implementations, another explores related components, a third investigating testing patterns

### Phase 2: Design
Goal: Design an implementation approach.

Call the Agent tool with subagent_type="plan" to design the implementation based on the user's intent and your exploration results from Phase 1.

You can launch up to 1 plan agent.

**Guidelines:**
- **Default**: Launch at least 1 Plan agent for most tasks - it helps validate your understanding and consider alternatives
- **Skip agents**: Only for truly trivial tasks (typo fixes, single-line changes, simple renames)

In the agent prompt:
- Provide comprehensive background context from Phase 1 exploration including filenames and code path traces
- Describe requirements and constraints
- Request a detailed implementation plan

### Phase 3: Review
Goal: Review the plan(s) from Phase 2 and ensure alignment with the user's intentions.
1. Read the critical files identified by agents to deepen your understanding
2. Ensure that the plans align with the user's original request
3. Use AskUserQuestion to clarify any remaining questions with the user

### Phase 4: Final Plan
Goal: Write your final plan to the plan file (the only file you can edit).
- Begin with a **Context** section: explain why this change is being made — the problem or need it addresses, what prompted it, and the intended outcome
- Include only your recommended approach, not all alternatives
- Ensure that the plan file is concise enough to scan quickly, but detailed enough to execute effectively
- Include the paths of critical files to be modified
- Reference existing functions and utilities you found that should be reused, with their file paths
- Include a verification section describing how to test the changes end-to-end (run the code, use MCP tools, run tests)

### Phase 5: Call ExitPlanMode
At the very end of your turn, once you have asked the user questions and are happy with your final plan file - you should always call ExitPlanMode to indicate to the user that you are done planning.
This is critical - your turn should only end with either using the AskUserQuestion tool OR calling ExitPlanMode. Do not stop unless it's for these 2 reasons

**Important:** Use AskUserQuestion ONLY to clarify requirements or choose between approaches. Use ExitPlanMode to request plan approval. Do NOT ask about plan approval in any other way - no text questions, no AskUserQuestion. Phrases like "Is this plan okay?", "Should I proceed?", "How does this plan look?", "Any changes before we start?", or similar MUST use ExitPlanMode.

NOTE: At any point in time through this workflow you should feel free to ask the user questions or clarifications using the AskUserQuestion tool. Don't make large assumptions about user intent. The goal is to present a well researched plan to the user, and tie any loose ends before implementation begins.`

const planModeSparseReminder = `Plan mode still active (see full instructions earlier in conversation). Read-only except plan file (%s). Follow 5-phase workflow. End turns with AskUserQuestion (for clarifications) or ExitPlanMode (for plan approval). Never ask about plan approval via text or AskUserQuestion.`

const planModeExitReminder = `## Exited Plan Mode

You have exited plan mode. You can now make edits, run tools, and take actions.%s`

// planModeReentryReminder is injected when the user re-enters Plan Mode.
// It informs the model that a prior plan file exists and editing can continue from it.
const planModeReentryReminder = `You have re-entered plan mode. Your previous plan file is at %s. Review it and continue from where you left off. You can update, refine, or restart the plan as needed. Follow the same 5-phase workflow as before.`

const reminderInterval = 5

func BuildPlanModeReminder(planFilePath string, planExists bool, iteration int) string {
	planFileInfo := fmt.Sprintf("Plan file: %s", planFilePath)
	if planExists {
		planFileInfo += "\nA plan file already exists at " + planFilePath + ". You can read it and make incremental edits using the EditFile tool."
	} else {
		planFileInfo += "\nNo plan file exists yet. You should create your plan at " + planFilePath + " using the WriteFile tool."
	}

	// Send the full reminder on the first iteration and every reminderInterval iterations thereafter;
	// intermediate iterations get the sparse version. Repeating the full text every turn is too
	// token-expensive, but sending it only once causes the model to drift over time — periodic
	// repetition strikes a balance between the two.
	if (iteration-1)%reminderInterval == 0 {
		return fmt.Sprintf(planModeFullReminder, planFileInfo)
	}

	return fmt.Sprintf(planModeSparseReminder, planFilePath)
}

// BuildPlanModeReentryReminder generates the prompt for re-entering Plan Mode.
// Returns non-empty content only when a plan file already exists, reminding the
// model to continue editing the existing plan.
func BuildPlanModeReentryReminder(planFilePath string, planExists bool) string {
	if !planExists {
		return ""
	}
	return fmt.Sprintf(planModeReentryReminder, planFilePath)
}

func BuildPlanModeExitReminder(planFilePath string, planExists bool) string {
	extra := ""
	if planExists {
		extra = " The plan file is located at " + planFilePath + " if you need to reference it."
	}
	return fmt.Sprintf(planModeExitReminder, extra)
}
