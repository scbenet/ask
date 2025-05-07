package app

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/scbenet/ask/internal/llm"
	"github.com/scbenet/ask/internal/ui"
	"github.com/scbenet/ask/internal/ui/modelpicker"
	// "github.com/charmbracelet/bubbles/filepicker" Keep for later
)

// define different views/states the application can be in
type viewState int

// add other views later
const (
	chatView viewState = iota
	modelPickerView
	// filePickerView // Keep for later
)

type App struct {
	width  int
	height int

	activeView  viewState
	chat        *ui.Chat
	modelPicker *modelpicker.Model
	// filePicker filepicker.Model // Keep for later
	llmClient           llm.LLMClient
	conversationHistory []llm.Message
	//add other view models later

	// State
	selectedModel string

	// keybindings
	quitKey        key.Binding
	modelPickerKey key.Binding
	lastError      error // To potentially display errors
}

func New() *App {
	// init chat view
	chatModel := ui.New(80, 24)

	availableModels := []string{
		"google/gemini-2.5-flash-preview",
		"google/gemini-2.5-pro-preview/03-25",
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
		// fall back to dummy client if openrouter client creation fails
		log.Printf("Error initializing openrouter client: %v. Falling back to dummy client", err)
		if err != nil {
			fmt.Println("Error initializing LLM Client:", err)
			os.Exit(1)
		}
	}

	defaultModel := availableModels[0]

	return &App{
		activeView:  chatView,
		chat:        chatModel,
		modelPicker: mp,
		// filePicker:    fp, // Keep for later
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
		// filePickerKey: key.NewBinding( // Keep for later
		// 	key.WithKeys("ctrl+f"),
		// 	key.WithHelp("ctrl+f", "context"),
		// ),
	}
}

// Init function initializes the application
func (a *App) Init() tea.Cmd {
	// Call the Init method of the initial active view (the chat view)
	// and return its command (which should be textarea.Blink).
	return a.chat.Init()
	// return tea.Batch(a.chat.Init(), a.filePicker.Init()) // Add filepicker later

}

