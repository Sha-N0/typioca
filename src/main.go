package main

import (
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	input "github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/timer"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"golang.org/x/term"

	"github.com/muesli/reflow/indent"
	"github.com/muesli/reflow/wordwrap"
)

type myTimer struct {
	timer     timer.Model
	duration  time.Duration
	isRunning bool // Inner is running is being handled weirdly.
	timedout  bool
}

type mistakes struct {
	mistakesAt     map[int]bool
	rawMistakesCnt int // Should never be reduced
}

type StringStyle func(string) termenv.Style

type styles struct {
	correct      StringStyle
	toEnter      StringStyle
	mistakes     StringStyle
	cursor       StringStyle
	runningTimer StringStyle
	stoppedTimer StringStyle
	greener      StringStyle
}

type model struct {
	styles       styles
	timer        myTimer
	wordsToEnter string
	inputBuffer  []rune
	rawInputCnt  int // Should not be reduced
	mistakes     mistakes
	completed    bool
	cursor       int
}

func initialModel() model {
	generator := NewGenerator()
	generator.Count = 200

	testDuration := time.Second * 15

	textToEnter := generator.Generate()

	profile := termenv.ColorProfile()

	fore := termenv.ForegroundColor()

	return model{
		styles: styles{
			correct: func(str string) termenv.Style {
				return termenv.String(str).Foreground(fore)
			},
			toEnter: func(str string) termenv.Style {
				return termenv.String(str).Foreground(fore).Faint()
			},
			mistakes: func(str string) termenv.Style {
				return termenv.String(str).Foreground(profile.Color("1")).Underline()
			},
			cursor: func(str string) termenv.Style {
				return termenv.String(str).Reverse().Bold()
			},
			runningTimer: func(str string) termenv.Style {
				return termenv.String(str).Foreground(profile.Color("2"))
			},
			stoppedTimer: func(str string) termenv.Style {
				return termenv.String(str).Foreground(profile.Color("2")).Faint()
			},
			greener: func(str string) termenv.Style {
				return termenv.String(str).Foreground(profile.Color("6")).Faint()
			},
		},
		timer: myTimer{
			timer:     timer.NewWithInterval(testDuration, time.Second),
			duration:  testDuration,
			isRunning: false,
			timedout:  false,
		},
		wordsToEnter: textToEnter,
		inputBuffer:  make([]rune, 0),
		rawInputCnt:  0,
		mistakes: mistakes{
			mistakesAt:     make(map[int]bool, 0),
			rawMistakesCnt: 0,
		},
		completed: false,
		cursor:    0,
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		input.Blink,
	)
}

func contains(s []string, e string) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func floor(value int) int32 {
	return int32(math.Max(0, float64(value)))
}

func dropLast(value string) string {
	return dropLastN(1, value)
}

func dropLastN(n int, value string) string {
	return value[:len(value)-n]
}

func dropLastRune(runes []rune) []rune {
	le := len(runes)
	if le != 0 {
		return runes[:le-1]
	} else {
		return runes
	}
}

func toKeysSlice(mp map[int]bool) []int {
	acc := []int{}
	for key := range mp {
		acc = append(acc, key)
	}
	return acc
}

func calculateNormalizedWpm(m model) int {
	return calculateWpm(m, len(m.inputBuffer)/5)
}

func calculateRawWpm(m model) int {
	return calculateWpm(m, len(strings.Split(string(m.inputBuffer), " ")))
}

func calculateWpm(m model, wordCnt int) int {
	grossWpm := float64(wordCnt) / m.timer.duration.Minutes()
	netWpm := grossWpm - (float64(len(m.mistakes.mistakesAt)) / m.timer.duration.Minutes())

	return int(netWpm)
}

func calculateCpm(m model) int {
	return int(float64(m.rawInputCnt) / m.timer.duration.Minutes())
}

