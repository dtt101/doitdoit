package taskstore

// Insert preserves the legacy insertion rule: before the completed block.
func (d Data) Insert(key string, task Task) {
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
	} else {
		d[key] = InsertAt(tasks, index, task)
	}
}

// These operations take validated indices from the caller. Presentation concerns
// (selection, collapsed sections, feedback, and undo navigation) stay in the UI.
func (d Data) Delete(key string, index int) {
	tasks := d[key]
	d[key] = append(tasks[:index], tasks[index+1:]...)
}

func (d Data) Toggle(key string, index int) {
	task := d[key][index]
	task.Completed = !task.Completed
	d.Delete(key, index)
	d[key] = GroupTasksByCompletion(d[key])
	if task.Completed {
		d[key] = append(d[key], task)
	} else {
		d.Insert(key, task)
	}
}

func (d Data) Edit(key string, index int, title string) { d[key][index].Title = title }

func (d Data) Reorder(key string, from, to int) {
	d[key][from], d[key][to] = d[key][to], d[key][from]
}

func (d Data) Move(source string, index int, target, dueDate string) {
	task := d[source][index]
	task.DueDate = dueDate
	if source == target {
		d[source][index] = task
		return
	}
	d.Delete(source, index)
	d.Insert(target, task)
}
