package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- TIC-80 DB16 PALETTE ---
var (
	ColorVoid   = lipgloss.Color("#140c1c")
	ColorPurple = lipgloss.Color("#442434")
	ColorBlue   = lipgloss.Color("#30346d")
	ColorGrey   = lipgloss.Color("#4e4a4e")
	ColorBrown  = lipgloss.Color("#854c30")
	ColorGreen  = lipgloss.Color("#346524")
	ColorRed    = lipgloss.Color("#d04648")
	ColorWhite  = lipgloss.Color("#deeed6")
	
	RainbowColors = []lipgloss.Color{
		lipgloss.Color("#d04648"), lipgloss.Color("#d27d2c"), lipgloss.Color("#dad45e"),
		lipgloss.Color("#6daa2c"), lipgloss.Color("#597dce"), lipgloss.Color("#574290"),
	}

	// --- STYLES ---
	styleApp = lipgloss.NewStyle().Background(ColorVoid).Foreground(ColorWhite)

	styleNormal = lipgloss.NewStyle().Foreground(ColorBlue).Background(ColorVoid).Padding(0, 1)
	styleSelected = lipgloss.NewStyle().Foreground(ColorWhite).Background(ColorVoid).Bold(true).Padding(0, 1)

	styleLog = lipgloss.NewStyle().Foreground(ColorGrey).Background(ColorVoid).PaddingLeft(1)
	styleSuccess = lipgloss.NewStyle().Foreground(ColorGreen).Background(ColorVoid).Bold(true)
	styleError = lipgloss.NewStyle().Foreground(ColorRed).Background(ColorVoid).Bold(true)

	// TERMINAL BOX
	styleTermBox = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorGrey).
		Background(ColorVoid).
		Padding(0, 1)

	styleTermText = lipgloss.NewStyle().Foreground(lipgloss.Color("#666666"))
)

const DEPS_CMD = "dnf -y install @development-tools"
const DEPS_PKGS = "dnf -y install gcc gcc-c++ cmake ruby rubygem-rake libglvnd-devel libglvnd-gles freeglut-devel alsa-lib-devel git libX11-devel libXext-devel libXcursor-devel libXi-devel libXrandr-devel mesa-libGLU-devel curl"

type installStep struct {
	desc string
	cmd  string
}

func renderRainbow(text string) string {
	var s strings.Builder
	for i, char := range text {
		color := RainbowColors[i%len(RainbowColors)]
		s.WriteString(lipgloss.NewStyle().Foreground(color).Background(ColorVoid).Render(string(char)))
	}
	return s.String()
}

// --- MODEL ---
type state int

const (
	stateMenu state = iota
	stateRunning
	stateDone
)

type model struct {
	width       int
	height      int
	cursor      int
	choices     []string
	state       state
	spinner     spinner.Model
	
	steps       []installStep
	currentStep int
	logMsg      string
	err         error

	// Terminal
	viewport    viewport.Model
	showTerm    bool
	termContent string
}

func initialModel() model {
	s := spinner.New()
	s.Spinner = spinner.MiniDot
	s.Style = lipgloss.NewStyle().Foreground(ColorRed).Background(ColorVoid)

	vp := viewport.New(0, 0)
	vp.Style = styleTermBox

	return model{
		choices:  []string{"Install TIC-80 Pro", "Upgrade (Rebuild)", "Uninstall", "Exit"},
		spinner:  s,
		state:    stateMenu,
		logMsg:   "type help for help",
		viewport: vp,
		showTerm: false,
	}
}

func (m model) Init() tea.Cmd {
	return m.spinner.Tick
}

