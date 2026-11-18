package tray

import (
	"image/png"
	"bytes"
	"testing"
)

func TestFormatCompact(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{2431.68, "2431"},
		{10500, "11k"},
		{999, "999"},
		{99.5, "99.5"},
	}
	for _, c := range cases {
		if got := FormatCompact(c.in); got != c.want {
			t.Errorf("FormatCompact(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestRenderIconProducesValidPNG(t *testing.T) {
	for _, state := range []IconState{IconNormal, IconRefreshing, IconWarning} {
		data := RenderIcon("2.4k", state)
		if len(data) == 0 {
			t.Fatalf("state %v produced empty icon", state)
		}
		if _, err := png.Decode(bytes.NewReader(data)); err != nil {
			t.Fatalf("state %v produced invalid PNG: %v", state, err)
		}
	}
}
