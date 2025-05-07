package ui

import (
	"fmt"
	"log"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// LLMReplyMsg is emitted when a response arrives from the LLM.
type LLMReplyMsg struct {
	Content string
}

// Message to send to API
type SendPromptMsg struct{ Prompt string }

// Chat is the main chat view (history + input field).
type Chat struct {
	history viewport.Model
	input   textarea.Model

	sending    bool // true while waiting for the model response
	historyBuf strings.Builder

	sendKey key.Binding

	// style handles
	userStyle      lipgloss.Style
	assistantStyle lipgloss.Style
	borderStyle    lipgloss.Style
}

func (c *Chat) SetSending(sending bool) {
	c.sending = sending
	if sending {
		c.input.Placeholder = "Assistant is thinking..."
	} else {
		c.input.Placeholder = "Write a message…"
	}
}

// returns an initialized Chat with sane defaults.
func New(width, height int) *Chat {
	// textarea (user input)
	ti := textarea.New()
	ti.Placeholder = "Write a message…"
	ti.Focus()
	ti.CharLimit = 0
	ti.ShowLineNumbers = false

	// remap keys: shift+Enter (and Ctrl+J as a fallback) inserts newline
	// TODO shift+enter doesn't work yet, need to update to new bubbletea version to get kitty protocol support
	ti.KeyMap.InsertNewline = key.NewBinding(
		key.WithKeys("shift+enter", "ctrl+j"),
		key.WithHelp("⇧+Enter", "newline"),
	)

	// viewport (scrollable chat history)
	vp := viewport.New(width, 0)
	vp.KeyMap = CustomKeyMap()
	vp.SetContent("")

	c := &Chat{
		history:        vp,
		input:          ti,
		sendKey:        key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "send")),
		userStyle:      lipgloss.NewStyle().Bold(true),
		assistantStyle: lipgloss.NewStyle().Foreground(lipgloss.Color("#7D56F4")),
		borderStyle:    lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#777")),
	}

	return c
}

// Init implements tea.Model.
func (c *Chat) Init() tea.Cmd {
	return tea.Sequence(textarea.Blink, c.input.Focus())
}

// Update implements tea.Model.
func (c *Chat) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	var cmds []tea.Cmd

	switch m := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(m, c.sendKey) && !c.sending: // send prompt
			log.Println("Chat.Update: Send key matched")
			prompt := strings.TrimSpace(c.input.Value())

			if prompt == "" {
				log.Println("Chat.Update: Prompt is empty, breaking")
				break
			}

			// append user message to history
			availableWidth := c.history.Width
			userLabel := c.userStyle.Render("You")
			fullMessageLine := fmt.Sprintf("%s: %s", userLabel, prompt)
			rightAlignedStyle := lipgloss.NewStyle().
				Width(availableWidth). // IMPORTANT: Set width for alignment context
				Align(lipgloss.Right)  // align content to the right
			alignedUserMessage := rightAlignedStyle.Render(fullMessageLine)
			fmt.Fprintf(&c.historyBuf, "%s\n\n", alignedUserMessage)

			c.history.SetContent(c.historyBuf.String())
			c.history.GotoBottom()
			c.input.Reset()

			cmd = func() tea.Msg {
				return SendPromptMsg{Prompt: prompt}
			}
			cmds = append(cmds, cmd)

		default:
			// Let textarea process keys
			c.input, cmd = c.input.Update(msg)
			cmds = append(cmds, cmd)

			c.history, cmd = c.history.Update(msg)
			cmds = append(cmds, cmd)
		}

	case LLMReplyMsg:
		log.Printf("Chat.Update: LLMReplyMsg received: '%s'", m.Content)
		// append assistant message
		fmt.Fprintf(&c.historyBuf, "%s: %s\n\n", c.assistantStyle.Render("Assistant"), m.Content)

		c.history.SetContent(c.historyBuf.String())
		c.history.GotoBottom()
		log.Println("Chat.Update: Appended assistant message, set sending=false")

	case tea.WindowSizeMsg:
		inputHeight := lipgloss.Height(
			c.borderStyle.Render(c.input.View()),
		)
		c.history.Width = m.Width
		c.history.Height = m.Height - inputHeight
		c.input.SetWidth(m.Width - 2)
	}

	return c, tea.Batch(cmds...)
}

// View implements tea.Model.
func (c *Chat) View() string {
	inputView := c.borderStyle.Render(c.input.View())
	return lipgloss.JoinVertical(lipgloss.Left, c.history.View(), inputView)
}
