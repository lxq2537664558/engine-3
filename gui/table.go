// Copyright 2016 The G3N Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package gui

import (
	"fmt"
	"math"

	"github.com/g3n/engine/math32"
	"github.com/g3n/engine/window"
)

const (
	OnTableClick = "onTableClick"
)

//
// Table implements a panel which can contains child panels
// organized in rows and columns.
//
type Table struct {
	Panel                                // Embedded panel
	styles       *TableStyles            // pointer to current styles
	cols         []*TableColumn          // array of columns descriptors
	colmap       map[string]*TableColumn // maps column id to column descriptor
	firstRow     int                     // index of the first visible row
	lastRow      int                     // index of the last visible row
	rows         []*tableRow             // array of table rows
	headerHeight float32                 // header height
	vscroll      *ScrollBar              // vertical scroll bar
	showHeader   bool                    // header visibility flag
}

// TableColumn describes a table column
type TableColumn struct {
	Id        string  // Column id used to reference the column. Must be unique
	Name      string  // Column name shown in the header
	Width     float32 // Column preferable width in pixels
	Hidden    bool    // Hidden flag
	Format    string  // Format string for numbers and strings
	Alignment Align   // Cell content alignment: AlignNone|AlignLeft|AlignCenter|AlignRight
	Expand    int     // Width expansion factor
	order     int     // show order
	header    *Panel  // header panel
	label     *Label  // header label
}

// TableHeaderStyle describes the style of the table header
type TableHeaderStyle struct {
	Border      BorderSizes
	Paddings    BorderSizes
	BorderColor math32.Color4
	BgColor     math32.Color
	FgColor     math32.Color
}

// TableRowStyle describes the style of the table row
type TableRowStyle struct {
	Border      BorderSizes
	Paddings    BorderSizes
	BorderColor math32.Color4
	BgColor     math32.Color
	FgColor     math32.Color
}

// TableRowStyles describes all styles for the table row
type TableRowStyles struct {
	Normal   TableRowStyle
	Selected TableRowStyle
}

// TableStyles describes all styles of the table header and rows
type TableStyles struct {
	Header *TableHeaderStyle
	Row    *TableRowStyles
}

// TableClickEvent describes a mouse click event over a table
// It contains the original mouse event plus additional information
type TableClickEvent struct {
	window.MouseEvent         // Embedded window mouse event
	X                 float32 // Table content area X coordinate
	Y                 float32 // Table content area Y coordinate
	Header            bool    // True if header was clicked
	Row               int     // Index of table row (may be -1)
	Col               string  // Id of table column (may be empty)
}

// tableRow is panel which contains an entire table row of cells
type tableRow struct {
	Panel                 // embedded panel
	selected bool         // row selected flag
	cells    []*tableCell // array of row cells
}

// tableCell is a panel which contains one cell (a label)
type tableCell struct {
	Panel             // embedded panel
	label Label       // cell label
	value interface{} // cell current value
}

// NewTable creates and returns a pointer to a new Table with the
// specified width, height and columns
func NewTable(width, height float32, cols []TableColumn) (*Table, error) {

	t := new(Table)
	t.Panel.Initialize(width, height)
	t.styles = &StyleDefault.Table
	t.showHeader = true

	// Checks columns descriptors
	t.colmap = make(map[string]*TableColumn)
	t.cols = make([]*TableColumn, 0)
	for i := 0; i < len(cols); i++ {
		// Make a copy of the column descriptor argument and saves its pointer
		c := cols[i]
		t.cols = append(t.cols, &c)
		// Column id must not be empty
		if c.Id == "" {
			return nil, fmt.Errorf("Column with empty id")
		}
		// Column id must be unique
		if t.colmap[c.Id] != nil {
			return nil, fmt.Errorf("Column with duplicate id")
		}
		// Sets default format and order
		if c.Format == "" {
			c.Format = "%v"
		}
		c.order = i
		t.colmap[c.Id] = &c
	}

	// Create header panels
	for i := 0; i < len(t.cols); i++ {
		c := t.cols[i]
		c.header = NewPanel(0, 0)
		t.applyHeaderStyle(c.header)
		c.label = NewLabel(c.Name)
		c.header.Add(c.label)
		width := c.Width
		if width < c.label.Width()+c.header.MinWidth() {
			width = c.label.Width() + c.header.MinWidth()
		}
		c.header.SetContentSize(width, c.label.Height())
		t.headerHeight = c.header.Height()
		t.Panel.Add(c.header)
	}
	t.recalcHeader()

	// Subscribe to events
	t.Panel.Subscribe(OnMouseUp, t.onMouse)
	t.Panel.Subscribe(OnMouseDown, t.onMouse)
	t.Panel.Subscribe(OnKeyDown, t.onKeyEvent)
	t.Panel.Subscribe(OnKeyRepeat, t.onKeyEvent)
	t.Panel.Subscribe(OnResize, func(evname string, ev interface{}) {
		t.recalc()
	})
	return t, nil
}

