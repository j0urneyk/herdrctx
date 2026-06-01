package ui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"

	"github.com/j0urneyk/herdrctx/internal/herdr"
)

const (
	defaultRefreshInterval = 3 * time.Second
	refreshIndicatorDelay  = 300 * time.Millisecond
)

type statusKind int

const (
	statusInfo statusKind = iota
	statusSuccess
	statusWarning
	statusError
)

// Options configures the TUI model.
type Options struct {
	Client               *herdr.Client
	Context              context.Context
	Cancel               context.CancelFunc
	RefreshInterval      time.Duration
	Version              string
	DefaultDir           string
	CompleteHidden       CompletionHiddenMode
	CompleteVisibleCount int
	InsideHerdr          bool
	CurrentSocketPath    string
	AllowNested          bool
}

type model struct {
	client          *herdr.Client
	ctx             context.Context
	cancel          context.CancelFunc
	refreshInterval time.Duration
	version         string

	keys    keyMap
	help    help.Model
	table   table.Model
	spinner spinner.Model

	sessions   []herdr.Session
	dialog     *alertDialog
	confirm    *confirmation
	newSession *newSessionForm

	defaultDir           string
	completeHidden       CompletionHiddenMode
	completeVisibleCount int
	insideHerdr          bool
	currentSocketPath    string
	allowNested          bool

	width  int
	height int

	loading            bool
	busy               string
	showRefreshing     bool
	refreshIndicatorID uint64
	refreshRequestID   uint64

	lastRefresh time.Time
	status      string
	statusKind  statusKind
}

type refreshTickMsg time.Time

type refreshIndicatorMsg uint64

type sessionsLoadedMsg struct {
	RequestID   uint64
	Sessions    []herdr.Session
	Err         error
	RefreshedAt time.Time
}

type actionFinishedMsg struct {
	Action confirmAction
	Name   string
	Err    error
}

type attachFinishedMsg struct {
	Name string
	Err  error
}

