package ui

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"github.com/j0urneyk/herdrctx/internal/herdr"
	"github.com/j0urneyk/herdrctx/internal/preferences"
)

const (
	allHosts       = "*"
	maxHostQueries = 4
)

type hostState struct {
	record     preferences.Host
	client     *herdr.Client
	generation uint64
	request    uint64
	sessions   []herdr.Session
	last       time.Time
	err        error
	stale      bool
	loading    bool
	queued     bool
}
type hostCollection struct {
	entries        map[string]*hostState
	order          []string
	active         string
	nextGeneration uint64
	cursor         int
}
type hostLoadedMsg struct {
	Target              string
	Generation, Request uint64
	Sessions            []herdr.Session
	Err                 error
	CheckID             uint64
}
type hostsSavedMsg struct {
	Data preferences.Hosts
	Err  error
}
type hostMenu struct {
	cursor   int
	create   bool
	mode     newSessionMode
	form     bool
	editing  string
	fields   []textinput.Model
	field    int
	err      string
	removing bool
	saving   bool
}

func (m *model) initHosts(all bool) {
	f := &hostCollection{entries: map[string]*hostState{}, active: m.client.TargetID()}
	m.hosts = f
	m.addHost(preferences.Host{Label: "Local"}, m.clientForLocal())
	if m.hostError == nil {
		for _, h := range m.hostData.Items {
			m.addHost(h, nil)
		}
	}
	if _, ok := f.entries[m.client.TargetID()]; !ok {
		m.addHost(preferences.Host{Label: m.client.TargetID(), Destination: m.client.TargetID()}, m.client)
	}
	initial := f.entries[m.client.TargetID()]
	initial.sessions = m.sessions
	initial.last = m.lastRefresh
	initial.stale = m.remoteStale
	if all {
		f.active = allHosts
	}
	m.loading = false
}
func (m model) clientForLocal() *herdr.Client { c := *m.client; c.Remote = nil; return &c }
func (m *model) addHost(h preferences.Host, c *herdr.Client) {
	if _, ok := m.hosts.entries[h.Destination]; ok {
		return
	}
	if c == nil {
		value := *m.client
		value.Remote = &herdr.RemoteTarget{Destination: h.Destination}
		c = &value
	}
	m.hosts.nextGeneration++
	m.hosts.entries[h.Destination] = &hostState{record: h, client: c, generation: m.hosts.nextGeneration, stale: true}
	m.hosts.order = append(m.hosts.order, h.Destination)
}

func (m model) clientFor(target string) *herdr.Client {
	if m.hosts != nil {
		if h := m.hosts.entries[target]; h != nil {
			return h.client
		}
	}
	return m.client
}

func (m model) activeTarget() string {
	if m.hosts != nil {
		return m.hosts.active
	}
	return m.client.TargetID()
}

func (m model) hostVisible(target string) bool {
	return m.hosts == nil || m.hosts.active == allHosts || m.hosts.active == target
}

func (m model) needsSessionCheck(s herdr.Session) bool {
	if m.hosts != nil {
		h := m.hosts.entries[s.Target]
		return h == nil || h.stale || h.loading || h.queued
	}
	return m.needsRemoteCheck()
}

func (m *model) queueHostRefresh() tea.Cmd {
	for _, id := range m.hosts.order {
		if m.hostVisible(id) {
			m.hosts.entries[id].queued = true
		}
	}
	return m.pumpHostQueries()
}

func (m *model) pumpHostQueries() tea.Cmd {
	active := 0
	for _, h := range m.hosts.entries {
		if h.loading {
			active++
		}
	}
	var cmds []tea.Cmd
	// Rotating the starting host prevents a slow or frequently refreshed host starving peers.
	count := len(m.hosts.order)
	start := m.hosts.cursor
	for i := 0; i < count && active < maxHostQueries; i++ {
		n := (start + i) % count
		id := m.hosts.order[n]
		h := m.hosts.entries[id]
		pending := m.remotePending
		check := pending != nil && pending.Session.Target == id && pending.Waiting
		if h.loading || !check && (!h.queued || m.busy != "") {
			continue
		}
		ctx := m.ctx
		checkID := uint64(0)
		if check {
			ctx = pending.Context
			checkID = pending.ID
			pending.Waiting = false
			h.stale = true
		}
		h.loading = true
		h.queued = false
		h.request++
		active++
		m.hosts.cursor = (n + 1) % count
		client, generation, request := h.client, h.generation, h.request
		cmds = append(cmds, func() tea.Msg {
			if check {
				defer pending.Cancel()
			}
			var sessions []herdr.Session
			var err error
			if client.Remote != nil {
				err = client.EnsureMinimumVersion(ctx)
			}
			if err == nil {
				sessions, err = client.ListSessions(ctx)
			}
			return hostLoadedMsg{Target: id, Generation: generation, Request: request, Sessions: sessions, Err: err, CheckID: checkID}
		})
	}
	return tea.Batch(cmds...)
}

