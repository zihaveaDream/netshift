package ui

import (
	"strings"

	"github.com/egoist/mygo/ui"
)

// High-contrast, crisp Windows 11 Fluent utility grid tokens, in DIPs.
const (
	unit         float32 = 4
	windowPad    float32 = 14
	colGap       float32 = 14
	rowGap       float32 = 9
	labelW       float32 = 88
	labelWNarrow float32 = 68
	rowH         float32 = 30
	buttonH      float32 = 30
	panelHeaderH float32 = 36
	panelPad     float32 = 13
	statusH      float32 = 30
	cardRadius   float32 = 6
	ctrlRadius   float32 = 4
)

// High-legibility type scale tuned for Source Han Serif SC (思源宋体).
// Strictly two weights across the entire app (500 Medium / 700 Bold) for
// crisp stroke density and effortless long-session readability.
const (
	bodySize      float32 = 13.5
	hintSize      float32 = 12
	titleSize     float32 = 14
	weightRegular         = 500
	weightHeading         = 700
)

// High-contrast palette: crisp obsidian ink on pure white cards over a defined
// cool-slate sheet; all text pairs exceed WCAG AA/AAA contrast ratios.
var (
	colBackground     = ui.Hex("#e8edf3")
	colSurface        = ui.Hex("#ffffff")
	colSurfaceSubtle  = ui.Hex("#f3f6fa")
	colSurfaceHover   = ui.Hex("#e3effc")
	colSurfacePressed = ui.Hex("#cfe4fa")
	colBorder         = ui.Hex("#94a3b8")
	colBorderSoft     = ui.Hex("#c4d0dd")
	colText           = ui.Hex("#05080d") // near-black, ~20:1 on white
	colLabel          = ui.Hex("#111827") // deep obsidian for form labels (~17.7:1)
	colTextMuted      = ui.Hex("#334155") // high-contrast slate for hints (~10.4:1)
	colAccent         = ui.Hex("#0052a8")
	colAccentHover    = ui.Hex("#00438a")
	colAccentPressed  = ui.Hex("#003570")
	colAccentText     = ui.Hex("#ffffff")
	colPanelHeader    = ui.Hex("#f4f7fb")
	colStatusBar      = ui.Hex("#e4eaf1")
	colShadow         = ui.RGBA(11, 15, 20, 0.08)

	// High-contrast semantic colors + crisp pill backgrounds.
	colSuccess       = ui.Hex("#0a6610")
	colSuccessBg     = ui.Hex("#e3f5e6")
	colSuccessBorder = ui.Hex("#93d19e")
	colDanger        = ui.Hex("#b42318")
	colDangerBg      = ui.Hex("#fde8e7")
	colDangerBorder  = ui.Hex("#efa7a3")
	colInfo          = ui.Hex("#0052a3")
	colInfoBg        = ui.Hex("#e5f0fc")
	colInfoBorder    = ui.Hex("#9ec5f0")
	colWarn          = ui.Hex("#8a4b00")
	colWarnBg        = ui.Hex("#fff1db")
	colWarnBorder    = ui.Hex("#eab975")

	// Ping grid cell colors (high contrast for quick scanning).
	colOnline       = ui.Hex("#117a37")
	colOnlineBorder = ui.Hex("#0b5c28")
	colOffline      = ui.Hex("#e2e8f0")
	colOfflineText  = ui.Hex("#334155")
	colUnknown      = ui.Hex("#ffffff")

	// Terminal preview block colors.
	colCodeBg    = ui.Hex("#11161d")
	colCodeText  = ui.Hex("#f1f5f9")
	colCodeMuted = ui.Hex("#94a3b8")
)

const monoFont = "Consolas, Courier New, monospace"

// cachedTheme is initialized once to avoid per-frame theme allocations.
var cachedTheme = buildAppTheme()

func buildAppTheme() *ui.Theme {
	t := *ui.LightTheme()
	t.Background = colBackground
	t.Surface = colSurface
	t.SurfaceHover = colSurfaceHover
	t.SurfacePressed = colSurfacePressed
	t.Border = colBorder
	t.Text = colText
	t.TextMuted = colTextMuted
	t.Accent = colAccent
	t.AccentHover = colAccentHover
	t.AccentPressed = colAccentPressed
	t.AccentText = colAccentText
	t.Danger = colDanger
	t.Warning = colWarn
	t.Success = colSuccess
	t.Selection = ui.RGBA(0, 90, 181, 0.18)
	t.Focus = ui.RGBA(0, 90, 181, 0.75)
	t.Scrollbar = ui.RGBA(11, 15, 20, 0.34)
	t.Radius = ctrlRadius
	t.Spacing = unit
	t.FontSize = bodySize
	t.Font = "Noto Serif SC, Source Han Serif SC, 思源宋体, SimSun, Microsoft YaHei UI, serif"
	return &t
}