// NewModel returns a Bubble Tea model for the Herdr sessions TUI.
func NewModel(opts Options) tea.Model {
	client := opts.Client
	if client == nil {
		client = herdr.NewClient("herdr")
	}

	interval := opts.RefreshInterval
	if interval <= 0 {
		interval = defaultRefreshInterval
	}

	defaultDir := opts.DefaultDir
	if defaultDir == "" {
		var err error
		defaultDir, err = os.Getwd()
		if err != nil {
			defaultDir = "."
		}
	}

	keys := defaultKeyMap()
	tableKeys := table.DefaultKeyMap()
	tableKeys.HalfPageDown.SetKeys("ctrl+d")

	t := table.New(
		table.WithFocused(true),
		table.WithKeyMap(tableKeys),
		table.WithStyles(tableStyles()),
	)

	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	cancel := opts.Cancel
	if cancel == nil {
		ctx, cancel = context.WithCancel(ctx)
	}
	m := model{
		client:               client,
		ctx:                  ctx,
		cancel:               cancel,
		refreshInterval:      interval,
		version:              opts.Version,
		keys:                 keys,
		help:                 help.New(),
		table:                t,
		spinner:              spinnerModel(),
		defaultDir:           defaultDir,
		completeHidden:       opts.CompleteHidden.withDefault(),
		completeVisibleCount: completionVisibleCountWithDefault(opts.CompleteVisibleCount),
		insideHerdr:          opts.InsideHerdr,
		currentSocketPath:    opts.CurrentSocketPath,
		allowNested:          opts.AllowNested,
		loading:              true,
		refreshRequestID:     1,
		status:               "Loading sessions…",
		statusKind:           statusInfo,
	}
	m.configureTable()

	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(m.loadSessionsCmd(), m.scheduleRefresh(), m.spinner.Tick)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	if spinnerMsg, ok := msg.(spinner.TickMsg); ok {
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(spinnerMsg)
		if m.loading || m.busy != "" {
			cmds = append(cmds, cmd)
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.help.SetWidth(msg.Width)
		if m.newSession != nil {
			m.newSession.setWidth(msg.Width)
		}
		m.configureTable()
		return m, tea.Batch(cmds...)

	case refreshTickMsg:
		cmds = append(cmds, m.scheduleRefresh())
		if !m.loading && m.busy == "" {
			cmds = append(cmds, m.startRefresh(), m.loadSessionsCmd(), m.spinner.Tick)
		}
		return m, tea.Batch(cmds...)

	case refreshIndicatorMsg:
		if m.loading && uint64(msg) == m.refreshIndicatorID {
			m.showRefreshing = true
		}
		return m, tea.Batch(cmds...)

	case sessionsLoadedMsg:
		if msg.RequestID != 0 && msg.RequestID != m.refreshRequestID {
			return m, tea.Batch(cmds...)
		}

		m.loading = false
		m.showRefreshing = false
		if msg.Err != nil {
			m.setStatus(fmt.Sprintf("Refresh failed: %v", msg.Err), statusError)
			return m, tea.Batch(cmds...)
		}

		m.setSessions(msg.Sessions)
		m.lastRefresh = msg.RefreshedAt
		m.setStatus(fmt.Sprintf("Loaded %d session(s).", len(msg.Sessions)), statusSuccess)
		return m, tea.Batch(cmds...)

	case actionFinishedMsg:
		m.busy = ""
		if msg.Err != nil {
			m.showDialog(dialogError, fmt.Sprintf("%s failed", msg.Action), fmt.Sprintf("%s %q failed:\n\n%v", msg.Action, msg.Name, msg.Err))
			cmds = append(cmds, m.startRefresh())
			return m, tea.Batch(append(cmds, m.loadSessionsCmd(), m.spinner.Tick)...)
		}

		cmds = append(cmds, m.startRefresh())
		m.setStatus(fmt.Sprintf("%s %q complete.", msg.Action, msg.Name), statusSuccess)
		return m, tea.Batch(append(cmds, m.loadSessionsCmd(), m.spinner.Tick)...)

	case attachFinishedMsg:
		m.busy = ""
		cmds = append(cmds, m.startRefresh())
		if msg.Err != nil {
			m.showDialog(dialogError, "Attach failed", fmt.Sprintf("Attach %q failed:\n\n%v", msg.Name, msg.Err))
		} else {
			m.setStatus(fmt.Sprintf("Returned from %q.", msg.Name), statusInfo)
		}
		return m, tea.Batch(append(cmds, m.loadSessionsCmd(), m.spinner.Tick)...)

	case tea.KeyPressMsg:
		updated, cmd := m.handleKey(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return updated, tea.Batch(cmds...)
	}

	if m.newSession != nil {
		form := *m.newSession
		var cmd tea.Cmd
		form, cmd = form.update(msg, m.keys)
		m.newSession = &form
		m.configureTable()
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m model) View() tea.View {
	content := m.render()
	view := tea.NewView(content)
	view.AltScreen = true
	view.WindowTitle = "herdrctx"
	return view
}

func (m model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	return dispatchInputKey(m, msg, m.inputLayers())
}

func (m model) quitCmd() tea.Cmd {
	if m.cancel != nil {
		m.cancel()
	}

	return tea.Quit
}

func (m model) handleBusyKey(msg tea.KeyPressMsg) model {
	switch {
	case key.Matches(msg, m.keys.Up):
		m.table.MoveUp(1)
	case key.Matches(msg, m.keys.Down):
		m.table.MoveDown(1)
	case key.Matches(msg, m.keys.Help):
		m.help.ShowAll = !m.help.ShowAll
	}

	return m
}

func (m model) handleConfirmationKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if key.Matches(msg, m.keys.Cancel) {
		m.confirm = nil
		m.setStatus("Action cancelled.", statusInfo)
		return m, nil
	}

	if !key.Matches(msg, m.keys.Confirm) {
		return m, nil
	}

	confirmed := *m.confirm
	m.confirm = nil
	if !m.revalidateConfirmation(&confirmed) {
		return m, nil
	}

	m.busy = fmt.Sprintf("%s %q", confirmed.Action, confirmed.Session.Name)
	m.setStatus(fmt.Sprintf("Running %s for %q…", confirmed.Action, confirmed.Session.Name), statusInfo)

	return m, tea.Batch(m.runActionCmd(confirmed), m.spinner.Tick)
}

func (m model) handleNewSessionKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.newSession == nil {
		return m, nil
	}

	switch {
	case key.Matches(msg, m.keys.Dismiss):
		m.newSession = nil
		m.setStatus("New session cancelled.", statusInfo)
		m.configureTable()
		return m, nil
	case msg.String() == "enter":
		return m.submitNewSession()
	}

	form := *m.newSession
	var cmd tea.Cmd
	form, cmd = form.update(msg, m.keys)
	m.newSession = &form
	m.configureTable()
	return m, cmd
}

func (m model) openNewSession(mode newSessionMode) (tea.Model, tea.Cmd) {
	if m.nestedHerdrBlocked() {
		m.showDialog(dialogWarning, "Cannot create from inside Herdr", m.nestedHerdrCreateMessage())
		return m, nil
	}

	form, cmd := newSessionFormModel(mode, m.defaultDir, m.width, m.completeHidden, m.completeVisibleCount)
	m.newSession = &form
	m.configureTable()
	m.setStatus("Create a new session and attach immediately.", statusInfo)
	return m, cmd
}

func (m model) submitNewSession() (tea.Model, tea.Cmd) {
	if m.newSession == nil {
		return m, nil
	}

	form := *m.newSession
	submission, err := form.submit()
	if err != nil {
		form.setError(err)
		form.closeCompletion()
		cmd := form.focusActive()
		m.newSession = &form
		m.configureTable()
		return m, cmd
	}
	cmd, err := m.client.CreateAttachCommand(submission.Name, submission.Dir)
	if err != nil {
		form.setError(err)
		form.closeCompletion()
		focusCmd := form.focusActive()
		m.newSession = &form
		m.configureTable()
		return m, focusCmd
	}

	m.newSession = nil
	m.busy = fmt.Sprintf("new session %q", submission.Name)
	m.setStatus(fmt.Sprintf("Creating and attaching to %q…", submission.Name), statusInfo)
	m.configureTable()

	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return attachFinishedMsg{Name: submission.Name, Err: err}
	})
}