// ShowHeader shows or hides the table header
func (t *Table) ShowHeader(show bool) {

	if t.showHeader == show {
		return
	}
	t.showHeader = show
	for i := 0; i < len(t.cols); i++ {
		c := t.cols[i]
		c.header.SetVisible(t.showHeader)
	}
	t.recalc()
}

// ShowColumn sets the visibility of the column with the specified id
// If the column id does not exit the function panics.
func (t *Table) ShowColumn(col string, show bool) {

	c := t.colmap[col]
	if c == nil {
		panic("Invalid column id")
	}
	if c.Hidden == !show {
		return
	}
	c.Hidden = !show
	t.recalcHeader()
	// Recalculates all rows
	for ri := 0; ri < len(t.rows); ri++ {
		trow := t.rows[ri]
		t.recalcRow(trow)
	}
	t.recalc()
}

// ShowAllColumns shows all the table columns
func (t *Table) ShowAllColumns() {

	recalc := false
	for ci := 0; ci < len(t.cols); ci++ {
		c := t.cols[ci]
		if c.Hidden {
			c.Hidden = false
			recalc = true
		}
	}
	if !recalc {
		return
	}
	t.recalcHeader()
	// Recalculates all rows
	for ri := 0; ri < len(t.rows); ri++ {
		trow := t.rows[ri]
		t.recalcRow(trow)
	}
	t.recalc()
}

// RowCount returns the current number of rows in the table
func (t *Table) RowCount() int {

	return len(t.rows)
}

// SetRows clears all current rows of the table and
// sets new rows from the specifying parameter.
// Each row is a map keyed by the colum id.
// The map value currently can be a string or any number type
// If a row column is not found it is ignored
func (t *Table) SetRows(values []map[string]interface{}) {

	// Add missing rows
	if len(values) > len(t.rows) {
		count := len(values) - len(t.rows)
		for row := 0; row < count; row++ {
			t.insertRow(len(t.rows), nil)
		}
		// Remove remaining rows
	} else if len(values) < len(t.rows) {
		for row := len(values); row < len(t.rows); row++ {
			t.removeRow(row)
		}
	}

	// Set rows values
	for row := 0; row < len(values); row++ {
		t.SetRow(row, values[row])
	}
	t.firstRow = 0
	t.recalc()
}

// SetRow sets the value of all the cells of the specified row from
// the specified map indexed by column id.
func (t *Table) SetRow(row int, values map[string]interface{}) {

	if row < 0 || row >= len(t.rows) {
		panic("Invalid row index")
	}
	for ci := 0; ci < len(t.cols); ci++ {
		c := t.cols[ci]
		cv := values[c.Id]
		if cv == nil {
			continue
		}
		t.SetCell(row, c.Id, values[c.Id])
	}
	t.recalcRow(t.rows[row])
}

// SetCell sets the value of the cell specified by its row and column id
func (t *Table) SetCell(row int, colid string, value interface{}) {

	if row < 0 || row >= len(t.rows) {
		panic("Invalid row index")
	}
	c := t.colmap[colid]
	if c == nil {
		return
	}
	cell := t.rows[row].cells[c.order]
	cell.label.SetText(fmt.Sprintf(c.Format, value))
}

// SetColFormat sets the formatting string (Printf) for the specified column
// Update must be called to update the table.
func (t *Table) SetColFormat(id, format string) error {

	c := t.colmap[id]
	if c == nil {
		return fmt.Errorf("No column with id:%s", id)
	}
	c.Format = format
	return nil
}

// AddRow adds a new row at the end of the table with the specified values
func (t *Table) AddRow(values map[string]interface{}) {

	t.InsertRow(len(t.rows), values)
}

// InsertRow inserts the specified values in a new row at the specified index
func (t *Table) InsertRow(row int, values map[string]interface{}) {

	t.insertRow(row, values)
	t.recalc()
}

// RemoveRow removes from the specified row from the table
func (t *Table) RemoveRow(row int) {

	// Checks row index
	if row < 0 || row >= len(t.rows) {
		panic("Invalid row index")
	}
	t.removeRow(row)
	t.recalc()
}

// SelectedRow returns the index of the currently selected row
// or -1 if no row selected
func (t *Table) SelectedRow() int {

	for ri := 0; ri < len(t.rows); ri++ {
		if t.rows[ri].selected {
			return ri
		}
	}
	return -1
}

