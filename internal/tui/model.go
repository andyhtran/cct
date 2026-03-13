package tui

import (
	"fmt"
	"os"
	"strings"

	"github.com/andyhtran/cct/internal/session"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

var (
	userStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("12")).
			Bold(true)

	assistantStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("5")).
			Bold(true)

	toolStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	toolNameStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("3"))

	separatorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))

	helpStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8"))
)

type Model struct {
	session  *session.Session
	messages []Message
	viewport viewport.Model
	renderer *glamour.TermRenderer
	ready    bool
	width    int
	height   int
}

func NewModel(s *session.Session, messages []Message) Model {
	renderer, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(0),
	)

	return Model{
		session:  s,
		messages: messages,
		renderer: renderer,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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

func (m Model) View() string {
	if !m.ready {
		return "Loading..."
	}

	header := m.renderHeader()
	footer := m.renderFooter()

	return fmt.Sprintf("%s\n%s\n%s", header, m.viewport.View(), footer)
}

func (m Model) renderHeader() string {
	title := fmt.Sprintf(" Session %s ", m.session.ShortID)
	if m.session.ProjectName != "" {
		title += fmt.Sprintf("• %s ", m.session.ProjectName)
	}
	if m.session.GitBranch != "" {
		title += fmt.Sprintf("(%s) ", m.session.GitBranch)
	}

	line := strings.Repeat("─", max(0, m.width-len(title)-2))
	return separatorStyle.Render(fmt.Sprintf("─%s%s─", title, line))
}

func (m Model) renderFooter() string {
	info := fmt.Sprintf(" %d messages ", len(m.messages))
	scroll := fmt.Sprintf(" %3.f%% ", m.viewport.ScrollPercent()*100)
	help := " q: quit • j/k: scroll • g/G: top/bottom "

	gap := m.width - len(info) - len(scroll) - len(help)
	if gap < 0 {
		gap = 0
	}

	return helpStyle.Render(info + strings.Repeat(" ", gap) + help + scroll)
}

func (m Model) renderContent() string {
	var b strings.Builder

	for i, msg := range m.messages {
		switch msg.Kind {
		case KindUser:
			b.WriteString(userStyle.Render("▌ User"))
			b.WriteString("\n\n")
			rendered := m.renderMarkdown(msg.Text)
			b.WriteString(rendered)

		case KindAssistant:
			b.WriteString(assistantStyle.Render("▌ Assistant"))
			b.WriteString("\n\n")
			rendered := m.renderMarkdown(msg.Text)
			b.WriteString(rendered)

		case KindToolCall:
			toolLine := fmt.Sprintf("  ▸ %s", toolNameStyle.Render(msg.ToolName))
			if msg.Text != "" {
				toolLine += toolStyle.Render(fmt.Sprintf(" — %s", truncate(msg.Text, 60)))
			}
			b.WriteString(toolStyle.Render(toolLine))
			b.WriteString("\n")
			continue

		case KindToolResult:
			continue
		}

		if i < len(m.messages)-1 {
			b.WriteString("\n")
			b.WriteString(separatorStyle.Render(strings.Repeat("─", m.width)))
			b.WriteString("\n\n")
		}
	}

	return b.String()
}

func (m Model) renderMarkdown(text string) string {
	if m.renderer == nil {
		return text
	}
	rendered, err := m.renderer.Render(text)
	if err != nil {
		return text
	}
	return strings.TrimSpace(rendered)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func Run(s *session.Session) error {
	f, err := os.Open(s.FilePath)
	if err != nil {
		return fmt.Errorf("cannot open session file: %w", err)
	}
	defer func() { _ = f.Close() }()

	messages := ParseMessages(f)
	if len(messages) == 0 {
		return fmt.Errorf("no messages found in session")
	}

	m := NewModel(s, messages)
	p := tea.NewProgram(m, tea.WithAltScreen())

	_, err = p.Run()
	return err
}