func (m model) attachSelected() (tea.Model, tea.Cmd) {
	session, ok := m.selectedSession()
	if !ok {
		m.showDialog(dialogWarning, "No session selected", "Select a session before attaching.")
		return m, nil
	}
	if m.nestedHerdrBlocked() {
		m.showDialog(dialogWarning, "Cannot attach from inside Herdr", m.nestedHerdrAttachMessage(session.Name, session.SocketPath))
		return m, nil
	}
	cmd, err := m.client.AttachCommand(session.Name)
	if err != nil {
		m.showDialog(dialogWarning, "Session cannot be attached", fmt.Sprintf("%q cannot be attached:\n\n%v", session.Name, err))
		return m, nil
	}

	m.busy = fmt.Sprintf("attach %q", session.Name)
	m.setStatus(fmt.Sprintf("Attaching to %q…", session.Name), statusInfo)

	return m, tea.ExecProcess(cmd, func(err error) tea.Msg {
		return attachFinishedMsg{Name: session.Name, Err: err}
	})
}

func (m model) nestedHerdrBlocked() bool {
	return m.insideHerdr && !m.allowNested
}

func (m model) nestedHerdrCreateMessage() string {
	return "Creating a session attaches immediately, which would launch nested Herdr. Run herdrctx outside Herdr, or use --allow-nested with Herdr experimental.allow_nested."
}

func (m model) nestedHerdrAttachMessage(target string, socketPath string) string {
	if socketPath != "" && socketPath == m.currentSocketPath {
		return fmt.Sprintf("Already inside %q; attaching here would launch nested Herdr.", target)
	}

	return fmt.Sprintf("Cannot attach %q from inside Herdr; run herdrctx outside Herdr or use --allow-nested with Herdr experimental.allow_nested.", target)
}

