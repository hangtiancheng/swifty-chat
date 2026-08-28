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

package commands

import (
	"fmt"
	"sort"
	"strings"
)

type CommandType string

const (
	TypeLocal   CommandType = "local"
	TypeLocalUI CommandType = "local-ui"
	TypePrompt  CommandType = "prompt"
	// TypeSkillFork is for skills declared with `mode: fork`. The handler
	// runs the skill in an isolated sub-agent (no main-loop touch) and
	// returns the final assistant text; the TUI dispatcher inserts that
	// text into the main chat as an assistant message instead of pumping
	// it through the regular Agent Loop.
	TypeSkillFork CommandType = "skill-fork"
)

type Context struct {
	Args           string
	MemoryList     func() []string
	MemoryClear    func()
	TokenCount     func() (input, output int)
	PermissionMode func() string
	ToolCount      func() int
	SessionInfo    func() string
	SkillList      func() []SkillInfo
	SkillReload    func() int // reload catalog + prompt, returns new skill count
	MCPInfo        func() string
	WorkDir        string
	Model          string
}

type SkillInfo struct {
	Name        string
	Description string
}

type Handler func(ctx *Context) string

type Command struct {
	Name        string
	Description string
	Aliases     []string
	Type        CommandType
	ArgPrompt   string
	Hidden      bool
	Handler     Handler
}

type Registry struct {
	commands map[string]*Command
	aliases  map[string]string
}

func NewRegistry() *Registry {
	return &Registry{
		commands: make(map[string]*Command),
		aliases:  make(map[string]string),
	}
}

func (r *Registry) Register(cmd *Command) {
	if _, exists := r.commands[cmd.Name]; exists {
		panic(fmt.Sprintf("commands: duplicate command name %q", cmd.Name))
	}
	if owner, exists := r.aliases[cmd.Name]; exists {
		panic(fmt.Sprintf("commands: command name %q collides with alias of %q", cmd.Name, owner))
	}
	for _, alias := range cmd.Aliases {
		if _, exists := r.commands[alias]; exists {
			panic(fmt.Sprintf("commands: alias %q for %q collides with existing command name", alias, cmd.Name))
		}
		if owner, exists := r.aliases[alias]; exists {
			panic(fmt.Sprintf("commands: alias %q for %q already registered by %q", alias, cmd.Name, owner))
		}
	}
	r.commands[cmd.Name] = cmd
	for _, alias := range cmd.Aliases {
		r.aliases[alias] = cmd.Name
	}
}

// HasConflict reports whether cmd's name or any of its aliases would collide
// with something already in the registry. Dynamic loaders (e.g. user
// commands from disk) should call this before Register to filter out
// conflicting entries, since Register panics on collision.
func (r *Registry) HasConflict(cmd *Command) bool {
	if r.Find(cmd.Name) != nil {
		return true
	}
	for _, alias := range cmd.Aliases {
		if r.Find(alias) != nil {
			return true
		}
	}
	return false
}

func (r *Registry) Find(name string) *Command {
	if cmd, ok := r.commands[name]; ok {
		return cmd
	}
	if canonical, ok := r.aliases[name]; ok {
		return r.commands[canonical]
	}
	return nil
}

func (r *Registry) ListCommands() []*Command {
	var cmds []*Command
	for _, cmd := range r.commands {
		if !cmd.Hidden {
			cmds = append(cmds, cmd)
		}
	}
	sort.Slice(cmds, func(i, j int) bool {
		return cmds[i].Name < cmds[j].Name
	})
	return cmds
}

func (r *Registry) Complete(prefix string) []string {
	var matches []string
	for name, cmd := range r.commands {
		if !cmd.Hidden && strings.HasPrefix(name, prefix) {
			matches = append(matches, name)
		}
	}
	for alias, canonical := range r.aliases {
		if cmd := r.commands[canonical]; cmd != nil && !cmd.Hidden && strings.HasPrefix(alias, prefix) {
			matches = append(matches, alias)
		}
	}
	sort.Strings(matches)
	return matches
}

func Parse(input string) (name string, args string) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "/") {
		return "", ""
	}
	input = input[1:]
	parts := strings.SplitN(input, " ", 2)
	name = strings.ToLower(parts[0])
	if len(parts) > 1 {
		args = strings.TrimSpace(parts[1])
	}
	return
}

