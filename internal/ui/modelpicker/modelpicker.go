package modelpicker

import (
	"fmt"
	"io"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Model struct {
	list         list.Model
	selectedItem string // store the selected item temporarily?
}

type Item string

// ModelSelectedMsg is emitted when a new model is selected
type ModelSelectedMsg struct {
	Model string
}

type PickerCancelledMsg struct{}

func (i Item) FilterValue() string {
	return string(i)
}

type itemDelegate struct{}

func (d itemDelegate) Height() int {
	return 1
}
func (d itemDelegate) Spacing() int {
	return 0
}

func (d itemDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return nil
}

func (d itemDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	i, ok := listItem.(Item)
	if !ok {
		return
	}

	str := fmt.Sprintf("%d. %s", index+1, i)

	fn := lipgloss.NewStyle().PaddingLeft(4).Render
	if index == m.Index() {
		// style for currently selected item
		fn = func(s ...string) string {
			return lipgloss.NewStyle().
				PaddingLeft(2).
				Foreground(lipgloss.Color("#7D56F4")).
				Render("> " + s[0])
		}
	}

	fmt.Fprint(w, fn(str))
}

// creates new model picker component
func New(modelNames []string) *Model {
	items := make([]list.Item, len(modelNames))
	for i, name := range modelNames {
		items[i] = Item(name)
	}

	const defaultWidth = 40
	const listHeight = 14

	l := list.New(items, itemDelegate{}, defaultWidth, listHeight)
	l.Title = "Select your model"
	l.SetShowStatusBar(false)
	l.SetFilteringEnabled(true)
	l.Styles.Title = lipgloss.NewStyle().
		Foreground(lipgloss.Color("#FFFDF5")).
		Background(lipgloss.Color("#7D56F4")).
		Padding(0, 1)
	l.Styles.PaginationStyle = list.DefaultStyles().PaginationStyle.PaddingLeft(4)
	l.Styles.HelpStyle = list.DefaultStyles().HelpStyle.PaddingLeft(4).PaddingBottom(1)

	return &Model{list: l}
}

// initializes model picker, currently does nothing
func (m *Model) Init() tea.Cmd {
	return nil
}

// update handles messages for the model picker
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:

		m.list.SetWidth(msg.Width)
		m.list.SetHeight(msg.Height - 4)
		return m, nil

	case tea.KeyMsg:

		switch msg.String() {

		case "enter":
			selected, ok := m.list.SelectedItem().(Item)
			if ok {
				m.selectedItem = string(selected)
				return m, func() tea.Msg {
					return ModelSelectedMsg{Model: m.selectedItem}
				}
			}
		}
	}

	// delegate non handled messages (inlcuding navigation)
	// to the underlying list model
	m.list, cmd = m.list.Update(msg)
	return m, cmd
}

func (m *Model) View() string {
	return "\n" + m.list.View()
}

func (m *Model) SetTitle(title string) {
	m.list.Title = title
}