func (m model) confirmStop() (tea.Model, tea.Cmd) {
	session, ok := m.selectedSession()
	if !ok {
		m.showDialog(dialogWarning, "No session selected", "Select a session before stopping it.")
		return m, nil
	}
	if !session.Running {
		m.showDialog(dialogWarning, "Session already stopped", fmt.Sprintf("%q is already stopped.", session.Name))
		return m, nil
	}
	if herdr.IsJSONFlagName(session.Name) {
		m.showDialog(dialogWarning, "Session cannot be stopped", fmt.Sprintf("Herdr treats %q as a stop flag, so herdrctx cannot stop this session.", session.Name))
		return m, nil
	}

	m.confirm = &confirmation{Action: confirmStop, Session: session}
	return m, nil
}

func (m model) confirmDelete() (tea.Model, tea.Cmd) {
	session, ok := m.selectedSession()
	if !ok {
		m.showDialog(dialogWarning, "No session selected", "Select a session before deleting it.")
		return m, nil
	}
	if session.Default {
		m.showDialog(dialogWarning, "Default session cannot be deleted", "Herdr does not support deleting the default session.")
		return m, nil
	}
	if herdr.IsJSONFlagName(session.Name) {
		m.showDialog(dialogWarning, "Session cannot be deleted", fmt.Sprintf("Herdr treats %q as a delete flag, so herdrctx cannot delete this session.", session.Name))
		return m, nil
	}
	if session.Running {
		m.showDialog(dialogWarning, "Stop session before deleting", fmt.Sprintf("Stop %q before deleting it.", session.Name))
		return m, nil
	}

	m.confirm = &confirmation{Action: confirmDelete, Session: session}
	return m, nil
}

func (m *model) revalidateConfirmation(confirmed *confirmation) bool {
	session, ok := m.sessionByName(confirmed.Session.Name)
	if !ok {
		m.showDialog(dialogWarning, "Session no longer available", fmt.Sprintf("%q is no longer in the session list.", confirmed.Session.Name))
		return false
	}

	confirmed.Session = session
	switch confirmed.Action {
	case confirmStop:
		if !session.Running {
			m.showDialog(dialogWarning, "Session already stopped", fmt.Sprintf("%q is already stopped.", session.Name))
			return false
		}
	case confirmDelete:
		if session.Default {
			m.showDialog(dialogWarning, "Default session cannot be deleted", "Herdr does not support deleting the default session.")
			return false
		}
		if herdr.IsJSONFlagName(session.Name) {
			m.showDialog(dialogWarning, "Session cannot be deleted", fmt.Sprintf("Herdr treats %q as a delete flag, so herdrctx cannot delete this session.", session.Name))
			return false
		}
		if session.Running {
			m.showDialog(dialogWarning, "Stop session before deleting", fmt.Sprintf("Stop %q before deleting it.", session.Name))
			return false
		}
	}

	return true
}

func (m model) loadSessionsCmd() tea.Cmd {
	requestID := m.refreshRequestID

	return func() tea.Msg {
		sessions, err := m.client.ListSessions(m.ctx)
		return sessionsLoadedMsg{RequestID: requestID, Sessions: sessions, Err: err, RefreshedAt: time.Now()}
	}
}

func (m model) scheduleRefresh() tea.Cmd {
	return tea.Tick(m.refreshInterval, func(t time.Time) tea.Msg {
		return refreshTickMsg(t)
	})
}

func (m *model) startRefresh() tea.Cmd {
	m.loading = true
	m.showRefreshing = false
	m.refreshIndicatorID++
	m.refreshRequestID++
	id := m.refreshIndicatorID

	return tea.Tick(refreshIndicatorDelay, func(time.Time) tea.Msg {
		return refreshIndicatorMsg(id)
	})
}

