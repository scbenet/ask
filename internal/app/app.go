package app

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/scbenet/ask/internal/llm"
	"github.com/scbenet/ask/internal/ui"
	"github.com/scbenet/ask/internal/ui/modelpicker"
	// "github.com/charmbracelet/bubbles/filepicker"
)

// define different views/states the application can be in
type viewState int

const (
	chatView viewState = iota
	modelPickerView
	// filePickerView
)

type App struct {
	width  int
	height int

	activeView  viewState
	chat        *ui.Chat
	modelPicker *modelpicker.Model
	// filePicker filepicker.Model
	llmClient llm.LLMClient
	helpF     *help.Model

	// State
	selectedModel       string
	conversationHistory []llm.Message
	streamChan          chan tea.Msg

	// keybindings
	quitKey        key.Binding
	modelPickerKey key.Binding
	lastError      error
}

func New() *App {
	// init chat view
	chatModel := ui.New(80, 24)

	// TODO move this to a config file or something
	availableModels := []string{
		"google/gemini-2.5-flash-preview",
		"google/gemini-2.5-pro-preview",
		"openai/o4-mini-high",
		"openai/o3",
		"openai/gpt-4.1",
		"deepseek/deepseek-chat-v3-0324",
		"microsoft/mai-ds-r1:free",
		"anthropic/claude-3.7-sonnet",
		"anthropic/claude-3.7-sonnet:thinking",
	}

	mp := modelpicker.New(availableModels)

	// --- File Picker Setup (Keep placeholder) ---
	//fp := filepicker.New()
	//fp.CurrentDirectory = "."

	// --- LLM Client Setup ---
	llmSvc, err := llm.NewOpenRouterClient()
	if err != nil {
		log.Printf("Error initializing openrouter client: %v", err)
		os.Exit(1)
	}

	defaultModel := availableModels[0]

	return &App{
		activeView:  chatView,
		chat:        chatModel,
		modelPicker: mp,
		// filePicker:    fp,
		llmClient:           llmSvc,
		conversationHistory: []llm.Message{},
		selectedModel:       defaultModel,

		quitKey: key.NewBinding(
			key.WithKeys("ctrl+c"),
			key.WithHelp("ctrl+c", "quit"),
		),
		modelPickerKey: key.NewBinding(
			key.WithKeys("ctrl+k"),
			key.WithHelp("ctrl+k", "models"),
		),
		// filePickerKey: key.NewBinding(
		// 	key.WithKeys("ctrl+f"),
		// 	key.WithHelp("ctrl+f", "context"),
		// ),
	}
}

// Init function initializes the application
func (a *App) Init() tea.Cmd {
	return a.chat.Init()
	// return tea.Batch(a.chat.Init(), a.filePicker.Init())
}

// helper function to create a command that listens to our stream channel
func listenToStream(ch chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			// channel has been closed by the sender
			// this implies the stream has ended (either with StreamEndMsg or StreamErrorMsg)
			return nil
		}
		return msg
	}
}

