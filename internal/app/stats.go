package app

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/andyhtran/cct/internal/output"
	"github.com/andyhtran/cct/internal/session"
)

type StatsCmd struct{}

type statsData struct {
	Total          int           `json:"total_sessions"`
	Projects       int           `json:"unique_projects"`
	ThisWeek       int           `json:"sessions_this_week"`
	ThisMonth      int           `json:"sessions_this_month"`
	TopProjects    []projectStat `json:"top_projects"`
	RecentProjects []projectStat `json:"recent_projects"`
}

type projectStat struct {
	Name     string `json:"name"`
	Sessions int    `json:"sessions,omitempty"`
	LastUsed string `json:"last_used,omitempty"`
}

func (cmd *StatsCmd) Run(globals *Globals) error {
	files := session.DiscoverFiles("")
	if !globals.JSON && len(files) > 50 {
		fmt.Fprintf(os.Stderr, "Scanning %d sessions...\n", len(files))
	}
	sessions := session.ScanFiles(files, false)

	if len(sessions) == 0 {
		fmt.Println("  No sessions found.")
		return nil
	}

	now := time.Now()
	weekAgo := now.AddDate(0, 0, -7)
	monthAgo := now.AddDate(0, -1, 0)

	projectCounts := make(map[string]int)
	projectRecent := make(map[string]time.Time)
	thisWeek := 0
	thisMonth := 0

	for _, s := range sessions {
		name := s.ProjectName
		if name == "" {
			name = "(unknown)"
		}
		projectCounts[name]++

		if s.Modified.After(projectRecent[name]) {
			projectRecent[name] = s.Modified
		}
		if s.Modified.After(weekAgo) {
			thisWeek++
		}
		if s.Modified.After(monthAgo) {
			thisMonth++
		}
	}

	// Top projects by session count
	type kv struct {
		name  string
		count int
	}
	var sorted []kv
	for name, count := range projectCounts {
		sorted = append(sorted, kv{name, count})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].count > sorted[j].count
	})

	topN := 10
	if topN > len(sorted) {
		topN = len(sorted)
	}

	// Most recent projects
	type rv struct {
		name string
		when time.Time
	}
	var recent []rv
	for name, t := range projectRecent {
		recent = append(recent, rv{name, t})
	}
	sort.Slice(recent, func(i, j int) bool {
		return recent[i].when.After(recent[j].when)
	})

	recentN := 5
	if recentN > len(recent) {
		recentN = len(recent)
	}

	if globals.JSON {
		data := statsData{
			Total:     len(sessions),
			Projects:  len(projectCounts),
			ThisWeek:  thisWeek,
			ThisMonth: thisMonth,
		}
		for _, kv := range sorted[:topN] {
			data.TopProjects = append(data.TopProjects, projectStat{
				Name:     kv.name,
				Sessions: kv.count,
			})
		}
		for _, rv := range recent[:recentN] {
			data.RecentProjects = append(data.RecentProjects, projectStat{
				Name:     rv.name,
				LastUsed: output.FormatAge(rv.when),
			})
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(data)
	}

	fmt.Println()
	fmt.Printf("  %s  %d\n", output.Pad("Sessions:", 12, output.Dim), len(sessions))
	fmt.Printf("  %s  %d\n", output.Pad("Projects:", 12, output.Dim), len(projectCounts))
	fmt.Printf("  %s  %d\n", output.Pad("This week:", 12, output.Dim), thisWeek)
	fmt.Printf("  %s  %d\n", output.Pad("This month:", 12, output.Dim), thisMonth)

	fmt.Println()
	fmt.Println("  " + output.Bold("Top Projects (by sessions)"))
	for _, kv := range sorted[:topN] {
		fmt.Printf("    %s  %s\n", output.Pad(output.Truncate(kv.name, 30), 30, output.Bold), output.Dim(fmt.Sprintf("%d sessions", kv.count)))
	}

	fmt.Println()
	fmt.Println("  " + output.Bold("Most Recent Projects"))
	for _, rv := range recent[:recentN] {
		fmt.Printf("    %s  %s\n", output.Pad(output.Truncate(rv.name, 30), 30, output.Bold), output.Dim(output.FormatAge(rv.when)+" ago"))
	}
	fmt.Println()

	return nil
}