func (m *model) mergeHostSessions() {
	var sessions []herdr.Session
	for _, id := range m.hosts.order {
		h := m.hosts.entries[id]
		sessions = append(sessions, h.sessions...)
	}
	m.setSessions(sessions)
	m.updateDetails()
}

func (m model) handleHostLoaded(msg hostLoadedMsg) (tea.Model, tea.Cmd) {
	h := m.hosts.entries[msg.Target]
	if h == nil || h.generation != msg.Generation || h.request != msg.Request {
		return m, nil
	}
	// Release the query slot on command exit, even when its result was cancelled.
	h.loading = false
	pending := m.remotePending
	if pending != nil && pending.Session.Target == msg.Target && pending.Waiting {
		h.stale = true
		return m, m.pumpHostQueries()
	}
	if msg.CheckID != 0 && (pending == nil || pending.ID != msg.CheckID) {
		return m, m.pumpHostQueries()
	}
	h.err = msg.Err
	if msg.Err != nil {
		h.stale = true
	} else {
		h.sessions = msg.Sessions
		h.stale = false
		h.last = time.Now()
	}
	m.mergeHostSessions()
	if msg.CheckID != 0 {
		p := *pending
		m.remotePending = nil
		m.busy = ""
		if msg.Err != nil {
			m.showDialog(dialogError, "Remote check failed", fmt.Sprintf("No action was run on %s.\n\n%v", hostName(msg.Target), msg.Err))
			return m, m.pumpHostQueries()
		}
		s, ok := m.sessionByID(p.Session)
		if !ok {
			m.showDialog(dialogWarning, "Session no longer available", "The selected session is no longer listed on this host.")
			return m, m.pumpHostQueries()
		}
		if p.Action == "attach" {
			return m.attachSession(s)
		}
		confirmation := confirmation{Action: confirmAction(p.Action), Session: s}
		if !m.revalidateConfirmation(&confirmation) {
			return m, m.pumpHostQueries()
		}
		if p.Confirmed {
			return m.executeConfirmation(confirmation)
		}
		m.confirm = &confirmation
	} else if m.hostVisible(msg.Target) {
		if msg.Err != nil {
			m.setStatus(fmt.Sprintf("Refresh failed on %s: %v", hostName(msg.Target), msg.Err), statusError)
		} else {
			m.setStatus(fmt.Sprintf("Loaded %d session(s) on %s.", len(msg.Sessions), hostName(msg.Target)), statusSuccess)
		}
	}
	return m, m.pumpHostQueries()
}

func (m model) checkHostSession(s herdr.Session, action string, confirmed bool) (tea.Model, tea.Cmd) {
	m.remoteRequestID++
	ctx, cancel := context.WithCancel(m.ctx)
	m.remotePending = &remotePreflight{ID: m.remoteRequestID, Session: s.ID(), Action: action, Confirmed: confirmed, Waiting: true, Context: ctx, Cancel: cancel}
	m.busy = "checking remote session"
	m.setStatus("Checking remote session… Esc cancels.", statusInfo)
	return m, m.pumpHostQueries()
}

func hostName(id string) string {
	if id == allHosts {
		return "All hosts"
	}
	if id == "" {
		return "Local"
	}
	return id
}

func (m model) hostSummary() string {
	var parts []string
	for _, id := range m.hosts.order {
		if !m.hostVisible(id) {
			continue
		}
		h := m.hosts.entries[id]
		status := fmt.Sprintf("%d sessions", len(h.sessions))
		if h.last.IsZero() {
			status = "not loaded"
		} else {
			status += " @ " + h.last.Format("15:04:05")
		}
		if h.stale {
			status += " · last known values"
		}
		if h.loading {
			status += " · loading"
		}
		if h.err != nil {
			status += " · failed"
		}
		parts = append(parts, hostName(id)+": "+status)
	}
	summary := strings.Join(parts, " | ")
	for _, id := range m.hosts.order {
		if m.hostVisible(id) && m.hosts.entries[id].stale {
			return "Partial results · " + summary
		}
	}
	return summary
}

func (m model) openHosts(create bool, mode newSessionMode) (tea.Model, tea.Cmd) {
	if m.loading {
		m.showDialog(dialogWarning, "Host switch unavailable", "Wait for the current query to finish.")
		return m, nil
	}
	if m.hosts == nil {
		m.initHosts(false)
	}
	m.hostMenu = &hostMenu{create: create, mode: mode}
	if m.hostError != nil {
		m.hostMenu.err = "Host settings unavailable: " + m.hostError.Error()
	}
	return m, nil
}

