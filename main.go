package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// --- TIC-80 DB16 PALETTE ---
// Accurate hex codes from the TIC-80 fantasy console
var (
	ColorVoid   = lipgloss.Color("#140c1c") // 0: Void (Background)
	ColorPurple = lipgloss.Color("#442434") // 1: Purple
	ColorBlue   = lipgloss.Color("#30346d") // 2: Blue
	ColorGrey   = lipgloss.Color("#4e4a4e") // 3: Grey
	ColorBrown  = lipgloss.Color("#854c30") // 4: Brown
	ColorGreen  = lipgloss.Color("#346524") // 5: Green
	ColorRed    = lipgloss.Color("#d04648") // 6: Red
	ColorWhite  = lipgloss.Color("#deeed6") // 15: White-ish
	
	// Brighter / Alternate colors for rainbow effect
	RainbowColors = []lipgloss.Color{
		lipgloss.Color("#d04648"), // Red
		lipgloss.Color("#d27d2c"), // Orange
		lipgloss.Color("#dad45e"), // Yellow
		lipgloss.Color("#6daa2c"), // Green
		lipgloss.Color("#597dce"), // Blue
		lipgloss.Color("#574290"), // Purple
	}

	// --- STYLES ---
	
	// Base App: Dark Void Background
	styleApp = lipgloss.NewStyle().
			Background(ColorVoid).
			Foreground(ColorWhite)

	// Menu Item (Unselected)
	styleNormal = lipgloss.NewStyle().
			Foreground(ColorBlue). // Blueish text like the boot info
			Background(ColorVoid).
			Padding(0, 1)

	// Menu Item (Selected)
	styleSelected = lipgloss.NewStyle().
			Foreground(ColorWhite). // High contrast white
			Background(ColorVoid).  // Keep background dark
			Bold(true).
			Padding(0, 1)

	// Logs
	styleLog = lipgloss.NewStyle().
			Foreground(ColorGrey).
			Background(ColorVoid).
			PaddingLeft(1)

	// Success/Failure
	styleSuccess = lipgloss.NewStyle().
			Foreground(ColorGreen).
			Background(ColorVoid).
			Bold(true)
			
	styleError = lipgloss.NewStyle().
			Foreground(ColorRed).
			Background(ColorVoid).
			Bold(true)
)

// --- CONSTANTS ---
// FIX: Added libX11-devel, libXext-devel, libXcursor-devel, libXi-devel, libXrandr-devel
// These are required for SDL2 video initialization on Linux.
const DEPS_CMD = "dnf -y install @development-tools"
const DEPS_PKGS = "dnf -y install gcc gcc-c++ cmake ruby rubygem-rake libglvnd-devel libglvnd-gles freeglut-devel alsa-lib-devel git libX11-devel libXext-devel libXcursor-devel libXi-devel libXrandr-devel"

// --- STEP DEFINITION ---
type installStep struct {
	desc string
	cmd  string
}

// --- HELPER: RAINBOW TEXT ---
// Renders text with the TIC-80 boot screen rainbow pattern
func renderRainbow(text string) string {
	var s strings.Builder
	for i, char := range text {
		// Cycle through the 6 rainbow colors
		color := RainbowColors[i%len(RainbowColors)]
		style := lipgloss.NewStyle().Foreground(color).Background(ColorVoid)
		s.WriteString(style.Render(string(char)))
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
}

func initialModel() model {
	s := spinner.New()
	s.Spinner = spinner.MiniDot
	// Red spinner to match the cursor color
	s.Style = lipgloss.NewStyle().Foreground(ColorRed).Background(ColorVoid)

	return model{
		choices:  []string{"Install TIC-80 Pro", "Upgrade (Rebuild)", "Uninstall", "Exit"},
		spinner:  s,
		state:    stateMenu,
		logMsg:   "type help for help", // Easter egg text from boot screen
	}
}

func (m model) Init() tea.Cmd {
	return m.spinner.Tick
}

type stepFinishedMsg struct{ err error }
type logUpdateMsg string

// --- UPDATE ---
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		if m.state == stateMenu {
			switch msg.String() {
			case "ctrl+c", "q":
				return m, tea.Quit
			case "up", "k":
				if m.cursor > 0 {
					m.cursor--
				}
			case "down", "j":
				if m.cursor < len(m.choices)-1 {
					m.cursor++
				}
			case "enter":
				if m.cursor == 3 { // Exit
					return m, tea.Quit
				}
				m.state = stateRunning
				m.currentStep = 0
				m.err = nil
				m.steps = getSteps(m.cursor)
				return m, tea.Batch(m.spinner.Tick, runStep(m.steps[0]))
			}
		} else if m.state == stateDone {
			if msg.String() == "enter" || msg.String() == "q" {
				return m, tea.Quit
			}
		}

	case spinner.TickMsg:
		if m.state == stateRunning {
			var cmd tea.Cmd
			m.spinner, cmd = m.spinner.Update(msg)
			return m, cmd
		}

	case stepFinishedMsg:
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
		return m, runStep(m.steps[m.currentStep])
	}

	return m, nil
}