// insertRow is the internal version of InsertRow which does not call recalc()
func (t *Table) insertRow(row int, values map[string]interface{}) {

	// Checks row index
	if row < 0 || row > len(t.rows) {
		panic("Invalid row index")
	}

	// Creates tableRow panel
	trow := new(tableRow)
	trow.Initialize(0, 0)
	trow.cells = make([]*tableCell, 0)
	for ci := 0; ci < len(t.cols); ci++ {
		// Creates tableRow cell panel
		cell := new(tableCell)
		cell.Initialize(0, 0)
		cell.label.initialize("", StyleDefault.Font)
		cell.Add(&cell.label)
		trow.cells = append(trow.cells, cell)
		trow.Panel.Add(cell)
	}
	t.Panel.Add(trow)

	// Inserts tableRow in the table rows at the specified index
	t.rows = append(t.rows, nil)
	copy(t.rows[row+1:], t.rows[row:])
	t.rows[row] = trow
	t.updateRowStyle(row)

	// Sets the new row values from the specified map
	if values != nil {
		t.SetRow(row, values)
	}
	t.recalcRow(trow)
}

// ScrollDown scrolls the table the specified number of rows down if possible
func (t *Table) scrollDown(n int, selFirst bool) {

	// Calculates number of rows to scroll down
	maxFirst := t.calcMaxFirst()
	maxScroll := maxFirst - t.firstRow
	if maxScroll <= 0 {
		return
	}
	if n > maxScroll {
		n = maxScroll
	}

	t.firstRow += n
	// Update scroll bar if visible
	if t.vscroll != nil && t.vscroll.Visible() {
		t.vscroll.SetValue(float32(t.firstRow) / float32(maxFirst))
	}
	if selFirst {
		t.selectRow(t.firstRow)
	}
	t.recalc()
	return
}

// ScrollUp scrolls the table the specified number of rows up if possible
func (t *Table) scrollUp(n int, selLast bool) {

	// Calculates number of rows to scroll up
	if t.firstRow == 0 {
		return
	}
	if n > t.firstRow {
		n = t.firstRow
	}
	t.firstRow -= n
	// Update scroll bar if visible
	if t.vscroll != nil && t.vscroll.Visible() {
		t.vscroll.SetValue(float32(t.firstRow) / float32(t.calcMaxFirst()))
	}
	if selLast {
		t.selectRow(t.lastRow - n)
	}
	t.recalc()
}

// removeRow removes from the table the row specified its index
func (t *Table) removeRow(row int) {

	// Get row to be removed
	trow := t.rows[row]

	// Remove row from table
	copy(t.rows[row:], t.rows[row+1:])
	t.rows[len(t.rows)-1] = nil
	t.rows = t.rows[:len(t.rows)-1]

	// Dispose the row cell panels and its children
	for i := 0; i < len(trow.cells); i++ {
		cell := trow.cells[i]
		cell.DisposeChildren(true)
		cell.Dispose()
	}

	// Adjusts table first visible row if necessary
	//if t.firstRow == row {
	//	t.firstRow--
	//	if t.firstRow < 0 {
	//		t.firstRow = 0
	//	}
	//}
}

// onMouseEvent process subscribed mouse events
func (t *Table) onMouse(evname string, ev interface{}) {

	e := ev.(*window.MouseEvent)
	t.root.SetKeyFocus(t)
	switch evname {
	case OnMouseDown:
		// Creates and dispatch TableClickEvent
		var tce TableClickEvent
		tce.MouseEvent = *e
		t.findClick(&tce)
		t.Dispatch(OnTableClick, tce)
		// Select left clicked row
		if tce.Button == window.MouseButtonLeft && tce.Row >= 0 {
			t.selectRow(tce.Row)
			t.recalc()
		}
	case OnMouseUp:
	default:
		return
	}
	t.root.StopPropagation(StopAll)
}

// onKeyEvent receives subscribed key events for the list
func (t *Table) onKeyEvent(evname string, ev interface{}) {

	kev := ev.(*window.KeyEvent)
	switch kev.Keycode {
	case window.KeyUp:
		t.selPrev()
	case window.KeyDown:
		t.selNext()
	case window.KeyPageUp:
		t.prevPage()
	case window.KeyPageDown:
		t.nextPage()
	}
}