func (m model) hostMenuView() string {
	menu := m.hostMenu
	title := "Hosts"
	if menu.create {
		title = "Choose a host for the new session"
	}
	var b strings.Builder
	b.WriteString(title + "\n\n")
	if menu.form {
		b.WriteString("Label\n" + menu.fields[0].View() + "\nSSH destination\n" + menu.fields[1].View() + "\nTab switches field · Enter saves · Esc cancels")
	} else {
		first, last := menuRange(len(m.hosts.order), menu.cursor, m.height)
		fmt.Fprintf(&b, "Showing %d–%d of %d\n", first+1, last, len(m.hosts.order))
		for i := first; i < last; i++ {
			id := m.hosts.order[i]
			h := m.hosts.entries[id]
			prefix := "  "
			if i == menu.cursor {
				prefix = "> "
			}
			state := "not loaded"
			if !h.last.IsZero() {
				state = h.last.Format("15:04:05")
			}
			if h.loading {
				state += " loading"
			}
			if h.err != nil {
				state += " failed"
			}
			fmt.Fprintf(&b, "%s%s · %s · %s\n", prefix, sanitizeDisplay(h.record.Label), sanitizeDisplay(hostName(id)), state)
			if h.record.Session != "" {
				fmt.Fprintf(&b, "    Profile session: %s\n", h.record.Session)
			}
		}
		switch {
		case menu.removing:
			b.WriteString("\nRemove this saved host? Sessions and favorites remain. y confirms · Esc cancels")
		case menu.create:
			b.WriteString("\nEnter selects · Esc cancels")
		default:
			b.WriteString("\nEnter selects · A shows all hosts · n adds · e edits · d removes · i imports machines · Esc closes")
		}
	}
	if menu.saving {
		b.WriteString("\nSaving host settings…")
	}
	if menu.err != "" {
		b.WriteString("\n" + menu.err)
	}
	return dialogBoxStyle.Width(navigationWidth(m.width)).Render(b.String())
}

func (m model) editHost(edit bool) (tea.Model, tea.Cmd) {
	menu := m.hostMenu
	if m.hostStore == nil || m.hostError != nil {
		menu.err = fmt.Sprintf("Host settings unavailable: %v", m.hostError)
		return m, nil
	}
	h := m.hosts.entries[m.hosts.order[menu.cursor]]
	if edit && (h.record.Destination == "" || h.loading) {
		menu.err = "Local or busy hosts cannot be edited."
		return m, nil
	}
	menu.form = true
	menu.field = 0
	menu.editing = ""
	menu.fields = []textinput.Model{textinput.New(), textinput.New()}
	for i := range menu.fields {
		menu.fields[i].CharLimit = 1024
		menu.fields[i].SetWidth(max(10, navigationWidth(m.width)-6))
	}
	if edit {
		menu.editing = h.record.Destination
		menu.fields[0].SetValue(h.record.Label)
		menu.fields[1].SetValue(h.record.Destination)
	}
	return m, menu.fields[0].Focus()
}

func (m model) saveHost(change preferences.HostChange) (tea.Model, tea.Cmd) {
	if m.hostStore == nil || m.hostError != nil {
		m.hostMenu.err = fmt.Sprintf("Host settings unavailable: %v", m.hostError)
		return m, nil
	}
	for _, h := range m.hosts.entries {
		if h.loading {
			m.hostMenu.err = "Wait for host queries to finish before changing settings."
			return m, nil
		}
	}
	m.hostMenu.saving = true
	return m, func() tea.Msg { d, err := m.hostStore.Apply(change); return hostsSavedMsg{Data: d, Err: err} }
}

func (m model) handleHostsSaved(msg hostsSavedMsg) (tea.Model, tea.Cmd) {
	menu := m.hostMenu
	menu.saving = false
	if msg.Err != nil {
		menu.err = msg.Err.Error()
		return m, nil
	}
	m.hostData = msg.Data
	// Rebuild metadata while retaining clients and live query state for unchanged identities.
	// Local and the launch target remain available independently of saved metadata.
	keep := map[string]bool{"": true, m.client.TargetID(): true}
	for _, h := range msg.Data.Items {
		keep[h.Destination] = true
		if old := m.hosts.entries[h.Destination]; old != nil {
			old.record = h
		} else {
			m.addHost(h, nil)
		}
	}
	var order []string
	for _, id := range m.hosts.order {
		if keep[id] {
			order = append(order, id)
		} else {
			delete(m.hosts.entries, id)
		}
	}
	m.hosts.order = order
	if m.hosts.active != allHosts && !keep[m.hosts.active] {
		m.hosts.active = ""
	}
	menu.form = false
	menu.removing = false
	menu.err = ""
	menu.cursor = min(menu.cursor, len(order)-1)
	m.mergeHostSessions()
	return m, m.queueHostRefresh()
}