func (m model) runActionCmd(confirmed confirmation) tea.Cmd {
	return func() tea.Msg {
		var err error
		ctx := m.ctx
		switch confirmed.Action {
		case confirmStop:
			err = m.client.StopSession(ctx, confirmed.Session.Name)
		case confirmDelete:
			err = m.validateCurrentDeleteTarget(ctx, confirmed.Session.Name)
			if err == nil {
				err = m.client.DeleteSession(ctx, confirmed.Session.Name)
			}
		default:
			err = fmt.Errorf("unknown action %q", confirmed.Action)
		}

		return actionFinishedMsg{Action: confirmed.Action, Name: confirmed.Session.Name, Err: err}
	}
}

func (m model) validateCurrentDeleteTarget(ctx context.Context, name string) error {
	sessions, err := m.client.ListSessions(ctx)
	if err != nil {
		return fmt.Errorf("refresh session before delete: %w", err)
	}

	session, ok := sessionByName(sessions, name)
	if !ok {
		return fmt.Errorf("%q is no longer in the session list", name)
	}
	if session.Default {
		return fmt.Errorf("default session cannot be deleted")
	}
	if herdr.IsJSONFlagName(session.Name) {
		return fmt.Errorf("session name %q cannot be deleted by Herdr", session.Name)
	}
	if session.Running {
		return fmt.Errorf("stop %q before deleting it", session.Name)
	}

	return nil
}

func (m *model) configureTable() {
	width := m.width
	if width <= 0 {
		width = 100
	}

	tableWidth := max(40, width-2)
	nameWidth := 24
	statusWidth := 9
	remaining := tableWidth - nameWidth - statusWidth - 6
	if remaining < 24 {
		nameWidth = 18
		remaining = tableWidth - nameWidth - statusWidth - 6
	}
	if remaining < 16 {
		remaining = 16
	}

	dirWidth := remaining / 2
	socketWidth := remaining - dirWidth

	m.table.SetColumns([]table.Column{
		{Title: "Name", Width: nameWidth},
		{Title: "Status", Width: statusWidth},
		{Title: "Directory", Width: dirWidth},
		{Title: "Socket", Width: socketWidth},
	})
	m.table.SetWidth(tableWidth)

	height := m.height - 9
	m.table.SetHeight(max(5, height))
	m.table.SetRows(sessionRows(m.sessions, m.table.Columns()))
}

func (m *model) setSessions(sessions []herdr.Session) {
	selectedName := ""
	oldCursor := m.table.Cursor()
	if selected, ok := m.selectedSession(); ok {
		selectedName = selected.Name
	}

	m.sessions = sessions
	m.table.SetRows(sessionRows(sessions, m.table.Columns()))

	if len(sessions) == 0 {
		m.table.SetCursor(0)
		return
	}

	cursor := oldCursor
	if selectedName != "" {
		for i, session := range sessions {
			if session.Name == selectedName {
				cursor = i
				break
			}
		}
	}
	if cursor >= len(sessions) {
		cursor = len(sessions) - 1
	}
	if cursor < 0 {
		cursor = 0
	}

	m.table.SetCursor(cursor)
}

func sessionRows(sessions []herdr.Session, columns []table.Column) []table.Row {
	rows := make([]table.Row, 0, len(sessions))
	for _, session := range sessions {
		rows = append(rows, table.Row{
			displayCell(session.DisplayName(), columnWidth(columns, 0)),
			displayCell(session.Status(), columnWidth(columns, 1)),
			displayCell(session.SessionDir, columnWidth(columns, 2)),
			displayCell(session.SocketPath, columnWidth(columns, 3)),
		})
	}

	return rows
}

func displayCell(value string, width int) string {
	return truncate(sanitizeDisplay(value), width)
}

func columnWidth(columns []table.Column, index int) int {
	if index < 0 || index >= len(columns) {
		return 10
	}

	return columns[index].Width
}

func (m model) selectedSession() (herdr.Session, bool) {
	if len(m.sessions) == 0 {
		return herdr.Session{}, false
	}

	cursor := m.table.Cursor()
	if cursor < 0 || cursor >= len(m.sessions) {
		return herdr.Session{}, false
	}

	return m.sessions[cursor], true
}

