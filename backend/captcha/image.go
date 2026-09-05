package captcha

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
)

// Raster glyphs avoid shipping plaintext answers in SVG, markup or metadata.
var glyphs = [10][7]string{
	{"01110", "11011", "11011", "11011", "11011", "11011", "01110"},
	{"00100", "01100", "00100", "00100", "00100", "00100", "01110"},
	{"01110", "10001", "00001", "00010", "00100", "01000", "11111"},
	{"11110", "00001", "00001", "01110", "00001", "00001", "11110"},
	{"00010", "00110", "01010", "10010", "11111", "00010", "00010"},
	{"11111", "10000", "10000", "11110", "00001", "00001", "11110"},
	{"01110", "10000", "10000", "11110", "10001", "10001", "01110"},
	{"11111", "00001", "00010", "00100", "00100", "01000", "01000"},
	{"01110", "10001", "10001", "01110", "10001", "10001", "01110"},
	{"01110", "10001", "10001", "01111", "00001", "00001", "01110"},
}

func renderPNG(code string) (string, error) {
	noise := make([]byte, 512)
	if _, err := rand.Read(noise); err != nil {
		return "", err
	}
	canvas := image.NewRGBA(image.Rect(0, 0, 180, 58))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.RGBA{238, 247, 248, 255}}, image.Point{}, draw.Src)
	for n := 0; n < 100; n++ {
		canvas.Set(int(noise[n*2])%180, int(noise[n*2+1])%58, color.RGBA{145, 185, 193, 255})
	}
	for n := 0; n < 2; n++ {
		for x := 0; x < 180; x++ {
			y := 15 + n*25 + int(5*math.Sin(float64(x+int(noise[210+n]))/17))
			canvas.Set(x, y, color.RGBA{174, 207, 210, 255})
		}
	}
	const glyphWidth, glyphSpacing = 20, 28
	codeWidth := glyphWidth
	if len(code) > 1 {
		codeWidth += (len(code) - 1) * glyphSpacing
	}
	startX := (canvas.Bounds().Dx() - codeWidth) / 2
	for index, digit := range []byte(code) {
		x := startX + index*glyphSpacing + int(noise[220+index])%3
		y := 12 + int(noise[230+index])%7 - 3
		ink := color.RGBA{uint8(28 + noise[240+index]%35), uint8(70 + noise[250+index]%35), uint8(92 + noise[260+index]%35), 255}
		for row, cells := range glyphs[digit-'0'] {
			offset := int(math.Sin(float64(row)/2+float64(noise[270+index])) * 1.6)
			for column, bit := range []byte(cells) {
				if bit == '1' {
					draw.Draw(canvas, image.Rect(x+column*4+offset, y+row*4, x+(column+1)*4+offset, y+(row+1)*4), &image.Uniform{C: ink}, image.Point{}, draw.Src)
				}
			}
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, canvas); err != nil {
		return "", err
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes()), nil
}
