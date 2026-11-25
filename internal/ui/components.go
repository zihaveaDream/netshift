package ui

import (
	"github.com/egoist/mygo/ui"
)

// listRow is one row of an editable list of the advanced options: the fields
// on the given columns, then the move and delete buttons, all on one line.
func listRow(c *ui.Context, key string, index, total int, cells func(), onUp, onDown, onDelete func()) {
	ui.Row(c.Key(key)).Height(rowH).Gap(rowGap).AlignItems(ui.Center).Children(func() {
		cells()
		ui.Spacer(c).Grow(1)
		iconBtn := func(sym string, disabled bool, onClick func()) {
			fg := colText
			bg := colSurface
			bdr := colBorder
			if disabled {
				fg = colBorder
				bg = colSurfaceSubtle
				bdr = colBorderSoft
			}
			b := ui.ButtonBase(c).Size(28, rowH-4).Background(bg).Border(1, bdr).Radius(ctrlRadius).
				AlignItems(ui.Center).Justify(ui.Center).Disabled(disabled)
			if !disabled && b.Hovered() {
				b = b.Background(colSurfaceHover).Border(1, colAccent)
			}
			b.OnClick(onClick).Children(func() {
				ui.Text(c, sym).FontSize(hintSize).TextColor(fg).SingleLine()
			})
		}
		iconBtn("↑", index == 0, onUp)
		iconBtn("↓", index >= total-1, onDown)
		ui.Button(c, "删除").Size(56, rowH-4).FontSize(hintSize).TextColor(colDanger).OnClick(onDelete)
	})
}

// columnWidths are the widths of the editable lists, so the header and the
// rows share one grid.
const (
	colIP     float32 = 156
	colMask   float32 = 156
	colGW     float32 = 160
	colMetric float32 = 72
	colDNS    float32 = 260
)
