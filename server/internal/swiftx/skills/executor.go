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

package skills

import (
	"context"

	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/conversation"
	"github.com/hangtiancheng/swifty-chat/server/internal/swiftx/tools"
)

// SkillHost is the slice of Agent state that the Executor needs to drive
// inline-mode skills. Implemented by *agent.Agent; declared as an interface
// here so the skills package doesn't import the agent package (would create
// a cycle once LoadSkillTool starts referencing skills.Catalog).
type SkillHost interface {
	// ActivateSkill records the skill activation for tracking (/skills listing
	// and compaction recovery). The body is NOT re-injected every turn.
	ActivateSkill(name, body string)
	// ToolRegistry exposes the live tools.Registry so the executor can
	// register directory-type tools. Named ToolRegistry (not Registry) because
	// *agent.Agent already has an exported Registry field and Go forbids
	// method/field name collision.
	ToolRegistry() *tools.Registry
}

// SkillForkHost extends SkillHost with the ability to run an isolated
// sub-agent. Implemented by the TUI layer (which owns the LLM client +
// agent constructor) and passed into Executor.RunFork. Keeping it separate
// from SkillHost lets unit tests stub fork-only behaviour without faking
// the full sub-agent runtime.
type SkillForkHost interface {
	SkillHost
	// RunSubAgent runs `body` as the first user message in a fresh
	// conversation seeded with `seed` (already prepared per ForkContext
	// strategy) and returns the final assistant text. ctx cancellation
	// should abort the sub-agent.
	RunSubAgent(ctx context.Context, body string, seed []conversation.Message, model string) (string, error)
	// SnapshotParentMessages exposes the parent conversation messages so
	// the executor can build the seed per `fork_context`. Implementations
	// may return a shallow copy; the executor must not mutate the slice.
	SnapshotParentMessages() []conversation.Message
}

// RunInline records the skill activation on the host agent and returns the
// rendered prompt body. The caller (a slash-command handler) submits the
// returned body as a user message in the main conversation — it lives there
// as a regular message, not re-injected every turn.
func RunInline(_ context.Context, skill *Skill, args string, host SkillHost) (string, error) {
	body := skill.Render(args)
	host.ActivateSkill(skill.Meta.Name, body)
	return body, nil
}

// RunFork executes the skill in an isolated sub-agent and returns the
// final assistant text. The main conversation is not modified by the
// sub-agent; the caller (slash-command handler) is expected to insert
// the returned string into the main chat history as an assistant message.
//
// History carry-over is selected by skill.Meta.ForkContext:
//   - "full":  seed the sub-agent with the parent's full message history
//   - "recent": seed with the last 5 parent messages
//   - "none":  no seed (default; isolated like a fresh session)
func RunFork(ctx context.Context, skill *Skill, args string, host SkillForkHost) (string, error) {
	body := skill.Render(args)
	seed := buildForkSeed(skill.Meta.ForkContext, host.SnapshotParentMessages())
	return host.RunSubAgent(ctx, body, seed, skill.Meta.Model)
}

// buildForkSeed slices the parent message history according to the
// ForkContext strategy. Returns nil for "none" or unknown values so the
// sub-agent starts fresh.
//
// "full" currently performs no LLM-side summarisation — it copies the
// parent slice verbatim. A future refinement could route through
// compact.Summarise if context windows become a concern; for now keeping
// it identical to "recent" with a higher cap is sufficient.
func buildForkSeed(mode string, parent []conversation.Message) []conversation.Message {
	switch mode {
	case "full":
		out := make([]conversation.Message, len(parent))
		copy(out, parent)
		return out
	case "recent":
		if len(parent) <= 5 {
			out := make([]conversation.Message, len(parent))
			copy(out, parent)
			return out
		}
		out := make([]conversation.Message, 5)
		copy(out, parent[len(parent)-5:])
		return out
	default:
		return nil
	}
}