// Update function handles messages for the entire application
// delegates messages to the active view or handles global actions
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = m.Width
		a.height = m.Height
		// chat view handles its own resize logic internally
		chatModel, chatCmd := a.chat.Update(msg)
		a.chat = chatModel.(*ui.Chat) // type assertion needed TODO: figure out what this does lol
		cmds = append(cmds, chatCmd)
		// also send resize to model picker (it expects full window size)
		pickerModel, pickerCmd := a.modelPicker.Update(msg)
		a.modelPicker = pickerModel.(*modelpicker.Model)
		cmds = append(cmds, pickerCmd)

		// Send resize to file picker
		// fpModel, fpCmd := a.filePicker.Update(msg)
		// a.filePicker = fpModel.(filepicker.Model)
		// cmds = append(cmds, fpCmd)

	// -- handle key messages --
	case tea.KeyMsg:
		switch a.activeView {
		case chatView:
			if key.Matches(m, a.quitKey) {
				log.Printf("Key '%s' received in chatView", m.String())
				return a, tea.Quit
			}

			if key.Matches(m, a.modelPickerKey) {
				// ensure no active stream before switching views
				// could cancel the stream here instead (probably better to listen to the user)
				// but not all providers support stream cancellation (looking at you, google!)
				if a.streamChan != nil {
					log.Println("model picker key pressed during active stream, ignoring for now")
					return a, nil
				}
				a.activeView = modelPickerView
				a.modelPicker.SetTitle(fmt.Sprintf("Select a model (current: %s)", a.selectedModel))
				return a, nil
			}
			chatModel, chatCmd := a.chat.Update(msg)
			a.chat = chatModel.(*ui.Chat) // type assertion
			cmds = append(cmds, chatCmd)

		case modelPickerView:
			// TODO make ctrl-c from modelPicker to go back to chat

			pickerModel, pickerCmd := a.modelPicker.Update(msg)
			a.modelPicker = pickerModel.(*modelpicker.Model)
			cmds = append(cmds, pickerCmd)
		}

	// --- handle other message types ---
	case modelpicker.ModelSelectedMsg:
		log.Printf("ModelSelectedMsg received: %s", m.Model)
		a.selectedModel = m.Model
		a.activeView = chatView

	// TODO send this event from model picker on cancel key press
	case modelpicker.PickerCancelledMsg:
		log.Printf("PickerCancelledMsg received")
		a.activeView = chatView

	case ui.SendPromptMsg:
		// prevent multiple concurrent streams
		if a.streamChan != nil {
			log.Println("SendPromptMsg received while a stream is already active, ignoring...")
			return a, nil
		}
		a.chat.SetSending(true)
		log.Printf("SetSending: true")
		prompt := m.Prompt
		model := a.selectedModel
		log.Printf("Prompt: %s\nModel: %s", prompt, model)

		a.conversationHistory = append(a.conversationHistory, llm.Message{
			Role:    "user",
			Content: prompt,
		})
		historyCopy := make([]llm.Message, len(a.conversationHistory))
		copy(historyCopy, a.conversationHistory)
		log.Printf("History length for stream: %d", len(historyCopy))

		a.streamChan = make(chan tea.Msg) // create new channel for this stream
		go a.llmClient.StreamGenerate(context.Background(), model, historyCopy, a.streamChan)
		cmds = append(cmds, listenToStream(a.streamChan)) // start listening

	case llm.StreamChunkMsg:
		log.Printf("StreamChunkMsg received in app")
		if a.activeView == chatView {
			// pass chunk to chat for rendering
			chatModel, chatCmd := a.chat.Update(m)
			a.chat = chatModel.(*ui.Chat)
			cmds = append(cmds, chatCmd)
		}
		// continue listening for more chunks
		if a.streamChan != nil {
			cmds = append(cmds, listenToStream(a.streamChan))
		}

	case llm.StreamEndMsg:
		log.Printf("StreamEndMsg received in app, full response length: %d", len(m.FullResponse))
		// add complete response to conversation history
		a.conversationHistory = append(a.conversationHistory, llm.Message{
			Role:    "assistant",
			Content: m.FullResponse,
		})
		if a.activeView == chatView {
			responseDoneMsg := ui.StreamEndMsg{FullResponse: m.FullResponse}
			chatModel, chatCmd := a.chat.Update(responseDoneMsg)
			a.chat = chatModel.(*ui.Chat)
			cmds = append(cmds, chatCmd)
			a.chat.SetSending(false)
		}
		// done streaming, won't need this anymore
		a.streamChan = nil

	case llm.StreamErrorMsg:
		a.lastError = m.Err
		log.Printf("StreamErrorMsg received in app: %v", m.Err)
		errMsg := fmt.Sprintf("assistant stream error: %s", m.Err.Error())
		// display error in chat view
		errorReply := ui.StreamErrorMsg{Err: errMsg}
		if a.activeView == chatView {
			chatModel, chatCmd := a.chat.Update(errorReply) // Send error to chat
			a.chat = chatModel.(*ui.Chat)
			cmds = append(cmds, chatCmd)
			a.chat.SetSending(false) // Signal sending is done (due to error)
		}
		a.streamChan = nil

	// non-streaming response message
	case ui.LLMReplyMsg:
		log.Printf("LLMReplyMsg received")
		if a.activeView == chatView {
			chatModel, chatCmd := a.chat.Update(msg)
			a.chat = chatModel.(*ui.Chat)
			cmds = append(cmds, chatCmd)
			a.chat.SetSending(false)
		} else {
			log.Printf("LLMReplyMsg received but not in chatView, ignoring.")
		}

	// non-streaming response error message
	case llm.GenerationErrorMsg:
		a.lastError = m.Err
		// TODO: Display this error nicely, maybe append to chat history
		log.Printf("LLMError received: %s", a.lastError)
		errMsg := fmt.Sprintf("Assistant Error: %s", m.Err.Error())
		errorReply := ui.LLMReplyMsg{Content: errMsg} // Send as a reply
		chatModel, chatCmd := a.chat.Update(errorReply)
		a.chat = chatModel.(*ui.Chat)
		cmds = append(cmds, chatCmd)
		a.chat.SetSending(false)

	default:
		switch a.activeView {
		case chatView:
			chatModel, chatCmd := a.chat.Update(msg)
			a.chat = chatModel.(*ui.Chat)
			cmds = append(cmds, chatCmd)
		case modelPickerView:
			pickerModel, pickerCmd := a.modelPicker.Update(msg)
			a.modelPicker = pickerModel.(*modelpicker.Model)
			cmds = append(cmds, pickerCmd)
		}
	}
	return a, tea.Batch(cmds...)
}

// View renders the view for the currently active model.
func (a *App) View() string {
	switch a.activeView {
	case chatView:
		return a.chat.View()
	case modelPickerView:
		return a.modelPicker.View()
	// case contextPickerView:
	// 	return a.contextPicker.View()
	default:
		log.Printf("Error: Unknown view state in View(): %v", a.activeView)
		return "Unknown view state" // Should not happen
	}

	// TODO add header or footer showing selected model and available keyboard shortcuts?
}
