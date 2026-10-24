package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/dtt101/doitdoit/styles"
	"github.com/dtt101/doitdoit/taskclient"
)

type serviceState struct {
	recoveryBlocked       bool
	discardReview         bool
	connected             bool
	client                *taskclient.Client
	origin, draftPath     string
	tasks                 map[string]taskclient.Task
	deletedView           bool
	loaded, busy          bool
	queued, pending, undo *taskclient.Command
	sequence              uint64
	cursor                int64
	selection             string
	conflict              *taskclient.APIError
	conflictOffset        int
	location              *time.Location
}
type serviceResult struct {
	polled                        bool
	cursor                        int64
	readRecovery, recoveryBlocked bool
	sequence                      uint64
	task                          *taskclient.Task
	board                         *taskclient.Board
	command                       *taskclient.Command
	err                           error
	draft                         *taskclient.Command
}

func serviceTick() tea.Cmd {
	return tea.Tick(15*time.Second, func(t time.Time) tea.Msg { return reloadTickMsg(t) })
}

func (m Model) servicePoll() tea.Cmd {
	state := *m.cloud
	read := m.serviceRead(false)
	return func() tea.Msg {
		if !state.loaded {
			return read()
		}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		tasks := maps.Clone(state.tasks)
		cursor := state.cursor
		changed := false
		for range 10000 {
			page, err := state.client.Changes(ctx, cursor)
			if err != nil {
				var api *taskclient.APIError
				if errors.As(err, &api) && api.Status == 400 && api.Code == "invalid_request" {
					return read() // A restored database may invalidate the old cursor.
				}
				return serviceResult{sequence: state.sequence, err: err}
			}
			for _, event := range page.Events {
				current, exists := tasks[event.TaskID]
				if !exists || event.After.Revision > current.Revision {
					tasks[event.TaskID] = *event.After
				}
				changed = true
			}
			cursor = page.Cursor
			if page.NextCursor == nil {
				if !changed {
					return serviceResult{sequence: state.sequence, polled: true, cursor: cursor}
				}
				board := taskclient.Board{Timezone: state.location.String(), SnapshotCursor: cursor}
				for _, task := range tasks {
					if state.deletedView || task.DeletedAt == nil {
						board.Tasks = append(board.Tasks, task)
					}
				}
				return serviceResult{sequence: state.sequence, board: &board}
			}
		}
		return serviceResult{sequence: state.sequence, err: fmt.Errorf("service change page limit exceeded")}
	}
}

