package view

import (
	"context"

	"github.com/derailed/k9s/internal/ui"
	"github.com/gdamore/tcell"
	"github.com/rs/zerolog/log"
)

type Table struct {
	*ui.Table

	app      *App
	filterFn func(string)
}

func NewTable(title string) *Table {
	return &Table{
		Table: ui.NewTable(title),
	}
}

func (t *Table) Init(ctx context.Context) {
	t.app = mustExtractApp(ctx)
	ctx = context.WithValue(ctx, ui.KeyStyles, t.app.Styles)
	t.Table.Init(ctx)

	t.SearchBuff().AddListener(t.app.Cmd())
	t.SearchBuff().AddListener(t)
	t.bindKeys()
}

func (t *Table) Start()       {}
func (t *Table) Stop()        {}
func (t *Table) Name() string { return t.BaseTitle }

// BufferChanged indicates the buffer was changed.
func (t *Table) BufferChanged(s string) {}

// BufferActive indicates the buff activity changed.
func (t *Table) BufferActive(state bool, k ui.BufferKind) {
	t.app.BufferActive(state, k)
}

func (t *Table) saveCmd(evt *tcell.EventKey) *tcell.EventKey {
	if path, err := saveTable(t.app.Config.K9s.CurrentCluster, t.BaseTitle, t.GetFilteredData()); err != nil {
		t.app.Flash().Err(err)
	} else {
		t.app.Flash().Infof("File %s saved successfully!", path)
	}

	return nil
}

func (t *Table) setFilterFn(fn func(string)) {
	t.filterFn = fn

	cmd := t.SearchBuff().String()
	if ui.IsLabelSelector(cmd) && t.filterFn != nil {
		t.filterFn(ui.TrimLabelSelector(cmd))
	}
}

func (t *Table) bindKeys() {
	t.AddActions(ui.KeyActions{
		ui.KeySpace:         ui.NewKeyAction("Mark", t.markCmd, true),
		tcell.KeyCtrlSpace:  ui.NewKeyAction("Marks Clear", t.clearMarksCmd, true),
		tcell.KeyCtrlS:      ui.NewKeyAction("Save", t.saveCmd, true),
		ui.KeySlash:         ui.NewKeyAction("Filter Mode", t.activateCmd, false),
		tcell.KeyEscape:     ui.NewKeyAction("Filter Reset", t.resetCmd, false),
		tcell.KeyEnter:      ui.NewKeyAction("Filter", t.filterCmd, false),
		tcell.KeyBackspace2: ui.NewKeyAction("Erase", t.eraseCmd, false),
		tcell.KeyBackspace:  ui.NewKeyAction("Erase", t.eraseCmd, false),
		tcell.KeyDelete:     ui.NewKeyAction("Erase", t.eraseCmd, false),
		ui.KeyShiftI:        ui.NewKeyAction("Invert", t.SortInvertCmd, false),
		ui.KeyShiftN:        ui.NewKeyAction("Sort Name", t.SortColCmd(0), false),
		ui.KeyShiftA:        ui.NewKeyAction("Sort Age", t.SortColCmd(-1), false),
	})
}

func (t *Table) markCmd(evt *tcell.EventKey) *tcell.EventKey {
	if !t.RowSelected() {
		return evt
	}
	t.ToggleMark()
	t.Refresh()

	return nil
}

func (t *Table) clearMarksCmd(evt *tcell.EventKey) *tcell.EventKey {
	if !t.RowSelected() {
		return evt
	}
	t.ClearMarks()

	return nil
}

func (t *Table) filterCmd(evt *tcell.EventKey) *tcell.EventKey {
	if !t.SearchBuff().IsActive() {
		return evt
	}

	t.SearchBuff().SetActive(false)
	cmd := t.SearchBuff().String()
	if ui.IsLabelSelector(cmd) && t.filterFn != nil {
		t.filterFn(ui.TrimLabelSelector(cmd))
		return nil
	}
	t.Refresh()

	return nil
}

func (t *Table) eraseCmd(evt *tcell.EventKey) *tcell.EventKey {
	if t.SearchBuff().IsActive() {
		t.SearchBuff().Delete()
	}

	return nil
}

func (t *Table) resetCmd(evt *tcell.EventKey) *tcell.EventKey {
	log.Debug().Msgf("Table filter reset!")
	if t.SearchBuff().Empty() {
		return evt
	}

	if ui.IsLabelSelector(t.SearchBuff().String()) {
		t.filterFn("")
	}
	t.app.Flash().Info("Clearing filter...")
	t.SearchBuff().Reset()
	t.Refresh()

	return nil
}

func (t *Table) activateCmd(evt *tcell.EventKey) *tcell.EventKey {
	log.Debug().Msgf("Table filter activated!")
	if t.app.InCmdMode() {
		return evt
	}
	t.app.Flash().Info("Filter mode activated.")
	t.SearchBuff().SetActive(true)

	return nil
}