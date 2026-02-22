package paths

import "testing"

func TestClaudeDir(t *testing.T) {
	t.Setenv("HOME", "/tmp/fakehome")
	got := ClaudeDir()
	if got != "/tmp/fakehome/.claude" {
		t.Errorf("ClaudeDir() = %q, want /tmp/fakehome/.claude", got)
	}
}

func TestProjectsDir(t *testing.T) {
	t.Setenv("HOME", "/tmp/fakehome")
	got := ProjectsDir()
	if got != "/tmp/fakehome/.claude/projects" {
		t.Errorf("ProjectsDir() = %q, want /tmp/fakehome/.claude/projects", got)
	}
}