// Update function handles messages for the entire application
// delegates messages to the active view or handles global actions
func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	log.Printf("App.Update received msg type: %T", msg)
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width = m.Width
		a.height = m.Height
		// propogate resize message to all views
		// chat view handles its own resize logic internally
		chatModel, chatCmd := a.chat.Update(msg)
		a.chat = chatModel.(*ui.Chat) // type assertion needed TODO: figure out what this does lol
		cmds = append(cmds, chatCmd)
		// also send resize to model picker (it expects full window size)
		pickerModel, pickerCmd := a.modelPicker.Update(msg)
		a.modelPicker = pickerModel.(*modelpicker.Model)
		cmds = append(cmds, pickerCmd)

		// Send resize to file picker later
		// fpModel, fpCmd := a.filePicker.Update(msg)
		// a.filePicker = fpModel.(filepicker.Model)
		// cmds = append(cmds, fpCmd)

	case tea.KeyMsg:
		// global keybindings
		log.Printf("KeyMsg received: %s (ActiveView: %v)", m.String(), a.activeView)
		// --- message delegation ---
		// if KeyMsg isn't a blobal keybinding, delegate it to the active view
		switch a.activeView {
		case chatView:

			if key.Matches(m, a.quitKey) {
				log.Printf("Key '%s' received in chatView", m.String())
				return a, tea.Quit
			}

			// *** Explicitly check model picker key ***
			isModelPickerKey := key.Matches(m, a.modelPickerKey)
			// log.Printf("Checking for modelPickerKey (Ctrl+K): Matched = %t", isModelPickerKey)

			if isModelPickerKey {
				// log.Printf(">>> Model picker key matched! Switching view.")
				a.activeView = modelPickerView
				// Update picker title to show current selection
				a.modelPicker.SetTitle(fmt.Sprintf("Select a model (current: %s)", a.selectedModel))
				// log.Printf(">>> ActiveView is now: %v. Returning.", a.activeView)
				// Potentially reset list focus/filter: cmds = append(cmds, a.modelPicker.Init())?
				return a, nil
			}

			// if key.Matches(m, a.filePickerKey) { ... } // Add later

			// If not a global key handled above, delegate to chat view
			log.Printf("Key '%s' not handled globally in chatView, delegating to chat.Update", m.String())
			chatModel, chatCmd := a.chat.Update(msg)
			// IMPORTANT: update the model reference
			a.chat = chatModel.(*ui.Chat) // type assertion
			cmds = append(cmds, chatCmd)

		case modelPickerView:
			log.Printf("Key '%s' received in modelPickerView", m.String())
			// if key.Matches(m, a.quitKey) {
			// 	log.Printf("Quit key matched in modelPickerView")
			// 	return a, tea.Quit
			// }

			log.Printf("Delegating key '%s' to modelPicker.Update", m.String())
			pickerModel, pickerCmd := a.modelPicker.Update(msg)
			a.modelPicker = pickerModel.(*modelpicker.Model)
			cmds = append(cmds, pickerCmd)
			// Handle messages from picker, like ModelSelectedMsg
			// if modelSelected { a.activeView = chatView }
		}

	// --- handle other message types ---
	// messages specific to certain views might need routing
	// or could be handled directly by the child model if forwarded
	case modelpicker.ModelSelectedMsg:
		log.Printf("ModelSelectedMsg received: %s", m.Model)
		a.selectedModel = m.Model
		a.activeView = chatView
		log.Printf("Switched back to chatView after model selection.")

	// TODO send this event from model picker on cancel key press
	case modelpicker.PickerCancelledMsg:
		log.Printf("PickerCancelledMsg received")
		a.activeView = chatView

	case ui.SendPromptMsg:
		a.chat.SetSending(true)
		log.Printf("SetSending: true")
		prompt := m.Prompt
		model := a.selectedModel
		log.Printf("Prompt: %s\nModel: %s", prompt, model)

		a.conversationHistory = append(a.conversationHistory, llm.Message{
			Role:    "user",
			Content: prompt,
		})

		log.Printf("Prompt: %s\nModel: %s\nHistory length: %d", prompt, model, len(a.conversationHistory))

		// contextText := a.contextContentd
		cmd := func() tea.Msg {
			response, err := a.llmClient.Generate(context.Background(), model, prompt, a.conversationHistory)
			if err != nil {
				return llm.ErrorMsg{Err: err}
			}

			// add the assistant's response to history
			a.conversationHistory = append(a.conversationHistory, llm.Message{
				Role:    "assistant",
				Content: response,
			})

			return ui.LLMReplyMsg{Content: response}
		}
		cmds = append(cmds, cmd)

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

	case llm.ErrorMsg: // Message from the LLM command function (failure)
		a.lastError = m.Err
		// TODO: Display this error nicely, maybe append to chat history
		log.Printf("LLMError received: %s", a.lastError)
		errMsg := fmt.Sprintf("Assistant Error: %s", m.Err.Error())
		errorReply := ui.LLMReplyMsg{Content: errMsg} // Send as a reply
		chatModel, chatCmd := a.chat.Update(errorReply)
		a.chat = chatModel.(*ui.Chat)
		cmds = append(cmds, chatCmd)
		a.chat.SetSending(false) // Still need to reset sending state

	default:
		// if the message type isn't handled globally or specifically routed,
		// route it to the active view just in case
		// TODO will need to make sure child models handle unexpected types gracefully
		log.Printf("Default message case reached for type %T, delegating based on active view (%v)", msg, a.activeView)
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

	log.Printf("App.Update returning with %d commands", len(cmds))
	return a, tea.Batch(cmds...)
}

// View renders the view for the currently active model.
func (a *App) View() string {
	log.Printf("App.View called. ActiveView: %v", a.activeView)
	// Delegate rendering to the active view's View method
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
	// You could add a global header/footer here if desired,
	// wrapping the result of the active view.
	// Example: return lipgloss.JoinVertical(lipgloss.Left, header, activeViewContent, footer)
}
