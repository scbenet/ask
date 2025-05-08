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
	"github.com/scbenet/ask/internal/llm"
)

// LLMReplyMsg is emitted when a response arrives from the LLM.
type LLMReplyMsg struct{ Content string }

type StreamEndMsg struct{ FullResponse string }

type StreamErrorMsg struct{ Err string }

// Message to send to API
type SendPromptMsg struct{ Prompt string }

// Chat is the main chat view (history + input field).
type Chat struct {
	history viewport.Model
	input   textarea.Model

	sending           bool // true while waiting for the model response to finish
	historyBuf        strings.Builder
	assistantResponse strings.Builder // builds current assistant message during streaming

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
		// prepare for new assistant response
		fmt.Fprintf(&c.historyBuf, "%s: ", c.assistantStyle.Render("Assistant"))
		c.history.SetContent(c.historyBuf.String())
		c.history.GotoBottom()
		c.assistantResponse.Reset() // Ensure the buffer for the current response is clean
	} else {
		c.input.Placeholder = "Write a message…"
		// if SetSending(false) is called and no content was ever streamed for the current
		// assistant message (e.g., immediate error before any chunks), clean up the "Assistant: " prefix.
		currentContent := c.historyBuf.String()
		prefix := c.assistantStyle.Render("Assistant") + ": "
		if strings.HasSuffix(currentContent, prefix) && c.assistantResponse.Len() == 0 {
			c.historyBuf.Reset()
			c.historyBuf.WriteString(strings.TrimSuffix(currentContent, prefix))
			c.history.SetContent(c.historyBuf.String())
			c.history.GotoBottom()
		}
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
	return tea.Batch(textarea.Blink, c.input.Focus())
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
			userLabel := c.userStyle.Render("You")
			fullMessageLine := fmt.Sprintf("%s: %s", userLabel, prompt)
			fmt.Fprintf(&c.historyBuf, "%s\n\n", fullMessageLine)

			c.history.SetContent(c.historyBuf.String())
			c.history.GotoBottom()
			c.input.Reset()

			cmd = func() tea.Msg { return SendPromptMsg{Prompt: prompt} }
			cmds = append(cmds, cmd)

		default:
			c.input, cmd = c.input.Update(msg)
			cmds = append(cmds, cmd)
			c.history, cmd = c.history.Update(msg)
			cmds = append(cmds, cmd)
		}

	case llm.StreamChunkMsg:
		log.Printf("Chat.Update: StreamChunkMsg received: '%s'", m.Content)
		c.assistantResponse.WriteString(m.Content) // add to temporary buffer for current response
		fmt.Fprint(&c.historyBuf, m.Content)
		c.history.SetContent(c.historyBuf.String())
		c.history.GotoBottom()

	case StreamEndMsg:
		log.Printf("Chat.Update: StreamEndMsg received. Full response was: %s", m.FullResponse)
		if c.assistantResponse.Len() > 0 { // Only add newlines if content was received
			fmt.Fprintf(&c.historyBuf, "\n\n")
		}
		c.assistantResponse.Reset()
		c.history.SetContent(c.historyBuf.String())
		c.history.GotoBottom()

	case StreamErrorMsg:
		log.Printf("Chat.Update: StreamErrorMsg received: %s", m.Err)
		fmt.Fprintf(&c.historyBuf, "%s\n\n", m.Err)
		c.history.SetContent(c.historyBuf.String())
		c.history.GotoBottom()

	// primarily for non-streaming or error messages
	case LLMReplyMsg:
		log.Printf("Chat.Update: LLMReplyMsg received: '%s'", m.Content)
		// for error messages receieved after 'Assistant' name printed
		if strings.HasSuffix(c.historyBuf.String(), c.assistantStyle.Render("Assistant")+": ") {
			fmt.Fprintf(&c.historyBuf, "%s\n\n", m.Content)
		} else {
			// for regular non-streaming messages
			fmt.Fprintf(&c.historyBuf, "%s: %s\n\n", c.assistantStyle.Render("Assistant"), m.Content)
		}

		c.history.SetContent(c.historyBuf.String())
		c.history.GotoBottom()
		c.assistantResponse.Reset()
		log.Println("Chat.Update: Appended LLMReplyMsg")

	case tea.WindowSizeMsg:
		inputHeight := lipgloss.Height(c.borderStyle.Render(c.input.View()))
		c.history.Width = m.Width
		c.history.Height = m.Height - inputHeight
		c.input.SetWidth(m.Width - 2) // -2 for border
	}

	return c, tea.Batch(cmds...)
}

// View implements tea.Model.
func (c *Chat) View() string {
	inputView := c.borderStyle.Render(c.input.View())
	return lipgloss.JoinVertical(lipgloss.Left, c.history.View(), inputView)
}

func (c *Chat) ClearHistory() {
	c.historyBuf.Reset()
	c.assistantResponse.Reset()
	c.history.SetContent("")
}
