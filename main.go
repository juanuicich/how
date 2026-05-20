package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
)

// version is overridden at build time via -ldflags "-X main.version=...".
var version = "dev"

const basePrompt = `You are a shell command generator. The user describes a task; respond with ONLY the exact shell command, on a single line where possible. Chain steps with && when needed. No prose, no markdown fences, no backticks, no commentary. Just the command. Assume macOS with standard developer tools (git, gh, brew, rg, jq, etc.).`

const explainTemplate = "Explain this shell command in well-formatted Markdown. Start with a one-line summary, then break down each part: subcommands, flags, and arguments. Be precise but concise.\n\nCommand:\n```\n%s\n```"

type turn struct{ role, content string }

type state int

const (
	stateLoading state = iota
	stateCommand
	stateRefine
	stateExplainLoading
	stateExplain
)

type claudeMsg struct {
	content    string
	err        error
	forExplain bool
}

type model struct {
	state         state
	question      string
	command       string
	history       []turn
	spinner       spinner.Model
	input         textinput.Model
	viewport      viewport.Model
	width         int
	height        int
	err           error
	exitAndRun    bool
	explainOutput string
}

var (
	accent = lipgloss.Color("#A78BFA")
	good   = lipgloss.Color("#86EFAC")
	muted  = lipgloss.Color("#6B7280")
	danger = lipgloss.Color("#F87171")
	dim    = lipgloss.Color("#9CA3AF")

	pageStyle  = lipgloss.NewStyle().Padding(1, 2)
	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#1F1B2E")).
			Background(accent).
			Bold(true).
			Padding(0, 1)
	questionStyle = lipgloss.NewStyle().Foreground(dim).Italic(true)
	commandBox    = lipgloss.NewStyle().
			Foreground(good).
			Bold(true).
			Padding(0, 2).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(accent)
	refineBox = lipgloss.NewStyle().
			Padding(0, 1).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(accent)
	hintLabel = lipgloss.NewStyle().Foreground(muted)
	keyStyle  = lipgloss.NewStyle().Foreground(accent).Bold(true)
	errStyle  = lipgloss.NewStyle().Foreground(danger)
	spinStyle = lipgloss.NewStyle().Foreground(accent)
)

func initialModel(question string) model {
	sp := spinner.New()
	sp.Spinner = spinner.Points
	sp.Style = spinStyle

	ti := textinput.New()
	ti.Placeholder = "add more context..."
	ti.Prompt = "❯ "
	ti.PromptStyle = lipgloss.NewStyle().Foreground(accent).Bold(true)
	ti.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#F3F4F6"))
	ti.CharLimit = 500
	ti.Width = 60

	vp := viewport.New(80, 20)

	return model{
		state:    stateLoading,
		question: question,
		spinner:  sp,
		input:    ti,
		viewport: vp,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, callClaude(nil, m.question, false))
}

func callClaude(history []turn, userMsg string, forExplain bool) tea.Cmd {
	return func() tea.Msg {
		var prompt string
		if forExplain {
			prompt = fmt.Sprintf(explainTemplate, userMsg)
		} else {
			var sb strings.Builder
			sb.WriteString(basePrompt)
			if len(history) > 0 {
				sb.WriteString("\n\nConversation so far:")
				for _, t := range history {
					sb.WriteString("\n")
					sb.WriteString(t.role)
					sb.WriteString(": ")
					sb.WriteString(t.content)
				}
			}
			sb.WriteString("\n\nuser: ")
			sb.WriteString(userMsg)
			sb.WriteString("\nassistant:")
			prompt = sb.String()
		}
		out, err := exec.Command("claude", "-p", "--model", "sonnet", prompt).Output()
		if err != nil {
			return claudeMsg{err: err, forExplain: forExplain}
		}
		return claudeMsg{content: strings.TrimSpace(string(out)), forExplain: forExplain}
	}
}

