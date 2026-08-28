package conversation

import (
	"strings"
	"testing"
)

// Instructions, memory, and the skill catalog are all project-scoped and must
// live in the first system-reminder, not the system prompt — otherwise every
// project would have its own system prompt and cross-project caching would break.
func TestInjectLongTermMemoryCarriesSkills(t *testing.T) {
	m := NewManager()
	m.AddUserMessage("hello")
	m.InjectLongTermMemory("my instructions", "my memories", "- /pdf: fill forms")

	msgs := m.GetMessages()
	if len(msgs) != 2 {
		t.Fatalf("want 2 messages, got %d", len(msgs))
	}
	// The injected message must come first so the prefix position stays stable
	first := msgs[0]
	if first.Role != "user" {
		t.Errorf("injected message role = %q, want user", first.Role)
	}
	for _, want := range []string{"my instructions", "my memories", "- /pdf: fill forms"} {
		if !strings.Contains(first.Content, want) {
			t.Errorf("injected message missing %q:\n%s", want, first.Content)
		}
	}
	if !strings.HasPrefix(first.Content, "<system-reminder>") {
		t.Errorf("injected message should be wrapped in system-reminder, got:\n%s", first.Content)
	}
	if msgs[1].Content != "hello" {
		t.Errorf("original message should follow, got %q", msgs[1].Content)
	}
}

// Only one message is injected per session; repeated calls do not stack.
func TestInjectLongTermMemoryOnlyOnce(t *testing.T) {
	m := NewManager()
	m.InjectLongTermMemory("a", "b", "c")
	m.InjectLongTermMemory("a", "b", "c")

	if got := len(m.GetMessages()); got != 1 {
		t.Errorf("want 1 injected message, got %d", got)
	}
}

// No noise message is produced when all three sections are empty.
func TestInjectLongTermMemorySkipsWhenEmpty(t *testing.T) {
	m := NewManager()
	m.InjectLongTermMemory("", "", "")

	if got := len(m.GetMessages()); got != 0 {
		t.Errorf("want no message, got %d", got)
	}
}

// The skill catalog alone still triggers injection, since a project may have
// no SWIFTX.md and no memory.
func TestInjectLongTermMemorySkillsOnly(t *testing.T) {
	m := NewManager()
	m.InjectLongTermMemory("", "", "- /review: review code")

	msgs := m.GetMessages()
	if len(msgs) != 1 {
		t.Fatalf("want 1 message, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "- /review: review code") {
		t.Errorf("skill section missing:\n%s", msgs[0].Content)
	}
}
