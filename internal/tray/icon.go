package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"strconv"
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// IconState 表示托盘图标状态。
type IconState int

const (
	IconNormal IconState = iota
	IconRefreshing
	IconWarning
)

var (
	colorPrimary = color.RGBA{78, 168, 255, 255}
	colorWarn    = color.RGBA{251, 191, 36, 255}
	colorErr     = color.RGBA{248, 113, 113, 255}
	colorText    = color.RGBA{245, 247, 250, 255}
)

// FormatCompact 把余量压缩成托盘可显示短文本（最多 4 字符）。
func FormatCompact(v float64) string {
	if v >= 10000 {
		return strconv.FormatFloat(math.Round(v/1000), 'f', 0, 64) + "k"
	}
	// 1000~9999：优先显示整数（如 4028），仅当位数不足时用一位小数补足辨识度。
	if v >= 1000 {
		return strconv.FormatFloat(math.Trunc(v), 'f', 0, 64)
	}
	if v >= 100 {
		return strconv.FormatFloat(v, 'f', 0, 64)
	}
	s := strconv.FormatFloat(v, 'f', 1, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	return s
}

// RenderIcon 绘制 32x32 托盘图标：与主程序一致的深色底 + 蓝色 L logo，
// 并在底部叠加积分数字角标，兼顾品牌统一与常驻显示余量。
func RenderIcon(label string, state IconState) []byte {
	const size = 32
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{0, 0, 0, 0}}, image.Point{}, draw.Src)

	accent := colorPrimary
	accentDark := color.RGBA{47, 125, 251, 255}
	switch state {
	case IconWarning:
		accent = colorErr
		accentDark = color.RGBA{200, 70, 70, 255}
	case IconRefreshing:
		accent = colorWarn
		accentDark = color.RGBA{200, 150, 40, 255}
	}

	// 深色圆角底 + 描边（与前端 logo 风格一致）
	drawRoundedRect(img, 1, 1, size-2, size-2, 7, color.RGBA{16, 21, 31, 240})
	drawRoundedRectOutline(img, 1, 1, size-2, size-2, 7, accent)

	// 蓝色 L 字母（几何块），位于上半部
	bar := 3
	lLeft, lTop, lBottom, lRight := 9, 4, 18, 23
	for y := lTop; y < lBottom; y++ {
		c := lerpColor(accent, accentDark, float64(y-lTop)/float64(lBottom-lTop))
		for x := lLeft; x < lLeft+bar; x++ {
			img.Set(x, y, c)
		}
	}
	for x := lLeft; x < lRight; x++ {
		c := lerpColor(accentDark, accent, float64(x-lLeft)/float64(lRight-lLeft))
		for y := lBottom - bar; y < lBottom; y++ {
			img.Set(x, y, c)
		}
	}

	// 底部分隔条 + 积分数字角标
	drawRect(img, 4, 20, 24, 1, color.RGBA{35, 45, 61, 255})
	text := label
	if len(text) > 5 {
		text = text[:5]
	}
	face := basicfont.Face7x13
	w := font.MeasureString(face, text).Ceil()
	dotX := (size - w) / 2
	if dotX < 2 {
		dotX = 2
	}
	d := font.Drawer{Dst: img, Src: image.NewUniform(colorText), Face: face, Dot: fixed.P(dotX, 31)}
	d.DrawString(text)

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

func lerpColor(a, b color.RGBA, t float64) color.RGBA {
	return color.RGBA{
		uint8(float64(a.R)*(1-t) + float64(b.R)*t),
		uint8(float64(a.G)*(1-t) + float64(b.G)*t),
		uint8(float64(a.B)*(1-t) + float64(b.B)*t),
		255,
	}
}

func drawRect(img *image.RGBA, x, y, w, h int, c color.Color) {
	for j := y; j < y+h; j++ {
		for i := x; i < x+w; i++ {
			if i >= 0 && i < img.Bounds().Dx() && j >= 0 && j < img.Bounds().Dy() {
				img.Set(i, j, c)
			}
		}
	}
}

func drawRoundedRect(img *image.RGBA, x0, y0, w, h, r int, c color.Color) {
	for y := y0; y < y0+h; y++ {
		for x := x0; x < x0+w; x++ {
			dx, dy := 0, 0
			if x < x0+r {
				dx = x0 + r - x
			} else if x >= x0+w-r {
				dx = x - (x0 + w - r - 1)
			}
			if y < y0+r {
				dy = y0 + r - y
			} else if y >= y0+h-r {
				dy = y - (y0 + h - r - 1)
			}
			if dx*dx+dy*dy <= r*r {
				img.Set(x, y, c)
			}
		}
	}
}

func drawRoundedRectOutline(img *image.RGBA, x0, y0, w, h, r int, c color.Color) {
	for y := y0; y < y0+h; y++ {
		for x := x0; x < x0+w; x++ {
			inside := false
			dx, dy := 0, 0
			if x < x0+r {
				dx = x0 + r - x
			} else if x >= x0+w-r {
				dx = x - (x0 + w - r - 1)
			}
			if y < y0+r {
				dy = y0 + r - y
			} else if y >= y0+h-r {
				dy = y - (y0 + h - r - 1)
			}
			if dx*dx+dy*dy <= r*r {
				inside = true
			}
			if !inside {
				continue
			}
			edge := x <= x0 || x >= x0+w-1 || y <= y0 || y >= y0+h-1
			if !edge && dx*dx+dy*dy >= (r-1)*(r-1) {
				edge = true
			}
			if edge {
				img.Set(x, y, c)
			}
		}
	}
}