type stepLogAndFinishMsg struct {
	output string
	err    error
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.viewport.Width = msg.Width - 4
		m.viewport.Height = msg.Height / 3

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "tab", " ": // Spacebar or Tab toggles terminal
			m.showTerm = !m.showTerm
			return m, nil
		case "up", "k":
			if m.state == stateMenu && m.cursor > 0 { m.cursor-- }
		case "down", "j":
			if m.state == stateMenu && m.cursor < len(m.choices)-1 { m.cursor++ }
		case "enter":
			if m.state == stateMenu {
				if m.cursor == 3 { return m, tea.Quit }
				m.state = stateRunning
				m.currentStep = 0
				m.err = nil
				m.termContent = ""
				m.steps = getSteps(m.cursor)
				return m, tea.Batch(m.spinner.Tick, runStepStreamed(m.steps[0]))
			} else if m.state == stateDone {
				return m, tea.Quit
			}
		}

	case spinner.TickMsg:
		if m.state == stateRunning {
			m.spinner, cmd = m.spinner.Update(msg)
			cmds = append(cmds, cmd)
		}

	case stepLogAndFinishMsg:
		// Add output to viewport
		cmdName := m.steps[m.currentStep].desc
		m.termContent += fmt.Sprintf(">>> %s\n%s\n", cmdName, msg.output)
		m.viewport.SetContent(styleTermText.Render(m.termContent))
		m.viewport.GotoBottom()

		if msg.err != nil {
			m.state = stateDone
			m.err = msg.err
			return m, nil
		}
		m.currentStep++
		if m.currentStep >= len(m.steps) {
			m.state = stateDone
			m.logMsg = "Process Completed."
			return m, nil
		}
		return m, runStepStreamed(m.steps[m.currentStep])
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m model) View() string {
	var s strings.Builder

	title := renderRainbow("TIC-80 PRO MANAGER")
	version := lipgloss.NewStyle().Foreground(ColorGrey).Background(ColorVoid).Render(" version 1.2.3019 (fedora)")
	s.WriteString("\n " + title + "\n " + version + "\n\n")

	if m.state == stateMenu {
		for i, choice := range m.choices {
			if m.cursor == i {
				cursor := lipgloss.NewStyle().Foreground(ColorRed).Background(ColorVoid).Render(">█ ")
				s.WriteString(" " + cursor + styleSelected.Render(choice) + "\n")
			} else {
				s.WriteString("    " + styleNormal.Render(choice) + "\n")
			}
		}
		s.WriteString("\n " + styleLog.Render("Use arrow keys to select..."))
		s.WriteString("\n " + styleLog.Render("Press SPACE to toggle Logs"))

	} else if m.state == stateRunning {
		currentDesc := m.steps[m.currentStep].desc
		row := fmt.Sprintf(" %s %s", m.spinner.View(), styleNormal.Render(currentDesc))
		s.WriteString(row + "\n\n")
		
		progress := fmt.Sprintf(" Step %d of %d", m.currentStep+1, len(m.steps))
		s.WriteString(styleLog.Render(progress))
		s.WriteString("\n " + styleLog.Render("Press SPACE to toggle Logs"))

	} else if m.state == stateDone {
		if m.err != nil {
			s.WriteString(" " + styleError.Render("FAILED"))
			s.WriteString("\n " + styleLog.Render(m.err.Error()))
		} else {
			s.WriteString(" " + styleSuccess.Render("SUCCESS"))
			s.WriteString("\n " + styleLog.Render(m.logMsg))
		}
		s.WriteString("\n\n " + styleLog.Render("Press Enter to Exit."))
	}

	if m.showTerm {
		s.WriteString("\n\n")
		s.WriteString(m.viewport.View())
	}

	return styleApp.Width(m.width).Height(m.height).Render(s.String())
}

func getSteps(choice int) []installStep {
	// We use /var/tmp to avoid RAM disk limits
	buildDir := "/var/tmp/tic80-build"
	
	// FIX: Explicitly force the 'TIC80_PRO' definition into C/C++ flags.
	// This ensures the compiler sees it even if CMake logic misses it.
	cmakeFlags := "-DCMAKE_C_FLAGS=\"-DTIC80_PRO\" -DCMAKE_CXX_FLAGS=\"-DTIC80_PRO\" -DBUILD_PRO=On -DBUILD_WITH_ALL=On -DBUILD_SDL=On -DBUILD_SDLGPU=On -DBUILD_STATIC=On"

	switch choice {
	case 0, 1: // Install
		return []installStep{
			{"Installing Group Tools...", DEPS_CMD},
			{"Installing Deps (GLU/Curl/X11)...", DEPS_PKGS},
			{"Cleaning previous builds...", fmt.Sprintf("rm -rf %s", buildDir)},
			{"Creating build directory...", fmt.Sprintf("mkdir -p %s", buildDir)},
			{"Cloning Repository...", fmt.Sprintf("git clone --recursive https://github.com/nesbox/TIC-80.git %s/TIC-80", buildDir)},
			{"Patching SDL2...", fmt.Sprintf("cd %s/TIC-80/vendor/sdl2 && git fetch --tags && git checkout release-2.32.8", buildDir)},
			{"Configuring CMake (Forcing Pro)...", fmt.Sprintf("mkdir -p %s/TIC-80/build && cd %s/TIC-80/build && cmake %s ..", buildDir, buildDir, cmakeFlags)},
			{"Compiling...", fmt.Sprintf("cd %s/TIC-80/build && make -j$(nproc)", buildDir)},
			{"Installing...", fmt.Sprintf("cd %s/TIC-80/build && make install", buildDir)},
			{"Cleaning up...", fmt.Sprintf("rm -rf %s", buildDir)},
		}
	case 2: // Uninstall
		return []installStep{
			{"Removing Binary...", "rm -f /usr/local/bin/tic80"},
			{"Removing Desktop...", "rm -f /usr/local/share/applications/tic80.desktop"},
			{"Removing Icon...", "rm -f /usr/local/share/icons/hicolor/scalable/apps/tic80.svg"},
		}
	}
	return nil
}

func runStepStreamed(step installStep) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("bash", "-c", step.cmd)
		output, err := cmd.CombinedOutput()
		return stepLogAndFinishMsg{output: string(output), err: err}
	}
}

func main() {
	if os.Geteuid() != 0 {
		fmt.Println("Error: This program must be run as root (sudo).")
		os.Exit(1)
	}
	p := tea.NewProgram(initialModel(), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}
}