func calculateAccuracy(m model) float64 {
	mistakesRate := float64(m.mistakes.rawMistakesCnt*100) / float64(m.rawInputCnt)
	accuracy := 100 - mistakesRate
	return accuracy
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var commands []tea.Cmd

	switch msg := msg.(type) {

	case timer.TickMsg:
		timerUpdate, cmdUpdate := m.timer.timer.Update(msg)
		m.timer.timer = timerUpdate
		commands = append(commands, cmdUpdate)
		if m.timer.timer.Timedout() {
			m.timer.timedout = true
			m.completed = true
		}

	// Is it a key press?
	case tea.KeyMsg:

		// Cool, what was the actual key pressed?
		switch msg.String() {

		// These keys should exit the program.
		case "ctrl+c", "esc":
			return m, tea.Quit

		case "enter", "tab	":

		case "backspace":
			m.inputBuffer = dropLastRune(m.inputBuffer)

			//Delete mistakes
			inputLength := len(m.inputBuffer)
			_, ok := m.mistakes.mistakesAt[inputLength]
			if ok {
				delete(m.mistakes.mistakesAt, inputLength)
			}

		default:

			if !m.completed {
				m.inputBuffer = append(m.inputBuffer, msg.Runes...)
				m.rawInputCnt += len(msg.Runes)
			} else {
				break
			}

			if !m.timer.isRunning {
				commands = append(commands, m.timer.timer.Init())
				m.timer.isRunning = true
			}

			//todo this should be moved to non-time gamemode
			if len(m.inputBuffer) == len(m.wordsToEnter) {
				m.completed = true
			}

			currentInput := string(m.inputBuffer)

			if len(currentInput)-1 == len(m.wordsToEnter) {
				m.completed = true
			} else {

				letterToInput := m.wordsToEnter[len(m.inputBuffer)-1 : len(m.inputBuffer)]
				inputLetter := currentInput[floor(len(currentInput)-1):]

				if letterToInput != inputLetter {
					m.mistakes.mistakesAt[len(m.inputBuffer)-1] = true
					m.mistakes.rawMistakesCnt = m.mistakes.rawMistakesCnt + 1
				}

			}

			return m, tea.Batch(commands...)
		}
	}

	// Return the updated model to the Bubble Tea runtime for processing.
	return m, tea.Batch(commands...)
}

func style(str string, style StringStyle) string {
	return style(str).String()
}

func styleAllRunes(runes []rune, style StringStyle) string {
	acc := ""

	for _, char := range runes {
		acc += style(string(char)).String()
	}

	return acc
}

func (m model) View() string {
	s := ""

	if m.timer.timedout {
		rawWpm := calculateRawWpm(m)

		rawWpmShow := "raw: " + style(strconv.Itoa(rawWpm), m.styles.greener)
		cpm := "cpm: " + style(strconv.Itoa(calculateCpm(m)), m.styles.greener)
		wpm := "wpm: " + style(strconv.Itoa(calculateNormalizedWpm(m)), m.styles.runningTimer)
		givenTime := "time: " + style(m.timer.duration.String(), m.styles.greener)
		accuracy := "accuracy: " + style(fmt.Sprintf("%.1f", calculateAccuracy(m)), m.styles.greener)

		content := wpm + "\n\n" + accuracy + " " + rawWpmShow + " " + cpm + "\n" + givenTime

		var style = lipgloss.NewStyle().
			Align(lipgloss.Center).
			PaddingTop(1).
			PaddingBottom(1).
			PaddingLeft(5).
			PaddingRight(5).
			BorderStyle(lipgloss.DoubleBorder()).
			BorderForeground(lipgloss.Color("2"))

		termWidth, termHeight, _ := term.GetSize(0)

		s += lipgloss.Place(termWidth, termHeight, lipgloss.Center, lipgloss.Center, style.Render(content))

	} else if m.completed {
		s += "Out of words lol"
	} else {
		var lineLenLimit int = 40 // todo: calculate out of model. Have max lineLimit and lower taking term size in consideration

		var coloredTimer string
		if m.timer.isRunning {
			coloredTimer = style(m.timer.timer.View(), m.styles.runningTimer)
		} else {
			coloredTimer = style(m.timer.timer.View(), m.styles.stoppedTimer)
		}

		m.cursor = len(m.inputBuffer)

		lines := strings.Split(m.paragraphView(lineLenLimit), "\n")
		cursorLine := findCursorLine(strings.Split(m.paragraphView(lineLenLimit), "\n"), m.cursor)

		linesAroundCursor := strings.Join(getLinesAroundCursor(lines, cursorLine), "\n")

		termWidth, termHeight, _ := term.GetSize(0)

		// Vertical positioning
		for i := 0; i < termHeight/2-3; i++ {
			s += "\n"
		}

		avgLineLen := averageStringLen(lines[:len(lines)-1])
		indentBy := uint(termWidth/2) - (uint(avgLineLen) / 2)

		s += m.indent(coloredTimer, indentBy) + "\n\n" + m.indent(linesAroundCursor, indentBy)
	}

	// Send the UI for rendering
	return s
}

