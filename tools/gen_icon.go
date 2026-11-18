//go:build ignore

package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
)

type entry struct {
	size int
	dib  []byte
}

func main() {
	root := "d:/developProject/idea-codebuddy-usage"
	iconsDir := filepath.Join(root, "icons")
	winDir := filepath.Join(root, "build", "windows")
	_ = os.MkdirAll(iconsDir, 0o755)
	_ = os.MkdirAll(winDir, 0o755)

	sizes := []int{16, 32, 48, 64, 256}
	entries := make([]entry, 0, len(sizes))
	for _, size := range sizes {
		img := renderLogo(size)
		var pb bytes.Buffer
		_ = png.Encode(&pb, img)
		_ = os.WriteFile(filepath.Join(iconsDir, "logo_"+itoa(size)+".png"), pb.Bytes(), 0o644)
		if size == 256 {
			_ = os.WriteFile(filepath.Join(winDir, "icon.png"), pb.Bytes(), 0o644)
		}
		entries = append(entries, entry{size, encodeDIB(img)})
	}
	writeICO(filepath.Join(winDir, "icon.ico"), entries)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := [20]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func renderLogo(size int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	bg := color.RGBA{16, 21, 31, 255}
	draw.Draw(img, img.Bounds(), &image.Uniform{bg}, image.Point{}, draw.Src)
	pad := size / 9
	inner := image.Rect(pad, pad, size-pad, size-pad)
	drawRect(img, inner, color.RGBA{27, 35, 48, 255})
	drawRect(img, image.Rect(pad, pad, size-pad, pad+(size-2*pad)/3), color.RGBA{35, 45, 61, 255})

	accent1 := color.RGBA{78, 168, 255, 255}
	accent2 := color.RGBA{47, 125, 251, 255}
	bar := max(size/9, 2)
	left := size * 33 / 100
	top := size * 24 / 100
	bottom := size * 76 / 100
	right := size * 70 / 100
	for y := top; y < bottom; y++ {
		c := lerp(accent1, accent2, float64(y-top)/float64(bottom-top))
		for x := left; x < left+bar; x++ {
			img.Set(x, y, c)
		}
	}
	for x := left; x < right; x++ {
		c := lerp(accent2, accent1, float64(x-left)/float64(right-left))
		for y := bottom - bar; y < bottom; y++ {
			img.Set(x, y, c)
		}
	}
	transparent := color.RGBA{0, 0, 0, 0}
	cut := pad
	for y := 0; y < cut; y++ {
		for x := 0; x < cut; x++ {
			if (cut-x)*(cut-x)+(cut-y)*(cut-y) > cut*cut {
				img.Set(x, y, transparent)
				img.Set(size-1-x, y, transparent)
				img.Set(x, size-1-y, transparent)
				img.Set(size-1-x, size-1-y, transparent)
			}
		}
	}
	return img
}

func lerp(a, b color.RGBA, t float64) color.RGBA {
	return color.RGBA{
		uint8(float64(a.R)*(1-t) + float64(b.R)*t),
		uint8(float64(a.G)*(1-t) + float64(b.G)*t),
		uint8(float64(a.B)*(1-t) + float64(b.B)*t),
		255,
	}
}

func drawRect(img *image.RGBA, r image.Rectangle, c color.Color) {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			img.Set(x, y, c)
		}
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// encodeDIB 把 RGBA 图像编码为传统 ICO 使用的 DIB（BITMAPINFOHEADER + BGRA 底向上像素 + AND 掩码）。
func encodeDIB(src *image.RGBA) []byte {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	headerSize := 40
	xorSize := w * h * 4
	andStride := ((w + 31) / 32) * 4
	andSize := andStride * h
	data := make([]byte, headerSize+xorSize+andSize)

	le32put(data[0:4], uint32(headerSize))
	le32put(data[4:8], uint32(w))
	le32put(data[8:12], uint32(h*2)) // XOR + AND
	le16put(data[12:14], 1)          // planes
	le16put(data[14:16], 32)         // bit count
	le32put(data[16:20], 0)          // BI_RGB
	le32put(data[20:24], uint32(xorSize+andSize))

	// 像素自下而上，BGRA
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, g, bl, a := src.At(x, y).RGBA()
			o := headerSize + ((h-1-y)*w+x)*4
			data[o] = byte(bl >> 8)
			data[o+1] = byte(g >> 8)
			data[o+2] = byte(r >> 8)
			data[o+3] = byte(a >> 8)
		}
	}
	// AND mask：alpha<128 的像素置 1（自下而上，行按 4 字节对齐）
	andStart := headerSize + xorSize
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			_, _, _, a := src.At(x, y).RGBA()
			if a>>8 < 128 {
				row := andStart + (h-1-y)*andStride
				data[row+x/8] |= 1 << uint(7-(x%8))
			}
		}
	}
	return data
}

func writeICO(path string, entries []entry) {
	header := make([]byte, 6+16*len(entries))
	le16put(header[0:2], 0)
	le16put(header[2:4], 1)
	le16put(header[4:6], uint16(len(entries)))
	offset := 6 + 16*len(entries)
	for i, e := range entries {
		o := 6 + i*16
		w := e.size
		if w >= 256 {
			w = 0
		}
		header[o] = byte(w)
		header[o+1] = byte(w)
		header[o+2] = 0
		header[o+3] = 0
		le16put(header[o+4:o+6], 1)
		le16put(header[o+6:o+8], 32)
		le32put(header[o+8:o+12], uint32(len(e.dib)))
		le32put(header[o+12:o+16], uint32(offset))
		offset += len(e.dib)
	}
	out := append(header, []byte{}...)
	for _, e := range entries {
		out = append(out, e.dib...)
	}
	_ = os.WriteFile(path, out, 0o644)
}

func le16put(b []byte, v uint16) {
	binary.LittleEndian.PutUint16(b, v)
}
func le32put(b []byte, v uint32) {
	binary.LittleEndian.PutUint32(b, v)
}
