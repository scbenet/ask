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

type SendPromptMsg struct{ Prompt string } // Message from Chat view

// Chat is the main chat view (history + input field).
// It can be plugged directly into a root Bubble Tea program
// or nested inside a parent model.
type Chat struct {
	history viewport.Model
	input   textarea.Model

	sending    bool // while waiting for the model
	historyBuf strings.Builder

	sendKey key.Binding

	// style handles; tweak to taste
	userStyle      lipgloss.Style
	assistantStyle lipgloss.Style
	borderStyle    lipgloss.Style
}

// Add a method to update sending state from App
func (c *Chat) SetSending(sending bool) {
	c.sending = sending
	// Maybe change placeholder or style while sending?
	if sending {
		c.input.Placeholder = "Assistant is thinking..."
	} else {
		c.input.Placeholder = "Write a message…"
	}
}

// New returns an initialized Chat with sane defaults.
func New(width, height int) *Chat {
	// textarea (user input)
	ti := textarea.New()
	ti.Placeholder = "Write a message…"
	ti.Focus()
	ti.CharLimit = 0
	ti.ShowLineNumbers = false

	// remap keys: Shift+Enter (and Ctrl+J as a fallback) inserts newline
	ti.KeyMap.InsertNewline = key.NewBinding(
		key.WithKeys("shift+enter", "ctrl+j"),
		key.WithHelp("⇧+Enter", "newline"),
	)

	// viewport (scrollable chat history)
	vp := viewport.New(width, 0) // leave 5 rows for input & border
	vp.KeyMap = viewport.DefaultKeyMap()
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
		// log.Printf("Chat.Update received: %T", msg)
		switch {
		case key.Matches(m, c.sendKey) && !c.sending: // send prompt
			log.Println("Chat.Update: Send key matched")
			prompt := strings.TrimSpace(c.input.Value())
			// log.Printf("Chat.Update: Input value is: '%s'", c.input.Value())
			// log.Printf("Chat.Update: Trimmed prompt is: '%s'", prompt)
			if prompt == "" {
				// log.Println("Chat.Update: Prompt is empty, breaking")
				break
			}
			// append user message to history
			availableWidth := c.history.Width
			userLabel := c.userStyle.Render("You") // Style the "You" label
			fullMessageLine := fmt.Sprintf("%s: %s", userLabel, prompt)
			rightAlignedStyle := lipgloss.NewStyle().
				Width(availableWidth). // IMPORTANT: Set width for alignment context
				Align(lipgloss.Right)  // Align content to the right
			alignedUserMessage := rightAlignedStyle.Render(fullMessageLine)
			fmt.Fprintf(&c.historyBuf, "%s\n\n", alignedUserMessage)
			// debug: use plain formatting
			// fmt.Fprintf(&c.historyBuf, "You: %s\n\n", prompt)
			// log.Printf("Chat.Update: History buffer length: %d", c.historyBuf.Len())
			c.history.SetContent(c.historyBuf.String())
			c.history.GotoBottom()
			// log.Println("Chat.Update: History SetContent & GotoBottom called")
			c.input.Reset()

			// *** CHANGE HERE: Return SendPromptMsg command ***
			cmd = func() tea.Msg {
				return SendPromptMsg{Prompt: prompt} // Assuming SendPromptMsg is in ui package for now
			}
			cmds = append(cmds, cmd)

		default:
			// Let textarea process keys
			c.input, cmd = c.input.Update(msg)
			cmds = append(cmds, cmd)

			// Handle viewport scrolling AFTER textarea if preferred
			c.history, cmd = c.history.Update(msg)
			cmds = append(cmds, cmd)
		}

	case LLMReplyMsg:
		// log.Printf("Chat.Update: LLMReplyMsg received: '%s'", m.Content)
		// append assistant message
		fmt.Fprintf(&c.historyBuf, "%s: %s\n\n", c.assistantStyle.Render("Assistant"), m.Content)
		// debug: use plain formatting
		// fmt.Fprintf(&c.historyBuf, "Assistant: %s\n\n", m.Content)
		c.history.SetContent(c.historyBuf.String())
		c.history.GotoBottom()
		// log.Println("Chat.Update: Appended assistant message, set sending=false")

	case tea.WindowSizeMsg:
		// log.Printf("Chat.Update: WindowSizeMsg received: %dx%d", m.Width, m.Height)
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

// // requestLLMCmd mocks an asynchronous LLM call; replace with real API.
// func requestLLMCmd(prompt string) tea.Cmd {
// 	return func() tea.Msg {
// 		// Simulate latency
// 		time.Sleep(600 * time.Millisecond)
// 		// Echo back the prompt for now
// 		return LLMReplyMsg{Content: "Echo: " + prompt}
// 	}
// }