func averageStringLen(strings []string) int {
	var totalLen int = 0
	var cnt int = 0

	for _, str := range strings {
		currentLen := len(dropAnsiCodes(str))
		totalLen += currentLen
		cnt += 1
	}

	return totalLen / cnt
}

func getLinesAroundCursor(lines []string, cursorLine int) []string {
	cursor := cursorLine

	// 3 lines to show
	if cursorLine == 0 {
		cursor += 3
	} else {
		cursor += 2
	}

	low := int(math.Max(0, float64(cursorLine-1)))
	high := int(math.Min(float64(len(lines)), float64(cursor)))

	return lines[low:high]
}

func dropAnsiCodes(colored string) string {
	m := regexp.MustCompile("\x1b\\[[0-9;]*m")

	return m.ReplaceAllString(colored, "")
}

func (m model) indent(block string, indentBy uint) string {
	indentedBlock := indent.String(block, indentBy) // this crashes on small windows

	return indentedBlock
}

func (m model) paragraphView(lineLimit int) string {
	paragraph := m.colorInput()
	paragraph += m.colorCursor()
	paragraph += m.colorWordsToEnter()

	wrapped := wrapStyledParagraph(paragraph, lineLimit)

	return wrapped
}

func (m model) colorInput() string {
	mistakes := toKeysSlice(m.mistakes.mistakesAt)
	sort.Ints(mistakes)

	coloredInput := ""

	if len(mistakes) == 0 {

		coloredInput += styleAllRunes(m.inputBuffer, m.styles.correct)

	} else {

		previousMistake := -1

		for _, mistakeAt := range mistakes {
			sliceUntilMistake := m.inputBuffer[previousMistake+1 : mistakeAt]
			mistakeSlice := m.wordsToEnter[mistakeAt : mistakeAt+1]

			coloredInput += styleAllRunes(sliceUntilMistake, m.styles.correct)
			coloredInput += style(mistakeSlice, m.styles.mistakes)

			previousMistake = mistakeAt
		}

		inputAfterLastMistake := m.inputBuffer[previousMistake+1:]
		coloredInput += styleAllRunes(inputAfterLastMistake, m.styles.correct)
	}

	return coloredInput
}

func (m model) colorCursor() string {
	cursorLetter := m.wordsToEnter[len(m.inputBuffer) : len(m.inputBuffer)+1]

	return style(cursorLetter, m.styles.cursor)
}

func (m model) colorWordsToEnter() string {
	wordsToEnter := m.wordsToEnter[len(m.inputBuffer)+1:] // without cursor

	return style(wordsToEnter, m.styles.toEnter)
}

func wrapStyledParagraph(paragraph string, lineLimit int) string {

	// XXX: Replace spaces, because wordwrap trims them out at the ends
	paragraph = strings.Replace(paragraph, " ", "·", -1)

	f := wordwrap.NewWriter(lineLimit)
	f.Breakpoints = []rune{'·'}
	f.KeepNewlines = false
	f.Write([]byte(paragraph))

	paragraph = strings.Replace(f.String(), "·", " ", -1)

	return paragraph
}

func findCursorLine(lines []string, cursorAt int) int {

	lenAcc := 0
	cursorLine := 0
	for _, line := range lines {
		lineLen := len(dropAnsiCodes(line))

		lenAcc += lineLen

		if cursorAt <= lenAcc-1 {
			return cursorLine
		} else {
			cursorLine += 1
		}
	}

	return cursorLine
}

func main() {
	termenv.ClearScreen()
	termenv.SetWindowTitle("typioca")

	p := tea.NewProgram(initialModel())
	if err := p.Start(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
	println("bye!")
}