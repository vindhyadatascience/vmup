package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

type logLineMsg string
type progressDoneMsg struct{ err error }
type authNeededMsg struct{ kind string }
type authRetryMsg struct{}

type progressModel struct {
	spinner    spinner.Model
	viewport   viewport.Model
	lines      []string
	title      string
	done       bool
	err        error
	userScroll   bool // true if user has scrolled away from bottom
	hScroll      int  // horizontal scroll offset
	startTime    time.Time
	totalElapsed time.Duration
}

func newProgressModel(title string) progressModel {
	s := spinner.New()
	s.Spinner = spinner.Dot

	vp := viewport.New(80, 20)
	vp.SetContent("")

	return progressModel{
		spinner:   s,
		viewport:  vp,
		title:     title,
		startTime: time.Now(),
	}
}

func (m progressModel) Init() tea.Cmd {
	return m.spinner.Tick
}

func (m *progressModel) renderContent() {
	w := m.viewport.Width
	if w < 10 {
		w = 80
	}

	var rendered []string
	for _, line := range m.lines {
		if m.hScroll > 0 && len(line) > m.hScroll {
			line = line[m.hScroll:]
		} else if m.hScroll > 0 {
			line = ""
		}
		rendered = append(rendered, line)
	}

	m.viewport.SetContent(strings.Join(rendered, "\n"))
}

func (m progressModel) Update(msg tea.Msg) (progressModel, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.viewport.Width = msg.Width
		h := msg.Height - 6
		if h < 5 {
			h = 5
		}
		m.viewport.Height = h
		m.renderContent()

	case logLineMsg:
		m.lines = append(m.lines, string(msg))
		m.renderContent()
		if !m.userScroll {
			m.viewport.GotoBottom()
		}

	case progressDoneMsg:
		m.done = true
		m.totalElapsed = time.Since(m.startTime)
		m.err = msg.err
		if msg.err != nil {
			m.lines = append(m.lines, errorStyle.Width(m.viewport.Width).Render(fmt.Sprintf("\nError: %v", msg.err)))
		} else {
			m.lines = append(m.lines, successStyle.Render("\nDone!"))
		}
		m.renderContent()
		m.viewport.GotoBottom()
		m.userScroll = false

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			m.viewport.LineUp(1)
			m.userScroll = m.viewport.YOffset < m.viewport.TotalLineCount()-m.viewport.Height
		case "down", "j":
			m.viewport.LineDown(1)
			// If we're back at the bottom, re-enable auto-scroll
			if m.viewport.YOffset >= m.viewport.TotalLineCount()-m.viewport.Height {
				m.userScroll = false
			}
		case "left", "h":
			if m.hScroll > 0 {
				m.hScroll -= 10
				if m.hScroll < 0 {
					m.hScroll = 0
				}
				m.renderContent()
			}
		case "right", "l":
			m.hScroll += 10
			m.renderContent()
		case "home", "g":
			m.viewport.GotoTop()
			m.userScroll = true
		case "end", "G":
			m.viewport.GotoBottom()
			m.userScroll = false
		default:
			// Pass other keys (pgup, pgdn, etc.) to viewport
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			cmds = append(cmds, cmd)
			if m.viewport.YOffset >= m.viewport.TotalLineCount()-m.viewport.Height {
				m.userScroll = false
			} else {
				m.userScroll = true
			}
		}
	}

	return m, tea.Batch(cmds...)
}

func (m progressModel) View() string {
	var b strings.Builder

	if m.done {
		ts := formatElapsed(m.totalElapsed)
		if m.err != nil {
			b.WriteString(errorStyle.Render("✗ "+m.title) + " " + dimStyle.Render("("+ts+")"))
		} else {
			b.WriteString(successStyle.Render("✓ "+m.title) + " " + dimStyle.Render("("+ts+")"))
		}
	} else {
		ts := formatElapsed(time.Since(m.startTime))
		inlineTitle := titleStyle.MarginBottom(0)
		b.WriteString(m.spinner.View() + " " + inlineTitle.Render(m.title) + " " + dimStyle.Render("("+ts+")"))
	}

	b.WriteString("\n\n")
	b.WriteString(m.viewport.View())
	b.WriteString("\n")

	b.WriteString("\n")
	if m.done {
		b.WriteString(dimStyle.Render("↑/↓ scroll • ←/→ pan • enter continue • ctrl+c back"))
	} else {
		b.WriteString(dimStyle.Render("↑/↓ scroll • ←/→ pan • esc/ctrl+c back"))
	}

	return b.String()
}

// logWriter sends each line written to it as a tea.Msg
type logWriter struct {
	program *tea.Program
	buf     string
}

func newLogWriter(p *tea.Program) *logWriter {
	return &logWriter{program: p}
}

func (w *logWriter) Flush() {
	if w.buf != "" {
		w.program.Send(logLineMsg(w.buf))
		w.buf = ""
	}
}

func (w *logWriter) Write(p []byte) (int, error) {
	w.buf += string(p)
	for {
		idx := strings.Index(w.buf, "\n")
		if idx == -1 {
			break
		}
		line := w.buf[:idx]
		w.buf = w.buf[idx+1:]
		w.program.Send(logLineMsg(line))
	}
	return len(p), nil
}
