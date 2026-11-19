package model

import (
	"time"
)

const dateLayout = "2006-01-02"

type Task struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Notes     string    `json:"notes,omitempty"`
	Completed bool      `json:"completed"`
	CreatedAt time.Time `json:"created_at"`
	DueDate   string    `json:"due_date,omitempty"`
}

// TodoData maps a date string ("YYYY-MM-DD", "Future", "Overdue", "Deleted") to tasks.
type TodoData map[string][]Task

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func parseDate(s string) (time.Time, error) {
	return time.ParseInLocation(dateLayout, s, time.Local)
}

func cloneTodoData(data TodoData) TodoData {
	if data == nil {
		return nil
	}
	copied := make(TodoData, len(data))
	for k, list := range data {
		copied[k] = append([]Task(nil), list...)
	}
	return copied
}

func groupTasksByCompletion(tasks []Task) []Task {
	grouped := make([]Task, 0, len(tasks))
	for _, task := range tasks {
		if !task.Completed {
			grouped = append(grouped, task)
		}
	}
	for _, task := range tasks {
		if task.Completed {
			grouped = append(grouped, task)
		}
	}
	return grouped
}

func (d TodoData) insert(key string, task Task) {
	tasks := d[key]
	index := len(tasks)
	for i, existing := range tasks {
		if existing.Completed {
			index = i
			break
		}
	}
	if index == len(tasks) {
		d[key] = append(tasks, task)
		return
	}
	updated := make([]Task, 0, len(tasks)+1)
	updated = append(updated, tasks[:index]...)
	updated = append(updated, task)
	updated = append(updated, tasks[index:]...)
	d[key] = updated
}

func (d TodoData) remove(key string, index int) {
	tasks := d[key]
	d[key] = append(tasks[:index], tasks[index+1:]...)
}

func (d TodoData) toggle(key string, index int) {
	task := d[key][index]
	task.Completed = !task.Completed
	d.remove(key, index)
	d[key] = groupTasksByCompletion(d[key])
	if task.Completed {
		d[key] = append(d[key], task)
	} else {
		d.insert(key, task)
	}
}

func (d TodoData) edit(key string, index int, title string) {
	d[key][index].Title = title
}

func (d TodoData) reorder(key string, from, to int) {
	d[key][from], d[key][to] = d[key][to], d[key][from]
}

func (d TodoData) move(source string, index int, target, dueDate string) {
	task := d[source][index]
	task.DueDate = dueDate
	if source == target {
		d[source][index] = task
		return
	}
	d.remove(source, index)
	d.insert(target, task)
}
