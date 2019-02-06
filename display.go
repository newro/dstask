package dstask

import (
	"fmt"
	"golang.org/x/sys/unix"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	TABLE_MAX_WIDTH      = 160 // keep it readable
	MODE_HEADER          = 4
	FG_DEFAULT           = 250
	BG_DEFAULT_1         = 233
	BG_DEFAULT_2         = 232
	MODE_DEFAULT         = 0
	FG_ACTIVE            = 255
	BG_ACTIVE            = 166
	BG_PAUSED            = 235 // task that has been started then stopped
	FG_PRIORITY_CRITICAL = 160
	FG_PRIORITY_HIGH     = 166
	FG_PRIORITY_NORMAL   = FG_DEFAULT
	FG_PRIORITY_LOW      = 245
)

type RowStyle struct {
	// ansi mode
	Mode int
	// xterm 256-colour palette
	Fg int
	Bg int
}

// should use a better console library after first POC

/// display list of filtered tasks with context and filter
func (ts *TaskSet) Display() {
	if ts.numTasksLoaded == 0 {
		fmt.Println("\033[31mNo tasks found. Showing help.\033[0m")
		Help("")
	} else if len(ts.tasks) == 0 {
		ExitFail("No matching tasks in given context or filter.")
	} else if len(ts.tasks) == 1 {
		DisplayTask(ts.tasks[0])
		return
	} else {
		DisplayTasks(ts.tasks)
	}
}

// display a single task in detail, with numbered subtasks
func (t *Task) Display() {

}

type Table struct {
	Header       []string
	Rows         [][]string
	MaxColWidths []int
	TermWidth    int
	TermHeight   int
	RowStyles    []RowStyle
}

// header may  havetruncated words
func NewTable(header ...string) *Table {
	ws, err := unix.IoctlGetWinsize(int(os.Stdout.Fd()), unix.TIOCGWINSZ)
	if err != nil {
		ExitFail("Not a TTY")
	}

	return &Table{
		Header:       header,
		MaxColWidths: make([]int, len(header)),
		TermWidth:    int(ws.Col),
		TermHeight:   int(ws.Row),
		RowStyles: []RowStyle{
			RowStyle{
				Mode: MODE_HEADER,
			},
		},
	}
}

func (t *Table) AddRow(row []string, style RowStyle) {
	if len(row) != len(t.Header) {
		panic("Row is incorrect length")
	}

	for i, cell := range row {
		if t.MaxColWidths[i] < len(cell) {
			t.MaxColWidths[i] = len(cell)
		}
	}

	t.Rows = append(t.Rows, row)
	t.RowStyles = append(t.RowStyles, style)
}

// get widths appropriate to the terminal size and TABLE_MAX_WIDTH
// cells may require padding or truncation. Cell padding of 1char between
// fields recommended -- not included.
// A nice characteristic of this, is that if there are no populated cells the
// column will disappear.
func (t *Table) calcColWidths(gap int) []int {
	target := TABLE_MAX_WIDTH

	if t.TermWidth < target {
		target = t.TermWidth
	}

	colWidths := t.MaxColWidths[:]

	// account for gaps
	target -= gap*len(colWidths) - 1

	for SumInts(colWidths...) > target {
		// find max col width index
		var max, maxi int

		for i, w := range colWidths {
			if w > max {
				max = w
				maxi = i
			}
		}

		// decrement, if 0 abort
		if colWidths[maxi] == 0 {
			break
		}
		colWidths[maxi] = colWidths[maxi] - 1
	}

	return colWidths
}

// theme loosely based on https://github.com/GothenburgBitFactory/taskwarrior/blob/2.6.0/doc/rc/dark-256.theme
// render table, returning count of rows rendered
func (t *Table) Render(gap int) int {
	widths := t.calcColWidths(2)
	maxRows := t.TermHeight - gap
	rows := append([][]string{t.Header}, t.Rows...)

	for i, row := range rows {
		cells := row[:]
		for i, w := range widths {
			cells[i] = FixStr(cells[i], w)
		}

		line := strings.Join(cells, "  ")

		mode := t.RowStyles[i].Mode
		fg := t.RowStyles[i].Fg
		bg := t.RowStyles[i].Bg

		// defaults
		if mode == 0 {
			mode = MODE_DEFAULT
		}

		if fg == 0 {
			fg = FG_DEFAULT
		}

		if bg == 0 {
			/// alternate if not specified
			if i%2 != 0 {
				bg = BG_DEFAULT_1
			} else {
				bg = BG_DEFAULT_2
			}
		}

		// print style, line then reset
		fmt.Printf("\033[%d;38;5;%d;48;5;%dm%s\033[0m\n", mode, fg, bg, line)

		if i > maxRows {
			return i
		}
	}

	return len(t.Rows)
}

func DisplayTasks(tasks []*Task) {
	table := NewTable(
		"ID",
		"Priority",
		"Tags",
		"Project",
		"Summary",
	)

	now := time.Now()

	for _, t := range tasks {
		style := RowStyle{}

		if t.Status == STATUS_ACTIVE {
			style.Fg = FG_ACTIVE
			style.Bg = BG_ACTIVE
		} else if !t.Due.IsZero() && t.Due.Before(now) {
			style.Fg = FG_PRIORITY_HIGH
		} else if t.Priority == PRIORITY_CRITICAL {
			style.Fg = FG_PRIORITY_CRITICAL
		} else if t.Priority == PRIORITY_HIGH {
			style.Fg = FG_PRIORITY_HIGH
		} else if t.Priority == PRIORITY_LOW {
			style.Fg = FG_PRIORITY_LOW
		}

		if t.Status == STATUS_PAUSED {
			style.Bg = BG_PAUSED
		}

		table.AddRow(
			[]string{
				// id should be at least 2 chars wide to match column header
				// (headers can be truncated)
				fmt.Sprintf("%-2d", t.ID),
				t.Priority,
				strings.Join(t.Tags, " "),
				t.Project,
				t.Summary,
			},
			style,
		)
	}

	rowsRendered := table.Render(11)

	if rowsRendered == len(tasks) {
		fmt.Printf("\n%v tasks.\n", len(tasks))
	} else {
		fmt.Printf("\n%v tasks, truncated to %v lines.\n", len(tasks), rowsRendered)
	}
}

func DisplayTask(task *Task) {
	table := NewTable(
		"Name",
		"Value",
	)

	table.AddRow([]string{"ID", strconv.Itoa(task.ID)}, RowStyle{})
	table.AddRow([]string{"Priority", task.Priority}, RowStyle{})
	table.AddRow([]string{"Summary", task.Summary}, RowStyle{})
	table.AddRow([]string{"Notes", task.Notes}, RowStyle{})
	table.AddRow([]string{"Status", task.Status}, RowStyle{})
	table.AddRow([]string{"Project", task.Project}, RowStyle{})
	table.AddRow([]string{"Tags", strings.Join(task.Tags, ", ")}, RowStyle{})
	table.AddRow([]string{"UUID", task.UUID}, RowStyle{})
	table.AddRow([]string{"Created", task.Created.String()}, RowStyle{})
	if !task.Resolved.IsZero() {
		table.AddRow([]string{"Resolved", task.Resolved.String()}, RowStyle{})
	}
	if !task.Due.IsZero() {
		table.AddRow([]string{"Due", task.Due.String()}, RowStyle{})
	}
	table.Render(0)
}
