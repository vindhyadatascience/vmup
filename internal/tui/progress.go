package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

type logLineMsg string
type progressDoneMsg struct{ err error }

type progressModel struct {
	spinner  spinner.Model
	viewport viewport.Model
	lines    []string
	title    string
	done     bool
	err      error
}

func newProgressModel(title string) progressModel {
	s := spinner.New()
	s.Spinner = spinner.Dot

	vp := viewport.New(80, 20)
	vp.SetContent("")

	return progressModel{
		spinner:  s,
		viewport: vp,
		title:    title,
	}
}

func (m progressModel) Init() tea.Cmd {
	return m.spinner.Tick
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

	case logLineMsg:
		m.lines = append(m.lines, string(msg))
		m.viewport.SetContent(strings.Join(m.lines, "\n"))
		m.viewport.GotoBottom()

	case progressDoneMsg:
		m.done = true
		m.err = msg.err
		if msg.err != nil {
			m.lines = append(m.lines, errorStyle.Render(fmt.Sprintf("\nError: %v", msg.err)))
		} else {
			m.lines = append(m.lines, successStyle.Render("\nDone!"))
		}
		m.viewport.SetContent(strings.Join(m.lines, "\n"))
		m.viewport.GotoBottom()

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)

	case tea.KeyMsg:
		// Allow viewport scrolling
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m progressModel) View() string {
	var b strings.Builder

	if m.done {
		if m.err != nil {
			b.WriteString(errorStyle.Render("✗ " + m.title))
		} else {
			b.WriteString(successStyle.Render("✓ " + m.title))
		}
	} else {
		b.WriteString(m.spinner.View() + " " + titleStyle.Render(m.title))
	}

	b.WriteString("\n\n")
	b.WriteString(m.viewport.View())
	b.WriteString("\n")

	b.WriteString("\n")
	if m.done {
		b.WriteString(dimStyle.Render("Press enter to continue"))
	} else {
		b.WriteString(dimStyle.Render("esc back"))
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