// --- VIEW ---
func (m model) View() string {
	var s strings.Builder

	// 1. HEADER (Rainbow Style)
	// We use the custom function to paint the letters
	title := renderRainbow("TIC-80 PRO MANAGER")
	version := lipgloss.NewStyle().Foreground(ColorGrey).Background(ColorVoid).Render(" version 1.1.2837 (fedora)")
	s.WriteString("\n " + title + "\n " + version + "\n\n")

	// 2. CONTENT
	if m.state == stateMenu {
		for i, choice := range m.choices {
			if m.cursor == i {
				// Selected: Red Block Cursor + White Text
				cursor := lipgloss.NewStyle().Foreground(ColorRed).Background(ColorVoid).Render(">█ ")
				s.WriteString(" " + cursor + styleSelected.Render(choice) + "\n")
			} else {
				// Normal: Empty Space + Blueish Text
				s.WriteString("    " + styleNormal.Render(choice) + "\n")
			}
		}
		s.WriteString("\n " + styleLog.Render("Use arrow keys to select..."))

	} else if m.state == stateRunning {
		currentDesc := m.steps[m.currentStep].desc
		row := fmt.Sprintf(" %s %s", m.spinner.View(), styleNormal.Render(currentDesc))
		s.WriteString(row + "\n\n")
		
		progress := fmt.Sprintf(" Step %d of %d", m.currentStep+1, len(m.steps))
		s.WriteString(styleLog.Render(progress))

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

	// Force Full Screen Void Background
	return styleApp.Width(m.width).Height(m.height).Render(s.String())
}

// --- LOGIC ---
func getSteps(choice int) []installStep {
	switch choice {
	case 0, 1: // Install or Upgrade
		return []installStep{
			{"Installing Group Tools...", DEPS_CMD},
			{"Installing Libraries (X11/GL/Audio)...", DEPS_PKGS},
			{"Cleaning previous builds...", "rm -rf /tmp/tic80-build"},
			{"Creating build directory...", "mkdir -p /tmp/tic80-build"},
			{"Cloning Repository...", "git clone --recursive https://github.com/nesbox/TIC-80.git /tmp/tic80-build/TIC-80"},
			{"Configuring CMake...", "mkdir -p /tmp/tic80-build/TIC-80/build && cd /tmp/tic80-build/TIC-80/build && cmake -DBUILD_PRO=On .."},
			{"Compiling (Using all cores)...", "cd /tmp/tic80-build/TIC-80/build && make -j$(nproc)"},
			{"Installing to /usr/local/bin...", "cd /tmp/tic80-build/TIC-80/build && make install"},
			{"Cleaning up...", "rm -rf /tmp/tic80-build"},
		}
	case 2: // Uninstall
		return []installStep{
			{"Removing Binary...", "rm -f /usr/local/bin/tic80"},
			{"Removing Desktop Entry...", "rm -f /usr/local/share/applications/tic80.desktop"},
			{"Removing Icon...", "rm -f /usr/local/share/icons/hicolor/scalable/apps/tic80.svg"},
		}
	}
	return nil
}

func runStep(step installStep) tea.Cmd {
	return func() tea.Msg {
		cmd := exec.Command("bash", "-c", step.cmd)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return stepFinishedMsg{err: fmt.Errorf("Step '%s' failed:\n%s", step.desc, string(out))}
		}
		return stepFinishedMsg{err: nil}
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