// NewServiceModel uses task-oriented commands and never constructs a JSON Store.
func NewServiceModel(client *taskclient.Client, origin, draftPath string) (Model, error) {
	if client == nil || origin == "" || draftPath == "" {
		return Model{}, fmt.Errorf("service model requires a client, origin and recovery path")
	}
	m := Model{Data: TodoData{}, VisibleDays: 3, State: Browsing, TextInput: textinput.New(), cloud: &serviceState{client: client, origin: origin, draftPath: draftPath, tasks: map[string]taskclient.Task{}, busy: true}}
	m.configureTextInput("New task...")
	m.updateDateKeys()
	return m, nil
}
func (m Model) serviceRead(initial bool) tea.Cmd {
	state := *m.cloud
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		var b taskclient.Board
		var err error
		if state.deletedView {
			var e taskclient.Export
			e, err = state.client.Export(ctx)
			b = e.Board
		} else {
			b, err = state.client.Board(ctx)
		}
		result := serviceResult{sequence: state.sequence, err: err}
		if err == nil {
			result.board = &b
		}
		if initial {
			result.readRecovery = true
			draft, e := taskclient.LoadCommand(state.draftPath, state.origin)
			if e == nil {
				result.draft = &draft
			} else if !os.IsNotExist(e) {
				result.recoveryBlocked = true
				result.err = fmt.Errorf("retained draft needs review at %s: %w", state.draftPath, e)
			}
		}

		return result
	}
}
func (m Model) serviceWrite(command taskclient.Command) tea.Cmd {
	state := *m.cloud
	return func() tea.Msg {
		result := serviceResult{sequence: state.sequence, command: &command}
		data, err := command.JSON()
		if err == nil {
			err = taskclient.WritePrivate(state.draftPath, data)
		}
		if err != nil {
			result.err = fmt.Errorf("draft could not be retained; request not sent: %w", err)
			return result
		}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		task, err := state.client.Execute(ctx, command)
		if err != nil {
			result.err = err
			return result
		}
		result.task = &task
		if err := os.Remove(state.draftPath); err != nil && !os.IsNotExist(err) {
			result.err = fmt.Errorf("saved; recovery cleanup failed: %w", err)
		}
		return result
	}
}
func (m *Model) serviceData() {
	data := TodoData{}
	tasks := make([]taskclient.Task, 0, len(m.cloud.tasks))
	for _, t := range m.cloud.tasks {
		if (t.DeletedAt == nil && !m.ShowDeleted) || (t.DeletedAt != nil && m.ShowDeleted) {
			tasks = append(tasks, t)
		}
	}
	sort.SliceStable(tasks, func(i, j int) bool {
		a, b := tasks[i], tasks[j]
		aDate, bDate := "", ""
		if a.ScheduledDate != nil {
			aDate = *a.ScheduledDate
		}
		if b.ScheduledDate != nil {
			bDate = *b.ScheduledDate
		}
		if aDate != bDate {
			return aDate < bDate
		}
		if a.Status != b.Status {
			return a.Status == "open"
		}
		if a.Position == b.Position {
			return a.ID < b.ID
		}
		return a.Position < b.Position
	})
	for _, t := range tasks {
		key, date := "Future", ""
		if t.ScheduledDate != nil {
			key, date = *t.ScheduledDate, *t.ScheduledDate
		}
		if t.Status == "open" && date != "" && date < m.serviceNow().Format(dateLayout) {
			key = "Overdue"
		}
		if m.ShowDeleted {
			key = "Deleted"
		}
		data[key] = append(data[key], Task{ID: t.ID, Title: t.Title, Notes: t.Notes, Completed: t.Status == "completed", DueDate: date, CreatedAt: t.CreatedAt})
	}
	m.Data = data
	m.clampRow()
}
func (m Model) serviceNow() time.Time {
	if m.cloud != nil && m.cloud.location != nil {
		return time.Now().In(m.cloud.location)
	}
	return time.Now()
}
func (m *Model) serviceBoard(board taskclient.Board) {
	location, err := time.LoadLocation(board.Timezone)
	if err != nil {
		m.Err = fmt.Errorf("unknown owner timezone %q", board.Timezone)
		return
	}
	m.cloud.location = location
	m.cloud.cursor = board.SnapshotCursor
	if !m.cloud.loaded {
		m.updateDateKeysFrom(startOfDay(m.serviceNow()))
		m.todayKey = m.serviceNow().Format(dateLayout)
	}
	m.cloud.tasks = map[string]taskclient.Task{}
	for _, t := range board.Tasks {
		m.cloud.tasks[t.ID] = t
	}
	m.cloud.loaded = true
	m.cloud.connected = true
	m.serviceData()
}
func (m *Model) queueService(command taskclient.Command) {
	command.Origin = m.cloud.origin
	m.cloud.sequence++
	m.cloud.pending = &command
	m.cloud.queued = &command
	m.cloud.busy = true
	m.cloud.conflict = nil
	m.Err = nil
	m.saveErr = nil
	m.feedback = "Saving to service…"
}
func (m *Model) persistService() {
	if m.cloud.queued != nil {
		return
	}
	if !m.cloud.loaded || m.cloud.pending != nil {
		m.Err = fmt.Errorf("wait for the service before editing")
		return
	}
	after := map[string]Task{}
	keys := map[string]string{}
	for key, tasks := range m.Data {
		for _, task := range tasks {
			after[task.ID] = task
			keys[task.ID] = key
		}
	}
	for id, t := range after {
		old, exists := m.cloud.tasks[id]
		if !exists {
			var date *string
			if key := keys[id]; key != "Future" {
				if key == "Overdue" {
					key = m.serviceNow().Format(dateLayout)
				}
				date = &key
			}
			m.queueService(taskclient.Command{Action: "create", Create: &taskclient.CreateRequest{OperationID: taskclient.OperationID(), Title: t.Title, Notes: t.Notes, ScheduledDate: date}})
			return
		}
		changes := taskclient.Changes{}
		changed := false
		if t.Title != old.Title {
			v := t.Title
			changes.Title = &v
			changed = true
		}
		if t.Notes != old.Notes {
			v := t.Notes
			changes.Notes = &v
			changed = true
		}
		status := "open"
		if t.Completed {
			status = "completed"
		}
		if status != old.Status {
			changes.Status = &status
			changed = true
		}
		oldDate := ""
		if old.ScheduledDate != nil {
			oldDate = *old.ScheduledDate
		}
		if t.DueDate != oldDate {
			var date *string
			if t.DueDate != "" {
				v := t.DueDate
				date = &v
			}
			changes.ScheduledDate = &date
			changed = true
		}
		if changed {
			m.queueService(taskclient.Command{Action: "update", ID: id, Before: &old, Update: &taskclient.UpdateRequest{OperationID: taskclient.OperationID(), ExpectedRevision: old.Revision, Changes: changes}})
			return
		}
	}
	for id, old := range m.cloud.tasks {
		if old.DeletedAt != nil {
			continue
		}
		if _, exists := after[id]; !exists {
			m.queueService(taskclient.Command{Action: "delete", ID: id, Before: &old, Mutation: &taskclient.Mutation{OperationID: taskclient.OperationID(), ExpectedRevision: old.Revision}})
			return
		}
	}
	// Reorder is the only operation that changes display order without fields.
	id := m.cloud.selection
	if old, ok := m.cloud.tasks[id]; ok {
		selected := after[id]
		group := []string{}
		for _, t := range m.Data[keys[id]] {
			if t.Completed == selected.Completed && t.DueDate == selected.DueDate {
				group = append(group, t.ID)
			}
		}
		for i, target := range group {
			if target == id {
				var before, afterID *string
				if i > 0 {
					v := group[i-1]
					afterID = &v
				}
				if i+1 < len(group) {
					v := group[i+1]
					before = &v
				}
				prior := []taskclient.Task{}
				for _, t := range m.cloud.tasks {
					if t.DeletedAt == nil && t.Status == old.Status && reflectDate(t.ScheduledDate) == reflectDate(old.ScheduledDate) {
						prior = append(prior, t)
					}
				}
				sort.Slice(prior, func(i, j int) bool {
					if prior[i].Position == prior[j].Position {
						return prior[i].ID < prior[j].ID
					}
					return prior[i].Position < prior[j].Position
				})
				var priorAfter, priorBefore *string
				for j, t := range prior {
					if t.ID == id {
						if j > 0 {
							v := prior[j-1].ID
							priorAfter = &v
						}
						if j+1 < len(prior) {
							v := prior[j+1].ID
							priorBefore = &v
						}
					}
				}
				m.queueService(taskclient.Command{Action: "reorder", ID: id, Before: &old, PreviousAfter: priorAfter, PreviousBefore: priorBefore, Reorder: &taskclient.ReorderRequest{Mutation: taskclient.Mutation{OperationID: taskclient.OperationID(), ExpectedRevision: old.Revision}, AfterID: afterID, BeforeID: before}})
				return
			}
		}
	}
}
func (m *Model) serviceUndo(command taskclient.Command, task taskclient.Task) {
	mutation := taskclient.Mutation{OperationID: taskclient.OperationID(), ExpectedRevision: task.Revision}
	switch command.Action {
	case "create":
		m.cloud.undo = &taskclient.Command{Action: "delete", ID: task.ID, Before: &task, Mutation: &mutation}
	case "delete":
		m.cloud.undo = &taskclient.Command{Action: "restore", ID: task.ID, Before: &task, Mutation: &mutation}
	case "restore":
		m.cloud.undo = &taskclient.Command{Action: "delete", ID: task.ID, Before: &task, Mutation: &mutation}
	case "reorder":
		m.cloud.undo = &taskclient.Command{Action: "reorder", ID: task.ID, Before: &task, PreviousAfter: command.Reorder.AfterID, PreviousBefore: command.Reorder.BeforeID, Reorder: &taskclient.ReorderRequest{Mutation: mutation, AfterID: command.PreviousAfter, BeforeID: command.PreviousBefore}}
	case "update":
		if command.Before != nil {
			before := command.Before
			changes := taskclient.Changes{}
			original := command.Update.Changes
			if original.Title != nil {
				v := before.Title
				changes.Title = &v
			}
			if original.Notes != nil {
				v := before.Notes
				changes.Notes = &v
			}
			if original.Status != nil {
				v := before.Status
				changes.Status = &v
			}
			if original.ScheduledDate != nil {
				v := before.ScheduledDate
				changes.ScheduledDate = &v
			}
			m.cloud.undo = &taskclient.Command{Action: "update", ID: task.ID, Before: &task, Update: &taskclient.UpdateRequest{OperationID: mutation.OperationID, ExpectedRevision: mutation.ExpectedRevision, Changes: changes}}
		}
	default:
		m.cloud.undo = nil
	}
}
func (m Model) updateService(msg tea.Msg) (tea.Model, tea.Cmd) {
	copy := *m.cloud
	copy.tasks = maps.Clone(m.cloud.tasks)
	m.cloud = &copy
	switch value := msg.(type) {
	case serviceResult:
		if value.sequence != m.cloud.sequence {
			return m, nil
		}
		m.cloud.busy = false
		if value.polled {
			m.cloud.connected = true
			if m.State == Browsing {
				m.cloud.cursor = value.cursor
			}
		}
		if value.readRecovery {
			m.cloud.recoveryBlocked = value.recoveryBlocked
		}
		if value.command != nil {
			if value.task != nil {
				m.cloud.connected = true
				m.cloud.tasks[value.task.ID] = *value.task
				m.serviceUndo(*value.command, *value.task)
				m.cloud.pending = nil
				m.saveErr = nil
				m.feedback = "Saved to service"
				m.serviceData()
			}
			if value.err != nil {
				m.Err = value.err
				if value.task == nil {
					m.saveErr = value.err
					var conflict *taskclient.APIError
					if errors.As(value.err, &conflict) && conflict.Status == 409 {
						m.cloud.conflict = conflict
						m.cloud.conflictOffset = 0
					}
				}
			}
		} else if value.err != nil {
			m.cloud.connected = false
			m.Err = value.err
		}
		if value.board != nil && m.cloud.pending == nil && m.State == Browsing {
			m.serviceBoard(*value.board)
		}
		if value.draft != nil {
			m.cloud.pending = value.draft
			m.Err = fmt.Errorf("retained service command found; r retries the original operation")
			m.saveErr = m.Err
		}
		if value.task != nil {
			return m, m.servicePoll()
		}
		return m, nil
	case reloadTickMsg:
		if !m.cloud.busy && m.cloud.pending == nil && m.State == Browsing {
			m.cloud.sequence++
			m.cloud.busy = true
			return m, tea.Batch(serviceTick(), m.servicePoll())
		}
		return m, serviceTick()
	case dateTickMsg:
		if m.State != Browsing || m.cloud.pending != nil {
			return m, dateTick()
		}
		now := m.serviceNow()
		if m.todayKey == now.Format(dateLayout) {
			return m, dateTick()
		}
		m.todayKey = now.Format(dateLayout)
		m.serviceData()
		first := m.firstVisibleDate()
		if m.FocusToday || first.Before(startOfDay(now)) {
			m.updateDateKeysFrom(startOfDay(now))
		}
		return m, dateTick()
	case tea.KeyPressMsg:
		key := value.String()
		if (key == "q" || key == "ctrl+c") && m.State == Browsing && !m.ShowHelp && m.cloud.pending == nil {
			return m, tea.Quit
		}
		if m.cloud.conflict != nil {
			switch key {
			case "j", "down":
				m.cloud.conflictOffset++
			case "k", "up":
				m.cloud.conflictOffset = max(0, m.cloud.conflictOffset-1)
			case "a":
				current := m.cloud.conflict.Current
				if m.canApplyServiceConflict() {
					command := *m.cloud.pending
					update := *command.Update
					update.OperationID = taskclient.OperationID()
					update.ExpectedRevision = current.Revision
					command.Update = &update
					command.Before = current
					m.queueService(command)
				}
			case "d":
				return m.archiveServiceDraft()
			case "q", "ctrl+c":
				return m, tea.Quit
			}
		} else if m.cloud.pending != nil {
			switch key {
			case "j", "down":
				if m.cloud.discardReview {
					m.cloud.conflictOffset++
				}
			case "k", "up":
				if m.cloud.discardReview {
					m.cloud.conflictOffset = max(0, m.cloud.conflictOffset-1)
				}
			case "esc":
				m.cloud.discardReview = false
			case "d":
				if !m.cloud.busy {
					if m.cloud.discardReview {
						return m.archiveServiceDraft()
					}
					m.cloud.discardReview = true
					m.cloud.conflictOffset = 0
				}
			case "r":
				if !m.cloud.busy {
					command := *m.cloud.pending
					m.cloud.discardReview = false
					m.queueService(command)
				}
			case "q", "ctrl+c":
				data, err := m.cloud.pending.JSON()
				if err == nil {
					err = taskclient.WritePrivate(m.cloud.draftPath, data)
				}
				if err != nil {
					m.Err = err
					return m, nil
				}
				return m, tea.Quit
			}
		} else if m.ShowHelp || (m.State == Browsing && key == "?") {
			return m.updateShared(msg)
		} else if m.State == Browsing && key == "x" && !m.cloud.busy {
			m.ShowDeleted = !m.ShowDeleted
			m.cloud.deletedView = m.ShowDeleted
			m.ShowOverdue = false
			m.ShowFuture = false
			m.cloud.sequence++
			m.cloud.busy = true
			m.serviceData()
			return m, m.serviceRead(false)
		} else if m.ShowDeleted && m.State == Browsing && !m.cloud.busy && key != "r" {
			switch key {
			case "enter":
				if m.cloud.connected && !m.cloud.recoveryBlocked && m.hasSelectedTask() {
					id := m.Data["Deleted"][m.RowIdx].ID
					current := m.cloud.tasks[id]
					m.queueService(taskclient.Command{Action: "restore", ID: id, Before: &current, Mutation: &taskclient.Mutation{OperationID: taskclient.OperationID(), ExpectedRevision: current.Revision}})
				}
			case "j", "down":
				m.moveSelection(1)
			case "k", "up":
				m.moveSelection(-1)
			case "c":
				m.HideCompleted = !m.HideCompleted
				m.clampRow()
			}
		} else if m.State == Browsing && key == "r" {
			if m.cloud.busy {
				return m, nil
			}
			m.cloud.sequence++
			m.cloud.busy = true
			return m, m.serviceRead(m.cloud.recoveryBlocked)
		} else if m.State == Browsing && key == "o" {
			m.ShowOverdue = !m.ShowOverdue
			m.ShowFuture = false
			m.clampRow()
			return m, nil
		} else if m.State == Browsing && key == "u" {
			if m.cloud.loaded && !m.cloud.busy && m.cloud.connected && !m.cloud.recoveryBlocked && m.cloud.undo != nil {
				command := *m.cloud.undo
				m.cloud.undo = nil
				m.queueService(command)
			}
		} else if !m.cloud.loaded || m.cloud.busy || (!m.cloud.connected && m.State == Browsing) || m.cloud.recoveryBlocked {
			return m, nil
		} else {
			if key == "f" {
				m.ShowOverdue = false
			}
			if m.hasSelectedTask() {
				m.cloud.selection = m.Data[m.getCurrentKey()][m.RowIdx].ID
			}
			updated, cmd := m.updateShared(msg)
			m = updated.(Model)
			if m.cloud.queued != nil {
				command := *m.cloud.queued
				m.cloud.queued = nil
				return m, tea.Batch(cmd, m.serviceWrite(command))
			}
			return m, cmd
		}
	default:
		updated, cmd := m.updateShared(msg)
		return updated, cmd
	}
	if m.cloud.queued != nil {
		command := *m.cloud.queued
		m.cloud.queued = nil
		return m, m.serviceWrite(command)
	}
	return m, nil
}
func (m Model) canApplyServiceConflict() bool {
	return m.cloud.conflict != nil && m.cloud.conflict.Current != nil && m.cloud.conflict.Current.DeletedAt == nil && m.cloud.pending != nil && m.cloud.pending.Action == "update" && m.cloud.pending.Update != nil
}