// useTheme applies the cached Fluent theme to the current frame.
func useTheme(c *ui.Context) *ui.Theme {
	c.SetTheme(cachedTheme)
	return cachedTheme
}

// panel renders a crisp white card with defined border, subtle elevation shadow,
// accent bar header and padded content body.
func panel(c *ui.Context, title string, width float32, fn func()) ui.Element {
	return panelHeader(c, title, width, nil, fn)
}

// panelHeader is panel with an optional right-side header slot.
func panelHeader(c *ui.Context, title string, width float32, right func(), fn func()) ui.Element {
	col := ui.Column(c).Background(colSurface).Border(1, colBorder).Radius(cardRadius).
		Shadow(0, 1, 3, 0, colShadow).Clip()
	if width > 0 {
		col = col.Width(width)
	}
	return col.Children(func() {
		if title != "" {
			ui.Row(c).Height(panelHeaderH).PaddingX(panelPad).Gap(8).AlignItems(ui.Center).
				Background(colPanelHeader).
				BorderWidth(0, 0, 1, 0).BorderColor(colBorderSoft).
				Children(func() {
					ui.Box(c).Size(3.5, 14).Background(colAccent).Radius(2)
					ui.Text(c, title).FontSize(titleSize).FontWeight(weightHeading).TextColor(colText).SingleLine()
					ui.Spacer(c).Grow(1)
					if right != nil {
						right()
					}
				})
		}
		ui.Column(c).Grow(1).Padding(panelPad).Gap(rowGap).Children(fn)
	})
}

// pill draws a crisp semantic status badge.
func pill(c *ui.Context, text string, fg, bg, border ui.Color) ui.Element {
	return ui.Row(c).Height(22).PaddingX(8).AlignItems(ui.Center).
		Background(bg).Border(1, border).Radius(11).Children(func() {
		ui.Text(c, text).FontSize(hintSize).FontWeight(weightRegular).TextColor(fg).SingleLine()
	})
}

// quickChip draws a crisp clickable shortcut tag (e.g. /24, 阿里 DNS).
func quickChip(c *ui.Context, label string, disabled bool, onClick func()) ui.Element {
	bg := colSurfaceSubtle
	fg := colLabel
	bdr := colBorder
	b := ui.ButtonBase(c).Height(22).PaddingX(8).AlignItems(ui.Center).Justify(ui.Center).
		Background(bg).Border(1, bdr).Radius(ctrlRadius).Disabled(disabled)
	if !disabled && b.Hovered() {
		b = b.Background(colSurfaceHover).Border(1, colAccent)
		fg = colAccent
	}
	b.OnClick(onClick).Children(func() {
		ui.Text(c, label).FontSize(hintSize).FontWeight(weightRegular).TextColor(fg).SingleLine()
	})
	return b
}

// formRow is one labelled row: crisp label column + controls on a 30 DIP line.
func formRow(c *ui.Context, label string, fn func()) {
	formRowW(c, label, labelW, fn)
}

// formRowW is formRow with another label width.
func formRowW(c *ui.Context, label string, width float32, fn func()) {
	ui.Row(c).Height(rowH).Gap(rowGap).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).Width(width).FontSize(bodySize).FontWeight(weightRegular).TextColor(colLabel).SingleLine()
		fn()
	})
}

// pairRow is a row of two labelled fields that fill the row evenly.
func pairRow(c *ui.Context, label1 string, fn1 func(), label2 string, label2W float32, fn2 func()) {
	ui.Row(c).Height(rowH).Gap(rowGap).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label1).Width(labelW).FontSize(bodySize).FontWeight(weightRegular).TextColor(colLabel).SingleLine()
		fn1()
		ui.Text(c, label2).Width(label2W).FontSize(bodySize).FontWeight(weightRegular).TextColor(colLabel).SingleLine()
		fn2()
	})
}

// actionRow is a row of buttons aligned on the grid.
func actionRow(c *ui.Context, fn func()) {
	ui.Row(c).Height(buttonH).Gap(rowGap).AlignItems(ui.Center).Children(fn)
}

// button is a crisp action button; width <= 0 lets it grow flexibly.
func button(c *ui.Context, label string, width float32, onClick func()) ui.Element {
	b := ui.Button(c, label).Height(buttonH).FontSize(bodySize).FontWeight(weightRegular).OnClick(onClick)
	if width > 0 {
		b = b.Width(width)
	}
	return b
}

// primaryButton is the high-contrast accent primary action button.
func primaryButton(c *ui.Context, label string, width float32, onClick func()) ui.Element {
	b := ui.PrimaryButton(c, label).Height(buttonH).FontSize(bodySize).FontWeight(weightRegular).OnClick(onClick)
	if width > 0 {
		b = b.Width(width)
	}
	return b
}

