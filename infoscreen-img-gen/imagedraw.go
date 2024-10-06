package main

import (
	"bufio"
	"embed"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"math"
	"os"
	"os/exec"
	"time"

	"github.com/golang/freetype/truetype"
	"github.com/rs/zerolog/log"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

//go:embed resources/*.ttf
var resources embed.FS

const imgW = 800
const imgH = 600

var (
	dpi           = float64(72)
	labelFontfile = "resources/BitterPro-Medium.ttf"
	fontfile      = "resources/BitterPro-Bold.ttf"
	labelSize     = float64(24)
	size          = float64(90)
	updatedSize   = float64(16)
	spacing       = 1.1
)

func loadFont(fontfile string) (*truetype.Font, error) {
	fontBytes, err := resources.ReadFile(fontfile)
	if err != nil {
		return nil, err
	}
	f, err := truetype.Parse(fontBytes)
	if err != nil {
		return nil, err
	}
	return f, nil
}

func drawResult(measurements []Measurement, output string) {
	f, err := loadFont(fontfile)
	if err != nil {
		log.Error().Err(err)
		return
	}

	lf, err := loadFont(labelFontfile)
	if err != nil {
		log.Error().Err(err)
		return
	}

	fg, bg := image.Black, image.White
	rgba := image.NewRGBA(image.Rect(0, 0, imgW, imgH))
	draw.Draw(rgba, rgba.Bounds(), bg, image.Point{0, 0}, draw.Src)

	// Draw the text.
	h := font.HintingNone
	dl := &font.Drawer{
		Dst: rgba,
		Src: fg,
		Face: truetype.NewFace(lf, &truetype.Options{
			Size:    labelSize,
			DPI:     dpi,
			Hinting: h,
		}),
	}

	d := &font.Drawer{
		Dst: rgba,
		Src: fg,
		Face: truetype.NewFace(f, &truetype.Options{
			Size:    size,
			DPI:     dpi,
			Hinting: h,
		}),
	}

	do := &font.Drawer{
		Dst: rgba,
		Src: fg,
		Face: truetype.NewFace(f, &truetype.Options{
			Size:    updatedSize,
			DPI:     dpi,
			Hinting: h,
		}),
	}

	dy := int(math.Ceil(size * spacing * dpi / 72))
	ldy := int(math.Ceil(labelSize * spacing * dpi / 72))

	cols := 2
	rows := len(measurements) / cols
	rowH := imgH / rows
	colW := imgW / cols
	for idx, s := range measurements {
		cX := colW / 2
		cY := rowH*(idx/2) + (rowH / 2) - (ldy / 2) - (dy / 2)
		if (idx+1)%2 == 0 {
			cX = colW + colW/2
		}

		label := s.FormatLabel()
		dl.Dot = fixed.Point26_6{
			X: fixed.I(cX) - dl.MeasureString(label)/2,
			Y: fixed.I(cY),
		}
		dl.DrawString(label)

		val := s.FormatValue()
		d.Dot = fixed.Point26_6{
			X: fixed.I(cX) - d.MeasureString(val)/2,
			Y: fixed.I(cY + dy),
		}
		d.DrawString(val)
	}
	ruler := image.Black
	for i := 0; i < imgH; i++ {
		rgba.Set(imgW/2, i, ruler)
	}
	for y := 0; y < rows; y++ {
		for x := 0; x < imgW; x++ {
			rgba.Set(x, y*rowH, ruler)
		}
	}

	y := imgH - 10
	currentTime := time.Now()
	formattedTime := currentTime.Format("2.1 15:04:05")
	updatedText := fmt.Sprintf("Päivitetty: %s", formattedTime)
	do.Dot = fixed.Point26_6{
		X: fixed.I(imgW) - do.MeasureString(updatedText) - fixed.I(10),
		Y: fixed.I(y),
	}
	do.DrawString(updatedText)

	outFile, err := os.Create(output)
	if err != nil {
		log.Error().Err(err)
		os.Exit(1)
	}
	defer outFile.Close()
	b := bufio.NewWriter(outFile)
	err = png.Encode(b, rgba)
	if err != nil {
		log.Error().Err(err)
		os.Exit(1)
	}
	err = b.Flush()
	if err != nil {
		log.Error().Err(err)
		os.Exit(1)
	}

	cmd := exec.Command("convert", output, "-gravity", "center", "-extent", "800x600", "-colorspace", "gray", "-depth", "8", "-rotate", "-90", output)
	_, err = cmd.Output()
	if err != nil {
		log.Error().Err(err).Msg("Failed to run 'convert' command")
	}
	log.Info().Msg("Successfully wrote image")
}
