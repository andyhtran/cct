package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/andyhtran/cct/internal/plan"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
)

// PlanModel renders a single plan markdown file in a scrollable viewport.
type PlanModel struct {
	plan     *plan.Plan
	viewport viewport.Model
	renderer *glamour.TermRenderer
	ready    bool
	width    int
	height   int
}

func NewPlanModel(p *plan.Plan) PlanModel {
	renderer, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(0),
	)

	return PlanModel{
		plan:     p,
		renderer: renderer,
	}
}

func (m PlanModel) Init() tea.Cmd {
	return nil
}

func (m PlanModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		case "g":
			m.viewport.GotoTop()
		case "G":
			m.viewport.GotoBottom()
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		headerHeight := 3
		footerHeight := 1

		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-headerHeight-footerHeight)
			m.viewport.YPosition = headerHeight
			m.viewport.SetContent(m.renderContent())
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - headerHeight - footerHeight
			m.viewport.SetContent(m.renderContent())
		}
	}

	m.viewport, cmd = m.viewport.Update(msg)
	return m, cmd
}

func (m PlanModel) View() string {
	if !m.ready {
		return "Loading..."
	}

	header := m.renderHeader()
	footer := m.renderFooter()

	return fmt.Sprintf("%s\n%s\n%s", header, m.viewport.View(), footer)
}

func (m PlanModel) renderHeader() string {
	title := fmt.Sprintf(" %s ", m.plan.Name)
	if m.plan.Title != "" {
		title += fmt.Sprintf("• %s ", m.plan.Title)
	}

	line := strings.Repeat("─", max(0, m.width-len(title)-2))
	return separatorStyle.Render(fmt.Sprintf("─%s%s─", title, line))
}

func (m PlanModel) renderFooter() string {
	scroll := fmt.Sprintf(" %3.f%% ", m.viewport.ScrollPercent()*100)
	help := " q: quit • j/k: scroll • g/G: top/bottom "

	gap := m.width - len(scroll) - len(help)
	if gap < 0 {
		gap = 0
	}

	return helpStyle.Render(strings.Repeat(" ", gap) + help + scroll)
}

func (m PlanModel) renderContent() string {
	content, err := os.ReadFile(m.plan.Path)
	if err != nil {
		return fmt.Sprintf("Error reading plan: %v", err)
	}

	if m.renderer == nil {
		return string(content)
	}

	rendered, err := m.renderer.Render(string(content))
	if err != nil {
		return string(content)
	}

	return strings.TrimSpace(rendered)
}

// RunPlan opens an interactive TUI viewer for a plan file.
func RunPlan(p *plan.Plan) error {
	m := NewPlanModel(p)
	prog := tea.NewProgram(m, tea.WithAltScreen())

	_, err := prog.Run()
	return err
}
