package ui

import (
	"fmt"
	"log"
	"strings"

	"github.com/charmbracelet/bubbles/help"
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

type keyMap struct {
	SendPrompt   key.Binding
	NewLine      key.Binding
	ModelPicker  key.Binding
	PageDown     key.Binding
	PageUp       key.Binding
	HalfPageUp   key.Binding
	HalfPageDown key.Binding
	Up           key.Binding
	Down         key.Binding
	Help         key.Binding
	Quit         key.Binding
}

// ShortHelp returns keybindings to be shown in the mini help view. It's part
// of the key.Map interface.
func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.SendPrompt, k.NewLine, k.ModelPicker, k.Quit}
}

// FullHelp returns keybindings for the expanded help view. It's part of the
// key.Map interface.
func (k keyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.PageUp, k.PageDown, k.HalfPageUp, k.HalfPageDown}, // first column
		{k.Up, k.Down, k.SendPrompt, k.NewLine},              // second column
		{k.ModelPicker, k.Help, k.Quit},
	}
}

var keys = keyMap{
	PageDown: key.NewBinding(
		key.WithKeys("pgdown", "ctrl+f"),
		key.WithHelp("ctrl+f/pgdn", "page down"),
	),
	PageUp: key.NewBinding(
		key.WithKeys("pgup", "ctrl+b"),
		key.WithHelp("ctrl+b/pgup", "page up"),
	),
	HalfPageUp: key.NewBinding(
		key.WithKeys("ctrl+u"),
		key.WithHelp("ctrl+u", "½ page up"),
	),
	HalfPageDown: key.NewBinding(
		key.WithKeys("ctrl+d"),
		key.WithHelp("ctrl+d", "½ page down"),
	),
	Up: key.NewBinding(
		key.WithKeys("up", "ctrl+o"),
		key.WithHelp("↑/ctrl+o", "up"),
	),
	Down: key.NewBinding(
		key.WithKeys("down", "ctrl+p"),
		key.WithHelp("↓/ctrl+p", "down"),
	),
	SendPrompt: key.NewBinding(
		key.WithKeys("enter"),
		key.WithHelp("enter", "send your message"),
	),
	NewLine: key.NewBinding(
		key.WithKeys("shift+enter", "ctrl+j"),
		key.WithHelp("⇧enter/ctrl-j", "new line"),
	),
	ModelPicker: key.NewBinding(
		key.WithKeys("ctrl-k"),
		key.WithHelp("ctrl-k", "open model picker"),
	),
	Help: key.NewBinding(
		key.WithKeys("ctrl-q"),
		key.WithHelp("ctrl-q", "toggle help"),
	),
	Quit: key.NewBinding(
		key.WithKeys("ctrl+c"),
		key.WithHelp("ctrl-c", "quit"),
	),
}

// Chat is the main chat view (history + input field).
type Chat struct {
	history viewport.Model
	input   textarea.Model
	keys    keyMap
	help    help.Model

	sending           bool // true while waiting for the model response to finish
	historyBuf        strings.Builder
	assistantResponse strings.Builder // builds current assistant message during streaming

	sendKey key.Binding

	// style handles
	userStyle        lipgloss.Style
	assistantStyle   lipgloss.Style
	errorStyle       lipgloss.Style
	borderStyle      lipgloss.Style
	historyViewStyle lipgloss.Style
}

