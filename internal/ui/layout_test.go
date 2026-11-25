package ui

import (
	"image/color"
	"testing"

	"github.com/egoist/mygo/ui"
)

// bounds returns where a text sits in the frame.
func bounds(t *testing.T, tt *ui.Tester, label string) ui.Rect {
	t.Helper()
	r, ok := tt.Find(label)
	if !ok {
		t.Fatalf("%q is not in the frame; texts: %v", label, tt.Texts())
	}
	return r
}

func nearly(a, b float32) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d < 0.5
}

// TestMainLayoutOnTheGrid checks the geometry the design depends on: labels on
// one column, buttons of a row the same size, the two columns separated, and
// the row rhythm of the IPv4 panel even. It is the machine-checkable half of
// the visual review.
func TestMainLayoutOnTheGrid(t *testing.T) {
	app := newTestApp(t)
	tt := ui.NewTester(app.MainView, 900, 560)

	// Section titles of the left column start on one x.
	hardware := bounds(t, tt, "选择硬件")
	computer := bounds(t, tt, "计算机信息")
	mac := bounds(t, tt, "网卡 MAC 地址管理")
	if !nearly(hardware.X, computer.X) || !nearly(computer.X, mac.X) {
		t.Errorf("section titles do not share x: %v %v %v", hardware.X, computer.X, mac.X)
	}
	if !(hardware.Y < computer.Y && computer.Y < mac.Y) {
		t.Errorf("sections are not stacked: %v %v %v", hardware.Y, computer.Y, mac.Y)
	}

	// The labels of a two-field row share the line.
	nameLabel := bounds(t, tt, "计算机名:")
	groupLabel := bounds(t, tt, "工作组:")
	if !nearly(nameLabel.Y, groupLabel.Y) || !nearly(nameLabel.H, groupLabel.H) {
		t.Errorf("labels of a row are not on one line: %+v %+v", nameLabel, groupLabel)
	}

	// A row of buttons: same y, same height, left to right.
	row := []string{"启用网卡", "禁用网卡", "打开网络连接"}
	first := bounds(t, tt, row[0])
	prev := first
	for _, label := range row[1:] {
		r := bounds(t, tt, label)
		if !nearly(r.Y, first.Y) || !nearly(r.H, first.H) {
			t.Errorf("%q is off the button row: %+v vs %+v", label, r, first)
		}
		if r.X <= prev.X {
			t.Errorf("%q is not right of the previous button (x %v after %v)", label, r.X, prev.X)
		}
		prev = r
	}
	// The next row is one row height below, and its buttons stay on one line.
	second := bounds(t, tt, "随机生成计算机名")
	if second.Y <= first.Y {
		t.Errorf("the second button row is not below the first: %v %v", second.Y, first.Y)
	}
	prev = second
	for _, label := range []string{"修改计算机名", "修改工作组"} {
		r := bounds(t, tt, label)
		if !nearly(r.Y, second.Y) || !nearly(r.H, second.H) {
			t.Errorf("%q is off its row: %+v vs %+v", label, r, second)
		}
		if r.X <= prev.X {
			t.Errorf("%q is not right of the previous button (x %v after %v)", label, r.X, prev.X)
		}
		prev = r
	}

	// The IPv4 panel sits right of the left column, and its rows keep one
	// rhythm: the same label width and an even step between rows.
	ip := bounds(t, tt, "IP 地址:")
	mask := bounds(t, tt, "子网掩码:")
	gateway := bounds(t, tt, "默认网关:")
	dns1 := bounds(t, tt, "首选 DNS:")
	dns2 := bounds(t, tt, "备用 DNS:")
	if ip.X <= mac.X {
		t.Errorf("the IPv4 panel is not right of the left column: %v vs %v", ip.X, mac.X)
	}
	for _, r := range []ui.Rect{mask, gateway, dns1, dns2} {
		if !nearly(r.X, ip.X) || !nearly(r.W, ip.W) {
			t.Errorf("IPv4 labels do not share the label column: %+v vs %+v", r, ip)
		}
	}
	step1 := mask.Y - ip.Y
	step2 := gateway.Y - mask.Y
	if !nearly(step1, step2) {
		t.Errorf("IPv4 rows do not step evenly: %v then %v", step1, step2)
	}
	if step1 < 24 || step1 > 40 {
		t.Errorf("IPv4 row step %v is off the grid", step1)
	}

	// The status bar is the last line of the window.
	version := bounds(t, tt, "vtest")
	if version.Y <= dns2.Y {
		t.Errorf("the status bar is not below the content: %v vs %v", version.Y, dns2.Y)
	}
}

// TestWindowBackgroundIsTheSheet checks the palette reached the frame: the
// window is the light grey of the design, not the theme's default white.
func TestWindowBackgroundIsTheSheet(t *testing.T) {
	app := newTestApp(t)
	tt := ui.NewTester(app.MainView, 900, 560)
	img := tt.Image()
	if img == nil {
		t.Skip("no frame was rendered")
	}
	want := color.RGBA{R: 0xe8, G: 0xed, B: 0xf3, A: 0xff}
	got := img.RGBAAt(1, 1)
	if got != want {
		t.Errorf("window background = %v, want %v", got, want)
	}
	// A panel's face is white: sample inside the first panel, right of its
	// border and under its title strip.
	panelFace := img.RGBAAt(60, 120)
	if panelFace == want {
		t.Errorf("panel face is the window background, the panel border and fill are missing")
	}
}

// TestAdvancedLayoutOnTheGrid checks the editable lists line their header and
// rows up on the same columns.
func TestAdvancedLayoutOnTheGrid(t *testing.T) {
	app := newTestApp(t)
	app.Adv.Tab = 0
	app.Adv.IPs = []AddressRow{
		{Value: "192.168.1.10", Mask: "255.255.255.0"},
		{Value: "10.0.0.5", Mask: "255.0.0.0"},
	}
	tt := ui.NewTester(app.AdvancedView, 700, 560)
	tab := bounds(t, tt, "IP 地址")
	h1 := bounds(t, tt, "地址")
	h2 := bounds(t, tt, "掩码")
	if !nearly(h1.Y, h2.Y) {
		t.Errorf("table headers are not on one line: %v %v", h1.Y, h2.Y)
	}
	if h2.X <= h1.X {
		t.Errorf("the second header is not right of the first: %v %v", h1.X, h2.X)
	}
	if h1.Y <= tab.Y {
		t.Errorf("the header is not below the tabs: %v vs %v", h1.Y, tab.Y)
	}
	// The rows are below the header, and the add row under them.
	firstRow := bounds(t, tt, "删除")
	if firstRow.Y <= h1.Y {
		t.Errorf("the first row is not below the header: %v vs %v", firstRow.Y, h1.Y)
	}
	add := bounds(t, tt, "添加")
	if add.Y <= firstRow.Y {
		t.Errorf("the add button is not below the rows: %v vs %v", add.Y, firstRow.Y)
	}
}