func (m Model) archiveServiceDraft() (tea.Model, tea.Cmd) {
	data, err := m.cloud.pending.JSON()
	if err == nil {
		err = taskclient.WritePrivateExclusive(m.cloud.draftPath+".archived-"+taskclient.OperationID(), data)
	}
	if err == nil {
		err = os.Remove(m.cloud.draftPath)
		if os.IsNotExist(err) {
			err = nil
		}
	}
	if err != nil {
		m.Err = fmt.Errorf("draft could not be archived; original command retained: %w", err)
		return m, nil
	}
	m.cloud.pending, m.cloud.conflict = nil, nil
	m.cloud.discardReview = false
	m.saveErr, m.Err = nil, nil
	m.State = Browsing
	m.cloud.sequence++
	m.cloud.busy = true
	return m, m.serviceRead(false)
}

func (m Model) serviceConflictView() string {
	value := map[string]any{"attempt": m.cloud.pending}
	if m.cloud.conflict != nil {
		value["current"], value["error"] = m.cloud.conflict.Current, m.cloud.conflict.Code
	}
	data, _ := json.MarshalIndent(value, "", "  ")
	width := max(1, m.width-2)
	header := lipgloss.Wrap("Service conflict — review current values and retained command", width, "")
	controls := "↑/↓ scroll · d archive draft · q quit"
	if m.canApplyServiceConflict() {
		controls = "a apply UPDATE against reviewed revision · " + controls
	} else if m.cloud.conflict != nil && m.cloud.conflict.Current != nil && m.cloud.conflict.Current.DeletedAt != nil {
		controls = "Task is deleted; archive the edit and restore explicitly from Deleted. " + controls
	}
	if m.cloud.discardReview {
		header = lipgloss.Wrap("Review retained command. Its outcome may be unknown. Archiving does not cancel a server write. Do not recreate an uncertain task before checking the service.", width, "")
		controls = "↑/↓ scroll · d confirm archive and refresh · esc return · r retry original · q quit"
	}
	footer := lipgloss.Wrap(controls, width, "")
	lines := strings.Split(lipgloss.Wrap(plainNotes(string(data)), width, ""), "\n")
	height := max(1, m.height-lipgloss.Height(header)-lipgloss.Height(footer)-2)
	offset := min(m.cloud.conflictOffset, max(0, len(lines)-height))
	return header + "\n" + strings.Join(lines[offset:min(len(lines), offset+height)], "\n") + "\n" + footer
}
func (m Model) serviceStatus() string {
	if m.cloud.recoveryBlocked {
		return "Service · retained draft needs manual review; editing paused · r recheck"
	}
	if m.cloud.pending != nil {
		if m.cloud.busy {
			return "Service · saving…"
		}
		return "Service · not saved; private draft retained · r retry · d review/archive"
	}
	if !m.cloud.loaded {
		return "Service · connecting…"
	}
	if m.cloud.busy {
		return "Service · refreshing…"
	}
	if !m.cloud.connected {
		return "Service · disconnected; editing paused · r reconnect"
	}
	if m.ShowDeleted {
		return "Service · deleted tasks · enter restore · x return"
	}
	return "Service · saved · o overdue · x deleted · r refresh"
}

func (m Model) serviceStatusView() string {
	message := m.serviceStatus()
	width := m.footerContentWidth()
	if width <= 0 {
		width = 80
	}
	return lipgloss.NewStyle().Foreground(styles.Subtle).
		MarginLeft(m.footerStyle().GetMarginLeft()).Render(lipgloss.Wrap(message, width, ""))
}

func reflectDate(date *string) string {
	if date == nil {
		return ""
	}
	return *date
}