func (m model) sessionByName(name string) (herdr.Session, bool) {
	return sessionByName(m.sessions, name)
}

func sessionByName(sessions []herdr.Session, name string) (herdr.Session, bool) {
	for _, session := range sessions {
		if session.Name == name {
			return session, true
		}
	}

	return herdr.Session{}, false
}

func (m *model) setStatus(text string, kind statusKind) {
	m.status = sanitizeDisplay(text)
	m.statusKind = kind
}

func (m *model) showDialog(kind dialogKind, title string, body string) {
	m.dialog = &alertDialog{Kind: kind, Title: title, Body: body}
	switch kind {
	case dialogError:
		m.setStatus(title, statusError)
	case dialogWarning:
		m.setStatus(title, statusWarning)
	default:
		m.setStatus(title, statusInfo)
	}
}

func (m *model) closeDialog() {
	m.dialog = nil
}

func (m model) render() string {
	overlayView := m.overlayView()
	hasOverlay := overlayView != ""

	var b strings.Builder

	title := "herdrctx"
	if m.version != "" && m.version != "dev" {
		title += " " + subtleStyle.Render("v"+m.version)
	}
	b.WriteString(titleStyle.Render(title))
	b.WriteString("\n")
	b.WriteString(m.summaryLine())
	b.WriteString("\n\n")

	switch {
	case len(m.sessions) == 0 && m.loading:
		b.WriteString(m.spinner.View())
		b.WriteString(" Loading sessions…\n")
	case len(m.sessions) == 0:
		b.WriteString(warningStyle.Render("No Herdr sessions found."))
		b.WriteString("\n")
	default:
		b.WriteString(m.table.View())
		b.WriteString("\n")
	}

	if !hasOverlay {
		b.WriteString(m.statusView())
		b.WriteString("\n")

		if m.help.ShowAll {
			b.WriteString(m.help.FullHelpView(m.keys.FullHelp()))
		} else {
			b.WriteString(m.help.ShortHelpView(m.keys.ShortHelp()))
		}
	}

	view := b.String()
	if hasOverlay {
		return renderOverlay(view, overlayView, m.width, m.height)
	}

	return view
}

func (m model) overlayView() string {
	if m.dialog != nil {
		return m.dialog.render(m.width)
	}
	if m.confirm != nil {
		return m.confirmView()
	}
	if m.newSession != nil {
		return m.newSession.render(m.width)
	}

	return ""
}

func (m model) summaryLine() string {
	parts := []string{fmt.Sprintf("refresh: %s", m.refreshInterval)}
	if !m.lastRefresh.IsZero() {
		parts = append(parts, "last: "+m.lastRefresh.Format("15:04:05"))
	}
	if m.showRefreshing {
		parts = append(parts, m.spinner.View()+" refreshing")
	}
	if m.busy != "" {
		parts = append(parts, m.spinner.View()+" "+m.busy)
	}

	return subtleStyle.Render(strings.Join(parts, " · "))
}

func (m model) statusView() string {
	if m.status == "" {
		return ""
	}

	switch m.statusKind {
	case statusSuccess:
		return successStyle.Render(m.status)
	case statusWarning:
		return warningStyle.Render(m.status)
	case statusError:
		return errorStyle.Render(m.status)
	default:
		return subtleStyle.Render(m.status)
	}
}

func (m model) confirmView() string {
	if m.confirm == nil {
		return ""
	}

	content := warningStyle.Render(m.confirm.title()) + "\n\n" + m.confirm.body() + "\n\n" + subtleStyle.Render("Press y or Enter to confirm. Press n or Esc to cancel.")
	return confirmBoxStyle.Width(max(40, min(80, m.width-4))).Render(content)
}

func completionVisibleCountWithDefault(count int) int {
	if count <= 0 {
		return DefaultDirectoryCompletionVisibleCount
	}

	return count
}

func min(a, b int) int {
	if a < b {
		return a
	}

	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}

	return b
}