func (c *Chat) SetSending(sending bool) {
	c.sending = sending
	if sending {
		c.input.Placeholder = "Assistant is thinking..."
		c.assistantResponse.Reset() // ensure the buffer for the current response is clean
	} else {
		c.input.Placeholder = "Write a message…"
	}

	c.history.SetContent(c.historyBuf.String())
	c.history.GotoBottom()
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
		key.WithHelp("⇧enter/ctrl-j", "new line"),
	)

	// viewport (scrollable chat history)
	vp := viewport.New(width, 0)
	vp.KeyMap = CustomKeyMap()
	vp.SetContent("")

	helpModel := help.New()

	c := &Chat{
		history:          vp,
		input:            ti,
		keys:             keys,
		help:             helpModel,
		sendKey:          key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "send")),
		userStyle:        lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("#707070")),
		assistantStyle:   lipgloss.NewStyle(),
		errorStyle:       lipgloss.NewStyle().Foreground(lipgloss.Color("9")), // red for errors
		borderStyle:      lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("#777")),
		historyViewStyle: lipgloss.NewStyle().Padding(0, 1),
	}
	// set initial history width based on input width, will be refined by WindowSizeMsg
	// this is a fallback in case WindowSizeMsg is not received immediately or if needed before it.
	hPadding := c.historyViewStyle.GetPaddingLeft() + c.historyViewStyle.GetPaddingRight()
	c.history.Width = width - hPadding

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

	// ensure history width is positive for wrapping, default to a minimum if not.
	wrapWidth := c.history.Width
	if wrapWidth <= 0 {
		wrapWidth = 80 // A sensible default
	}

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
			rawUserMessage := fmt.Sprintf("> %s", prompt)
			styledAndWrappedUserMessage := c.userStyle.Width(wrapWidth).Render(rawUserMessage)
			fmt.Fprintf(&c.historyBuf, "%s\n\n", styledAndWrappedUserMessage)

			c.history.SetContent(c.historyBuf.String())
			c.history.GotoBottom()
			c.input.Reset()

			cmd = func() tea.Msg { return SendPromptMsg{Prompt: prompt} }
			cmds = append(cmds, cmd)

		case key.Matches(m, c.keys.Help):
			log.Println("Chat.Update: help key triggered")
			c.help.ShowAll = !c.help.ShowAll

		default:
			// pass messages to nested models
			var tiCmd, vpCmd, helpCmd tea.Cmd
			c.input, tiCmd = c.input.Update(msg)
			c.history, vpCmd = c.history.Update(msg)
			c.help, helpCmd = c.help.Update(msg)
			cmds = append(cmds, tiCmd, vpCmd, helpCmd)
		}

	case llm.StreamChunkMsg:
		log.Printf("Chat.Update: StreamChunkMsg received: '%s'", m.Content)
		c.assistantResponse.WriteString(m.Content) // add to temporary buffer for current response

		rawCurrentResponse := c.assistantResponse.String()
		styledAndWrappedResponse := c.assistantStyle.Width(wrapWidth).Render(rawCurrentResponse)

		// combine finalized history with currently streaming message
		c.history.SetContent(c.historyBuf.String() + styledAndWrappedResponse)
		c.history.GotoBottom()

	case StreamEndMsg:
		log.Printf("Chat.Update: StreamEndMsg received. Full response was: %s", m.FullResponse)
		styledAndWrappedFinalResponse := c.assistantStyle.Width(wrapWidth).Render(m.FullResponse)
		fmt.Fprintf(&c.historyBuf, "%s\n\n", styledAndWrappedFinalResponse)
		c.assistantResponse.Reset()
		c.history.SetContent(c.historyBuf.String())
		c.history.GotoBottom()

	case StreamErrorMsg:
		log.Printf("Chat.Update: StreamErrorMsg received: %s", m.Err)
		styledAndWrappedError := c.errorStyle.Width(wrapWidth).Render(m.Err)
		fmt.Fprintf(&c.historyBuf, "%s\n\n", styledAndWrappedError)

		c.assistantResponse.Reset() // Clear any partial streaming response
		c.history.SetContent(c.historyBuf.String())
		c.history.GotoBottom()

	// primarily for non-streaming or error messages
	case LLMReplyMsg:
		log.Printf("Chat.Update: LLMReplyMsg received: '%s'", m.Content)
		styledAndWrappedResponse := c.assistantStyle.Width(wrapWidth).Render(m.Content)
		fmt.Fprintf(&c.historyBuf, "%s\n\n", styledAndWrappedResponse)

		c.history.SetContent(c.historyBuf.String())
		c.history.GotoBottom()
		c.assistantResponse.Reset()
		log.Println("Chat.Update: Appended LLMReplyMsg")

	case tea.WindowSizeMsg:
		inputHeight := lipgloss.Height(c.borderStyle.Render(c.input.View()))
		helpHeight := lipgloss.Height(c.help.View(c.keys))

		// adjust history viewport size for padding
		hPadding := c.historyViewStyle.GetPaddingLeft() + c.historyViewStyle.GetPaddingRight()
		vPadding := c.historyViewStyle.GetPaddingTop() + c.historyViewStyle.GetPaddingBottom()

		newHistoryWidth := max(m.Width-hPadding, 1)
		c.history.Width = newHistoryWidth
		c.history.Height = m.Height - inputHeight - vPadding - helpHeight

		c.input.SetWidth(m.Width - 2) // -2 for border
		c.help.Width = m.Width - hPadding

		// after a resize, re-set content to allow existing history to re-wrap if needed
		// history contains pre-warpped strings, so old messages will not re-wrap, but
		// new messages will be wrapped correctly
		if c.sending && c.assistantResponse.Len() > 0 {
			rawCurrentResponse := c.assistantResponse.String()
			styledAndWrappedResponse := c.assistantStyle.Width(c.history.Width).Render(rawCurrentResponse)
			c.history.SetContent(c.historyBuf.String() + styledAndWrappedResponse)
		} else {
			c.history.SetContent(c.historyBuf.String())
		}
		// ensure view is scrolled properly after resize
		c.history.GotoBottom()
	}

	return c, tea.Batch(cmds...)
}

// View implements tea.Model.
func (c *Chat) View() string {
	inputView := c.borderStyle.Render(c.input.View())
	historyView := c.historyViewStyle.Render(c.history.View())
	helpView := c.historyViewStyle.Render(c.help.View(c.keys))
	return lipgloss.JoinVertical(lipgloss.Left, historyView, inputView, helpView)
}

func (c *Chat) ClearHistory() {
	c.historyBuf.Reset()
	c.assistantResponse.Reset()
	c.history.SetContent("")
}