// dangerButton is a clear destructive-tinted button.
func dangerButton(c *ui.Context, label string, width float32, onClick func()) ui.Element {
	b := ui.Button(c, label).Height(buttonH).FontSize(bodySize).FontWeight(weightRegular).TextColor(colDanger).OnClick(onClick)
	if width > 0 {
		b = b.Width(width)
	}
	return b
}

// textField is a crisp single-line input sized for the grid; width <= 0 lets it grow.
// Vertical padding is zeroed so mygo's single-line editor centers glyphs precisely
// along the input's vertical midline instead of pushing text toward the bottom border.
func textField(c *ui.Context, value *string, width float32) ui.Element {
	in := ui.TextInput(c, value).Height(rowH - 2).Padding(0, 10).FontSize(bodySize).FontWeight(weightRegular)
	if width > 0 {
		in = in.Width(width)
	}
	return in
}

// inlineField is a compact single-line table input with exact vertical text centering.
func inlineField(c *ui.Context, value *string, width, height, fontSize float32) ui.Element {
	in := ui.TextInput(c, value).Height(height).Padding(0, 8).FontSize(fontSize).FontWeight(weightRegular)
	if width > 0 {
		in = in.Width(width)
	}
	return in
}

// searchField is a single-line search input with centered icon and text.
func searchField(c *ui.Context, query *string, placeholder string) ui.Element {
	return ui.SearchField(c, query).Height(rowH - 2).Padding(0, 8).FontSize(bodySize).FontWeight(weightRegular).Placeholder(placeholder)
}

// monoField uses the unified app font so IP/MAC inputs match every adjacent
// label, select and text input in stroke weight and optical size.
func monoField(c *ui.Context, value *string, width float32) ui.Element {
	return textField(c, value, width)
}

// hint is the readable secondary line under a group.
func hint(c *ui.Context, text string) {
	ui.Text(c, text).FontSize(hintSize).TextColor(colTextMuted)
}

// note draws a crisp inline error banner when text is not empty.
func note(c *ui.Context, text string) {
	if text == "" {
		return
	}
	ui.Row(c).PaddingX(9).PaddingY(5).Background(colDangerBg).Border(1, colDangerBorder).Radius(ctrlRadius).
		AlignItems(ui.Center).Children(func() {
		ui.Text(c, text).FontSize(hintSize).TextColor(colDanger)
	})
}

// footerBar is the unified action bar placed directly above statusBar on all
// secondary windows, giving every dialog an identical bottom rhythm.
func footerBar(c *ui.Context, fn func()) {
	ui.Row(c).Height(buttonH + 12).PaddingX(windowPad).Gap(rowGap).AlignItems(ui.Center).
		Background(colSurface).BorderWidth(1, 0, 0, 0).BorderColor(colBorderSoft).Children(fn)
}

// statusBar is the footer strip with high-contrast semantic status tinting.
func statusBar(c *ui.Context, text, version string, busy bool) {
	fg := colLabel
	switch {
	case strings.HasPrefix(text, "✗"):
		fg = colDanger
	case strings.HasPrefix(text, "✓"):
		fg = colSuccess
	case busy:
		fg = colAccent
	}
	ui.Row(c).Height(statusH).PaddingX(windowPad).Gap(8).AlignItems(ui.Center).
		Background(colStatusBar).
		BorderWidth(1, 0, 0, 0).BorderColor(colBorder).Children(func() {
		if busy {
			ui.Spinner(c).Size(13, 13)
		}
		ui.Text(c, text).SingleLine().Grow(1).FontSize(hintSize).TextColor(fg)
		if version != "" {
			v := strings.TrimPrefix(version, "v")
			ui.Text(c, "Crafted By OAyy").FontSize(hintSize).TextColor(colTextMuted).SingleLine()
			ui.Text(c, "|").FontSize(hintSize).TextColor(colBorder).SingleLine()
			ui.Text(c, "v"+v).FontSize(hintSize).TextColor(colTextMuted).SingleLine()
		}
	})
}

// headerCol is one column caption of an editable list.
type headerCol struct {
	name  string
	width float32
}

// tableHeader puts the column captions over the rows of an editable list.
func tableHeader(c *ui.Context, cols ...headerCol) {
	ui.Row(c).Height(24).Gap(rowGap).AlignItems(ui.Center).Children(func() {
		for _, col := range cols {
			ui.Text(c, col.name).Width(col.width).FontSize(hintSize).FontWeight(weightHeading).TextColor(colLabel)
		}
	})
}

// monoText shows a crisp console line.
func monoText(c *ui.Context, text string) ui.Element {
	return ui.Text(c, text).Font(monoFont).FontSize(hintSize).TextColor(colText).SingleLine()
}