func handleHostsInput(m model, msg tea.KeyPressMsg) inputLayerResult {
	menu := m.hostMenu
	if menu.saving {
		return inputConsumed(m, nil)
	}
	if menu.form {
		switch msg.String() {
		case "esc":
			menu.form = false
			menu.err = ""
			return inputConsumed(m, nil)
		case "tab":
			menu.fields[menu.field].Blur()
			menu.field = 1 - menu.field
			return inputConsumed(m, menu.fields[menu.field].Focus())
		case "enter":
			h := preferences.Host{Label: menu.fields[0].Value(), Destination: menu.fields[1].Value()}
			if old := m.hosts.entries[menu.editing]; menu.editing != "" && old != nil && h.Destination == menu.editing {
				h.ProfileID = old.record.ProfileID
				h.Session = old.record.Session
			}
			next, cmd := m.saveHost(preferences.HostChange{Previous: menu.editing, Host: h})
			return inputConsumed(next.(model), cmd)
		}
		var cmd tea.Cmd
		menu.fields[menu.field], cmd = menu.fields[menu.field].Update(msg)
		return inputConsumed(m, cmd)
	}
	if menu.removing {
		if msg.String() == "esc" || msg.String() == "n" {
			menu.removing = false
		}
		if msg.String() == "y" {
			next, cmd := m.saveHost(preferences.HostChange{Previous: m.hosts.order[menu.cursor], Remove: true})
			return inputConsumed(next.(model), cmd)
		}
		return inputConsumed(m, nil)
	}
	switch msg.String() {
	case "esc", "q":
		m.hostMenu = nil
	case "up", "k":
		menu.cursor = max(0, menu.cursor-1)
	case "down", "j":
		menu.cursor = min(len(m.hosts.order)-1, menu.cursor+1)
	case "enter", "A":
		if menu.create && msg.String() == "A" {
			return inputConsumed(m, nil)
		}
		id := m.hosts.order[menu.cursor]
		if msg.String() == "A" {
			id = allHosts
		}
		m.hostMenu = nil
		if menu.create {
			next, cmd := m.openNewSessionOn(menu.mode, id)
			return inputConsumed(next.(model), cmd)
		}
		m.hosts.active = id
		m.configureTable()
		m.setStatus("Selected "+hostName(id), statusInfo)
		return inputConsumed(m, m.queueHostRefresh())
	case "n":
		if !menu.create {
			next, cmd := m.editHost(false)
			return inputConsumed(next.(model), cmd)
		}
	case "e":
		if !menu.create {
			next, cmd := m.editHost(true)
			return inputConsumed(next.(model), cmd)
		}
	case "d":
		if !menu.create && m.hosts.order[menu.cursor] != "" {
			menu.removing = true
		}
	case "i":
		if !menu.create {
			return inputConsumed(m, m.importMachinesCmd())
		}
	}
	return inputConsumed(m, nil)
}

// Check the local launch binary even when the host has never completed a list query.
type hostLaunchMsg struct {
	Command      *exec.Cmd
	Target, Name string
	Err          error
}

func (m model) prepareHostLaunch(command *exec.Cmd, target, name string) tea.Cmd {
	client := m.clientFor(target)
	return func() tea.Msg {
		return hostLaunchMsg{Command: command, Target: target, Name: name, Err: client.EnsureMinimumVersion(m.ctx)}
	}
}

func (m model) handleHostLaunch(msg hostLaunchMsg) (tea.Model, tea.Cmd) {
	if msg.Err != nil {
		m.busy = ""
		m.showDialog(dialogError, "Attach failed", msg.Err.Error())
		return m, m.queueHostRefresh()
	}
	return m, tea.ExecProcess(msg.Command, func(err error) tea.Msg { return attachFinishedMsg{Target: msg.Target, Name: msg.Name, Err: err} })
}

func menuRange(count, cursor, height int) (int, int) {
	if height <= 0 {
		height = 24
	}
	visible := max(1, min(8, (height-10)/2))
	start := max(0, min(cursor-visible+1, count-visible))
	return start, min(count, start+visible)
}

func (m model) hostQuerySlotsFull() bool {
	if m.hosts == nil {
		return false
	}
	count := 0
	for _, h := range m.hosts.entries {
		if h.loading {
			count++
		}
	}
	return count >= maxHostQueries
}