// findClick finds where in the table the specified mouse click event
// occurred updating the specified TableClickEvent with the click coordinates.
func (t *Table) findClick(ev *TableClickEvent) {

	x, y := t.ContentCoords(ev.Xpos, ev.Ypos)
	ev.X = x
	ev.Y = y
	ev.Row = -1
	// Find column id
	colx := float32(0)
	for ci := 0; ci < len(t.cols); ci++ {
		c := t.cols[ci]
		if c.Hidden {
			continue
		}
		colx += c.header.Width()
		if x < colx {
			ev.Col = c.Id
			break
		}
	}
	// If column not found the user clicked at the right of rows
	if ev.Col == "" {
		return
	}
	// Checks if is in header
	if t.showHeader && y < t.headerHeight {
		ev.Header = true
	}

	// Find row clicked
	rowy := float32(0)
	if t.showHeader {
		rowy = t.headerHeight
	}
	theight := t.ContentHeight()
	for ri := t.firstRow; ri < len(t.rows); ri++ {
		trow := t.rows[ri]
		rowy += trow.height
		if rowy > theight {
			break
		}
		if y < rowy {
			ev.Row = ri
			break
		}
	}
}

// selNext selects the next row if possible
func (t *Table) selNext() {

	// If selected row is last, nothing to do
	sel := t.SelectedRow()
	if sel == len(t.rows)-1 {
		return
	}
	// If no selected row, selects first visible row
	if sel < 0 {
		t.selectRow(t.firstRow)
		t.recalc()
		return
	}
	// Selects next row
	next := sel + 1
	t.selectRow(next)

	// Scroll down if necessary
	if next > t.lastRow {
		t.scrollDown(1, false)
	} else {
		t.recalc()
	}
}

// selPrev selects the previous row if possible
func (t *Table) selPrev() {

	// If selected row is first, nothing to do
	sel := t.SelectedRow()
	if sel == 0 {
		return
	}
	// If no selected row, selects last visible row
	if sel < 0 {
		t.selectRow(t.lastRow)
		t.recalc()
		return
	}
	// Selects previous row and selects previous
	prev := sel - 1
	t.selectRow(prev)

	// Scroll up if necessary
	if prev < t.firstRow && t.firstRow > 0 {
		t.scrollUp(1, false)
	} else {
		t.recalc()
	}
}

// nextPage increments the first visible row to show next page of rows
func (t *Table) nextPage() {

	if t.lastRow == len(t.rows)-1 {
		return
	}
	plen := t.lastRow - t.firstRow
	if plen <= 0 {
		return
	}
	t.scrollDown(plen, true)
}

// prevPage advances the first visible row
func (t *Table) prevPage() {

	if t.firstRow == 0 {
		return
	}
	plen := t.lastRow - t.firstRow
	if plen <= 0 {
		return
	}
	t.scrollUp(plen, true)
}

// selectRow sets the specified row as selected and unselects all other rows
func (t *Table) selectRow(ri int) {

	for i := 0; i < len(t.rows); i++ {
		trow := t.rows[i]
		if i == ri {
			trow.selected = true
		} else {
			trow.selected = false
		}
	}
}

// recalcHeader recalculates and sets the position and size of the header panels
func (t *Table) recalcHeader() {

	posx := float32(0)
	for i := 0; i < len(t.cols); i++ {
		c := t.cols[i]
		if c.Hidden {
			c.header.SetVisible(false)
			continue
		}
		c.header.SetPosition(posx, 0)
		c.header.SetVisible(true)
		posx += c.header.Width()
	}
}

// recalc calculates the visibility, positions and sizes of all row cells.
// should be called in the following situations:
// - the table is resized
// - row is added, inserted or removed
// - column alignment and expansion changed
// - column visibility is changed
// - horizontal or vertical scroll position changed
func (t *Table) recalc() {

	// Get initial Y coordinate and total height of the table for rows
	starty := t.headerHeight
	if !t.showHeader {
		starty = 0
	}
	theight := t.ContentHeight()

	// Determines if it is necessary to show the scrollbar or not.
	scroll := false
	py := starty
	for ri := 0; ri < len(t.rows); ri++ {
		trow := t.rows[ri]
		py += trow.height
		if py > theight {
			scroll = true
			break
		}
	}
	t.setVScrollBar(scroll)

	// Sets the position and sizes of all cells of the visible rows
	py = starty
	for ri := 0; ri < len(t.rows); ri++ {
		trow := t.rows[ri]
		// If row is before first row or its y coordinate is greater the table height,
		// sets it invisible
		if ri < t.firstRow || py > theight {
			trow.SetVisible(false)
			continue
		}
		// Set row y position and visible
		trow.SetPosition(0, py)
		trow.SetVisible(true)
		t.updateRowStyle(ri)
		// Set the last completely visible row index
		if py+trow.Height() <= theight {
			t.lastRow = ri
		}
		//log.Error("ri:%v py:%v theight:%v", ri, py, theight)
		py += trow.height
	}
}