func CreateDefaultRegistry() *Registry {
	r := NewRegistry()

	r.Register(&Command{
		Name:        "help",
		Description: "Show available commands",
		Aliases:     []string{"h", "?"},
		Type:        TypeLocal,
		Handler: func(ctx *Context) string {
			if ctx.Args != "" {
				cmd := r.Find(ctx.Args)
				if cmd == nil {
					return fmt.Sprintf("Unknown command: %s", ctx.Args)
				}
				var sb strings.Builder
				fmt.Fprintf(&sb, "/%s — %s\n", cmd.Name, cmd.Description)
				if len(cmd.Aliases) > 0 {
					fmt.Fprintf(&sb, "  Aliases: %s\n", strings.Join(cmd.Aliases, ", "))
				}
				if cmd.ArgPrompt != "" {
					fmt.Fprintf(&sb, "  Usage: %s\n", cmd.ArgPrompt)
				}
				return sb.String()
			}
			var sb strings.Builder
			sb.WriteString("Available commands:\n\n")
			for _, cmd := range r.ListCommands() {
				aliases := ""
				if len(cmd.Aliases) > 0 {
					aliases = ", /" + strings.Join(cmd.Aliases, ", /")
				}
				fmt.Fprintf(&sb, "  /%s%s\n    %s\n", cmd.Name, aliases, cmd.Description)
			}
			sb.WriteString("\nType /help <command> for details.")
			return sb.String()
		},
	})

	r.Register(&Command{
		Name:        "mcp",
		Description: "Show MCP server status",
		Type:        TypeLocal,
		Handler: func(ctx *Context) string {
			if ctx.MCPInfo == nil {
				return "No MCP servers configured"
			}
			info := ctx.MCPInfo()
			if info == "" {
				return "No MCP servers connected"
			}
			return info
		},
	})

	r.Register(&Command{
		Name:        "clear",
		Description: "Clear conversation and start fresh",
		Type:        TypeLocalUI,
	})

	r.Register(&Command{
		Name:        "compact",
		Description: "Compress conversation context",
		Aliases:     []string{"c"},
		Type:        TypeLocalUI,
	})

	r.Register(&Command{
		Name:        "status",
		Description: "Show current status",
		Aliases:     []string{"s"},
		Type:        TypeLocal,
		Handler: func(ctx *Context) string {
			var sb strings.Builder
			sb.WriteString("Swiftx Status\n")
			sb.WriteString("──────────────\n")
			fmt.Fprintf(&sb, "  Mode:      %s\n", ctx.PermissionMode())
			input, output := ctx.TokenCount()
			fmt.Fprintf(&sb, "  Tokens:    %d in / %d out\n", input, output)
			fmt.Fprintf(&sb, "  Tools:     %d enabled\n", ctx.ToolCount())
			memories := ctx.MemoryList()
			fmt.Fprintf(&sb, "  Memories:  %d entries\n", len(memories))
			fmt.Fprintf(&sb, "  Model:     %s\n", ctx.Model)
			fmt.Fprintf(&sb, "  Directory: %s\n", ctx.WorkDir)
			return sb.String()
		},
	})

	r.Register(&Command{
		Name:        "memory",
		Description: "Manage auto-memories",
		Type:        TypeLocal,
		Handler: func(ctx *Context) string {
			sub, subArgs := parseSubcommand(ctx.Args)

			switch sub {
			case "", "list":
				memories := ctx.MemoryList()
				if len(memories) == 0 {
					return "No memories stored yet."
				}
				var sb strings.Builder
				fmt.Fprintf(&sb, "Memories (%d entries):\n\n", len(memories))
				for i, mem := range memories {
					preview := mem
					if len(preview) > 80 {
						preview = preview[:80] + "…"
					}
					fmt.Fprintf(&sb, "  %d. %s\n", i+1, preview)
				}
				return sb.String()

			case "clear":
				ctx.MemoryClear()
				return "All auto-memories cleared."

			default:
				_ = subArgs
				return "Usage: /memory [list|clear]"
			}
		},
	})

	r.Register(&Command{
		Name:        "plan",
		Description: "Switch to plan mode (read-only)",
		Aliases:     []string{"p"},
		Type:        TypeLocalUI,
	})

	r.Register(&Command{
		Name:        "session",
		Description: "Session management",
		Type:        TypeLocal,
		Handler: func(ctx *Context) string {
			sub, _ := parseSubcommand(ctx.Args)
			switch sub {
			case "", "info":
				return ctx.SessionInfo()
			case "list":
				return ctx.SessionInfo()
			default:
				return "Usage: /session [list|info]"
			}
		},
	})

	r.Register(&Command{
		Name:        "resume",
		Description: "Resume a previous session",
		Aliases:     []string{"r"},
		Type:        TypeLocalUI,
	})

	r.Register(&Command{
		Name:        "rewind",
		Description: "Rewind conversation and files to a previous checkpoint",
		Type:        TypeLocalUI,
	})

	r.Register(&Command{
		Name:        "skills",
		Description: "List available skills (use '/skills reload' to hot-reload)",
		Type:        TypeLocal,
		Handler: func(ctx *Context) string {
			if strings.TrimSpace(ctx.Args) == "reload" {
				if ctx.SkillReload == nil {
					return "Skill reload not available."
				}
				count := ctx.SkillReload()
				return fmt.Sprintf("Skills reloaded. %d skill(s) available.", count)
			}
			skills := ctx.SkillList()
			if len(skills) == 0 {
				return "No skills installed.\n\nAdd skills to .swiftx/skills/<skill-name>/SKILL.md"
			}
			var sb strings.Builder
			fmt.Fprintf(&sb, "Available skills (%d):\n\n", len(skills))
			for _, s := range skills {
				desc := s.Description
				if len(desc) > 100 {
					desc = desc[:100] + "…"
				}
				fmt.Fprintf(&sb, "  /%s\n    %s\n\n", s.Name, desc)
			}
			sb.WriteString("Type /<skill-name> to invoke a skill.\n")
			sb.WriteString("Type /skills reload to hot-reload skills from disk.")
			return sb.String()
		},
	})

	r.Register(&Command{
		Name:        "sandbox",
		Description: "Configure OS-level sandbox for command execution",
		Type:        TypeLocalUI,
	})

	r.Register(&Command{
		Name:        "review",
		Description: "Review current code changes",
		Type:        TypePrompt,
		Handler: func(ctx *Context) string {
			prompt := "Please review the current git diff for code changes. Focus on:\n" +
				"1. Logic errors\n2. Security issues\n3. Performance problems\n4. Code style"
			if ctx.Args != "" {
				prompt += "\n\nAdditional focus: " + ctx.Args
			}
			return prompt
		},
	})

	return r
}

func parseSubcommand(args string) (sub string, rest string) {
	args = strings.TrimSpace(args)
	if args == "" {
		return "", ""
	}
	parts := strings.SplitN(args, " ", 2)
	sub = strings.ToLower(parts[0])
	if len(parts) > 1 {
		rest = strings.TrimSpace(parts[1])
	}
	return
}