func cleanCommand(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		lines := strings.Split(s, "\n")
		if len(lines) >= 2 {
			lines = lines[1:]
			if len(lines) > 0 && strings.HasPrefix(lines[len(lines)-1], "```") {
				lines = lines[:len(lines)-1]
			}
			s = strings.Join(lines, "\n")
		}
	}
	s = strings.Trim(s, "`")
	return strings.TrimSpace(s)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = max(40, msg.Width-6)
		m.viewport.Height = max(8, msg.Height-14)
		m.input.Width = max(20, msg.Width-12)
		return m, nil

	case claudeMsg:
		if msg.err != nil {
			m.err = msg.err
			m.state = stateCommand
			return m, nil
		}
		if msg.forExplain {
			rendered, err := glamour.Render(msg.content, "dark")
			if err != nil {
				rendered = msg.content
			}
			m.viewport.SetContent(rendered)
			m.explainOutput = rendered
			m.state = stateExplain
			return m, nil
		}
		cmd := cleanCommand(msg.content)
		m.command = cmd
		m.history = append(m.history, turn{"user", m.question})
		m.history = append(m.history, turn{"assistant", cmd})
		m.state = stateCommand
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case tea.KeyMsg:
		switch m.state {
		case stateCommand:
			switch msg.String() {
			case "q", "ctrl+c", "esc":
				return m, tea.Quit
			case "enter":
				m.exitAndRun = true
				return m, tea.Quit
			case "tab":
				m.state = stateRefine
				m.input.SetValue("")
				m.input.Focus()
				return m, textinput.Blink
			case "e":
				m.state = stateExplainLoading
				return m, tea.Batch(m.spinner.Tick, callClaude(nil, m.command, true))
			}
		case stateRefine:
			switch msg.String() {
			case "ctrl+c":
				return m, tea.Quit
			case "esc":
				m.state = stateCommand
				m.input.Blur()
				return m, nil
			case "enter":
				q := strings.TrimSpace(m.input.Value())
				if q == "" {
					return m, nil
				}
				m.question = q
				m.state = stateLoading
				m.input.Blur()
				return m, tea.Batch(m.spinner.Tick, callClaude(m.history, q, false))
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		case stateExplain:
			switch msg.String() {
			case "q", "esc", "ctrl+c":
				return m, tea.Quit
			case "enter":
				m.exitAndRun = true
				return m, tea.Quit
			}
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		case stateExplainLoading:
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

func (m model) View() string {
	// pageStyle has Padding(1,2) → 4 cols used.
	// commandBox has Border(2) + Padding(0,2)(4) → 6 cols used.
	contentW := max(20, m.width-4)
	boxW := max(10, contentW-6)
	// title "how" with Padding(0,1) renders 5 wide, plus 2-space separator = 7.
	questionW := max(10, contentW-7)

	box := commandBox.Width(boxW)
	question := questionStyle.Width(questionW).Render(m.question)

	header := lipgloss.JoinHorizontal(
		lipgloss.Top,
		titleStyle.Render("how"),
		"  ",
		question,
	)
	sections := []string{header, ""}

	switch m.state {
	case stateLoading:
		sections = append(sections, m.spinner.View()+" "+questionStyle.Render("thinking..."))

	case stateCommand:
		if m.err != nil {
			sections = append(sections, errStyle.Render("error: "+m.err.Error()))
		} else {
			sections = append(sections, box.Render(m.command))
		}
		sections = append(sections, "", hints(
			[2]string{"enter", "run"},
			[2]string{"tab", "refine"},
			[2]string{"e", "explain"},
			[2]string{"q", "quit"},
		))

	case stateRefine:
		sections = append(sections,
			box.Render(m.command),
			"",
			refineBox.Render(m.input.View()),
			"",
			hints(
				[2]string{"enter", "send"},
				[2]string{"esc", "back"},
			),
		)

	case stateExplainLoading:
		sections = append(sections,
			box.Render(m.command),
			"",
			m.spinner.View()+" "+questionStyle.Render("explaining..."),
		)

	case stateExplain:
		sections = append(sections,
			m.viewport.View(),
			"",
			box.Render(m.command),
			"",
			hints(
				[2]string{"enter", "run"},
				[2]string{"↑↓", "scroll"},
				[2]string{"q/esc", "quit"},
			),
		)
	}

	return pageStyle.Render(lipgloss.JoinVertical(lipgloss.Left, sections...))
}

func hints(pairs ...[2]string) string {
	parts := make([]string, 0, len(pairs))
	for _, p := range pairs {
		parts = append(parts, keyStyle.Render(p[0])+" "+hintLabel.Render(p[1]))
	}
	return strings.Join(parts, hintLabel.Render("  ·  "))
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "how — natural language to shell commands")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "usage: how <what you want to do>")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "examples:")
	fmt.Fprintln(w, "  how list files modified in the last hour")
	fmt.Fprintln(w, "  how find processes using port 3000")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "flags:")
	fmt.Fprintln(w, "  -h, --help       show this help")
	fmt.Fprintln(w, "  -v, --version    print version")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "requires the `claude` CLI (https://docs.claude.com/claude-code) on PATH and logged in.")
}

func main() {
	if len(os.Args) < 2 {
		printUsage(os.Stderr)
		os.Exit(1)
	}
	switch os.Args[1] {
	case "-h", "--help":
		printUsage(os.Stdout)
		return
	case "-v", "--version":
		fmt.Printf("how %s\n", version)
		return
	}
	if _, err := exec.LookPath("claude"); err != nil {
		fmt.Fprintln(os.Stderr, "error: `claude` CLI not found on PATH.")
		fmt.Fprintln(os.Stderr, "Install it from https://docs.claude.com/claude-code and run `claude login` first.")
		os.Exit(1)
	}
	question := strings.Join(os.Args[1:], " ")

	p := tea.NewProgram(initialModel(question), tea.WithAltScreen())
	finalModel, err := p.Run()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	m, ok := finalModel.(model)
	if !ok {
		os.Exit(1)
	}
	if m.explainOutput != "" {
		fmt.Print(m.explainOutput)
	}
	if m.explainOutput != "" && !m.exitAndRun && m.command != "" {
		contentW := max(20, m.width-4)
		boxW := max(10, contentW-6)
		fmt.Println(commandBox.Width(boxW).Render(m.command))
	}
	if m.exitAndRun && m.command != "" {
		fmt.Printf("\033[2m$\033[0m %s\n", m.command)
		c := exec.Command("sh", "-c", m.command)
		c.Stdin = os.Stdin
		c.Stdout = os.Stdout
		c.Stderr = os.Stderr
		if err := c.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				os.Exit(exitErr.ExitCode())
			}
			os.Exit(1)
		}
	}
}