// recalcRow recalculates the positions and sizes of all cells of the specified row
// Should be called when the row is created and column visibility or order is changed.
func (t *Table) recalcRow(trow *tableRow) {

	// Calculates and sets row height
	maxheight := float32(0)
	for ci := 0; ci < len(t.cols); ci++ {
		// If column is hidden, ignore
		c := t.cols[ci]
		if c.Hidden {
			continue
		}
		cell := trow.cells[c.order]
		cellHeight := cell.MinHeight() + cell.label.Height()
		if cellHeight > maxheight {
			maxheight = cellHeight
		}
	}
	trow.SetContentHeight(maxheight)

	// Sets row cells sizes and positions and sets row width
	px := float32(0)
	for ci := 0; ci < len(t.cols); ci++ {
		// If column is hidden, ignore
		c := t.cols[ci]
		cell := trow.cells[c.order]
		if c.Hidden {
			cell.SetVisible(false)
			continue
		}
		// Sets cell position and size
		cell.SetPosition(px, 0)
		cell.SetVisible(true)
		cell.SetSize(c.header.Width(), trow.ContentHeight())
		px += c.header.Width()
	}
	trow.SetContentWidth(px)
}

func (t *Table) sortCols() {

}

// setVScrollBar sets the visibility state of the vertical scrollbar
func (t *Table) setVScrollBar(state bool) {

	// Visible
	if state {
		var scrollWidth float32 = 20
		// Creates scroll bar if necessary
		if t.vscroll == nil {
			t.vscroll = NewVScrollBar(0, 0)
			t.vscroll.SetBorders(0, 0, 0, 1)
			t.vscroll.Subscribe(OnChange, t.onVScrollBarEvent)
			t.Panel.Add(t.vscroll)
		}
		// Initial y coordinate and height
		py := float32(0)
		height := t.ContentHeight()
		if t.showHeader {
			py = t.headerHeight
			height -= py
		}
		t.vscroll.SetSize(scrollWidth, height)
		t.vscroll.SetPositionX(t.ContentWidth() - scrollWidth)
		t.vscroll.SetPositionY(py)
		t.vscroll.recalc()
		t.vscroll.SetVisible(true)
		// Not visible
	} else {
		if t.vscroll != nil {
			t.vscroll.SetVisible(false)
		}
	}
}

// onVScrollBarEvent is called when a vertical scroll bar event is received
func (t *Table) onVScrollBarEvent(evname string, ev interface{}) {

	pos := t.vscroll.Value()
	maxFirst := t.calcMaxFirst()
	first := int(math.Floor((float64(maxFirst) * pos) + 0.5))
	if first == t.firstRow {
		return
	}
	t.firstRow = first
	t.recalc()
}

// calcMaxFirst calculates the maximum index of the first visible row
// such as the remaing rows fits completely inside the table
// It is used when scrolling the table vertically
func (t *Table) calcMaxFirst() int {

	// Get table height for rows considering if header is shown or not
	total := t.ContentHeight()
	if t.showHeader {
		total -= t.headerHeight
	}

	ri := len(t.rows) - 1
	if ri < 0 {
		return 0
	}
	height := float32(0)
	for {
		trow := t.rows[ri]
		height += trow.height
		if height > total {
			break
		}
		ri--
		if ri < 0 {
			break
		}
	}
	return ri + 1
}

// updateRowStyle applies the correct style for the specified row
func (t *Table) updateRowStyle(ri int) {

	row := t.rows[ri]
	if row.selected {
		t.applyRowStyle(row, &t.styles.Row.Selected)
		return
	}
	t.applyRowStyle(row, &t.styles.Row.Normal)
}

// applyRowStyle applies the specified style to all cells for the specified table row
func (t *Table) applyRowStyle(row *tableRow, trs *TableRowStyle) {

	for i := 0; i < len(row.cells); i++ {
		cell := row.cells[i]
		cell.SetBordersFrom(&trs.Border)
		cell.SetBordersColor4(&trs.BorderColor)
		cell.SetPaddingsFrom(&trs.Paddings)
		cell.SetColor(&trs.BgColor)
	}
}

// applyStyle applies the specified menu body style
func (t *Table) applyHeaderStyle(hp *Panel) {

	s := t.styles.Header
	hp.SetBordersFrom(&s.Border)
	hp.SetBordersColor4(&s.BorderColor)
	hp.SetPaddingsFrom(&s.Paddings)
	hp.SetColor(&s.BgColor)
}
