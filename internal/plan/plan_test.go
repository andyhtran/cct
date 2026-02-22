package plan

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractTitle(t *testing.T) {
	dir := t.TempDir()

	t.Run("with heading", func(t *testing.T) {
		path := filepath.Join(dir, "with-heading.md")
		if err := os.WriteFile(path, []byte("# My Great Plan\n\nSome content here.\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		got := extractTitle(path)
		if got != "My Great Plan" {
			t.Errorf("extractTitle() = %q, want %q", got, "My Great Plan")
		}
	})

	t.Run("heading after blank lines", func(t *testing.T) {
		path := filepath.Join(dir, "blank-lines.md")
		if err := os.WriteFile(path, []byte("\n\n# Delayed Heading\n\nContent.\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		got := extractTitle(path)
		if got != "Delayed Heading" {
			t.Errorf("extractTitle() = %q, want %q", got, "Delayed Heading")
		}
	})

	t.Run("no heading", func(t *testing.T) {
		path := filepath.Join(dir, "no-heading.md")
		if err := os.WriteFile(path, []byte("Just some text\nNo headings here\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		got := extractTitle(path)
		if got != "" {
			t.Errorf("extractTitle() = %q, want empty", got)
		}
	})

	t.Run("h2 not matched", func(t *testing.T) {
		path := filepath.Join(dir, "h2.md")
		if err := os.WriteFile(path, []byte("## This is H2\n\nNot matched.\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		got := extractTitle(path)
		if got != "" {
			t.Errorf("extractTitle() = %q, want empty (h2 should not match)", got)
		}
	})
}

func setupPlanFixtures(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)

	plansDir := filepath.Join(home, ".claude", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(plansDir, "auth-refactor.md"), []byte("# Auth Refactor\n\nOverhaul the authentication layer.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plansDir, "db-migration.md"), []byte("# Database Migration\n\nMigrate from MySQL to PostgreSQL.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFindPlan_Isolated(t *testing.T) {
	setupPlanFixtures(t)

	t.Run("exact match", func(t *testing.T) {
		p, err := FindPlan("auth-refactor")
		if err != nil {
			t.Fatal(err)
		}
		if p.Name != "auth-refactor" {
			t.Errorf("Name = %q, want auth-refactor", p.Name)
		}
	})

	t.Run("partial match", func(t *testing.T) {
		p, err := FindPlan("db-mig")
		if err != nil {
			t.Fatal(err)
		}
		if p.Name != "db-migration" {
			t.Errorf("Name = %q, want db-migration", p.Name)
		}
	})

	t.Run("no match", func(t *testing.T) {
		_, err := FindPlan("nonexistent")
		if err == nil {
			t.Fatal("expected error for no match")
		}
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("error = %q, want ErrNotFound", err)
		}
	})
}

func TestSearchPlans(t *testing.T) {
	setupPlanFixtures(t)

	t.Run("keyword found", func(t *testing.T) {
		matches, err := SearchPlans("authentication", 80)
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) != 1 {
			t.Fatalf("expected 1 match, got %d", len(matches))
		}
		if matches[0].Plan.Name != "auth-refactor" {
			t.Errorf("matched plan = %q, want auth-refactor", matches[0].Plan.Name)
		}
	})

	t.Run("no match", func(t *testing.T) {
		matches, err := SearchPlans("zzz_nothing_zzz", 80)
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) != 0 {
			t.Errorf("expected 0 matches, got %d", len(matches))
		}
	})

	t.Run("case insensitive", func(t *testing.T) {
		matches, err := SearchPlans("MYSQL", 80)
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) != 1 {
			t.Errorf("expected 1 match for case-insensitive search, got %d", len(matches))
		}
	})
}

func TestCopyPlan(t *testing.T) {
	setupPlanFixtures(t)
	destDir := t.TempDir()

	t.Run("default name", func(t *testing.T) {
		dest, err := CopyPlan("auth-refactor", destDir, "")
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Base(dest) != "auth-refactor.md" {
			t.Errorf("dest = %q, want auth-refactor.md", filepath.Base(dest))
		}
		data, err := os.ReadFile(dest)
		if err != nil {
			t.Fatal(err)
		}
		if len(data) == 0 {
			t.Error("expected non-empty copied file")
		}
	})

	t.Run("renamed", func(t *testing.T) {
		dest, err := CopyPlan("auth-refactor", destDir, "my-plan.md")
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Base(dest) != "my-plan.md" {
			t.Errorf("dest = %q, want my-plan.md", filepath.Base(dest))
		}
	})

	t.Run("auto md extension", func(t *testing.T) {
		dest, err := CopyPlan("auth-refactor", destDir, "another")
		if err != nil {
			t.Fatal(err)
		}
		if filepath.Base(dest) != "another.md" {
			t.Errorf("dest = %q, want another.md", filepath.Base(dest))
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := CopyPlan("nonexistent", destDir, "")
		if err == nil {
			t.Fatal("expected error for nonexistent plan")
		}
	})
}

func TestListPlans(t *testing.T) {
	setupPlanFixtures(t)

	plans, err := ListPlans()
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 2 {
		t.Fatalf("expected 2 plans, got %d", len(plans))
	}

	titles := map[string]string{}
	for _, p := range plans {
		titles[p.Name] = p.Title
	}
	if titles["auth-refactor"] != "Auth Refactor" {
		t.Errorf("auth-refactor title = %q, want Auth Refactor", titles["auth-refactor"])
	}
	if titles["db-migration"] != "Database Migration" {
		t.Errorf("db-migration title = %q, want Database Migration", titles["db-migration"])
	}
}
