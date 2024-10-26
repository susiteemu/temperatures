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

var (
	dpi           = float64(72)
	labelFontfile = "resources/BitterPro-Medium.ttf"
	fontfile      = "resources/BitterPro-Bold.ttf"
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

func drawResult(measurements []Measurement, imageConfiguration *GenerateImageConfiguration) {

	imgWidth := imageConfiguration.ImgW
	imgHeight := imageConfiguration.ImgH
	fontSizeL := imageConfiguration.FontL
	fontSizeM := imageConfiguration.FontM
	fontSizeS := imageConfiguration.FontS

	defaultFont, err := loadFont(fontfile)
	if err != nil {
		log.Error().Err(err)
		return
	}

	labelFont, err := loadFont(labelFontfile)
	if err != nil {
		log.Error().Err(err)
		return
	}

	fg, bg := image.Black, image.White
	rgba := image.NewRGBA(image.Rect(0, 0, imgWidth, imgHeight))
	draw.Draw(rgba, rgba.Bounds(), bg, image.Point{0, 0}, draw.Src)

	defaultFontHeight := int(math.Ceil(fontSizeL * spacing * dpi / 72))
	labelFontHeight := int(math.Ceil(fontSizeM * spacing * dpi / 72))
	smallFontHeight := int(math.Ceil(fontSizeS * spacing * dpi / 72))

	// Draw the text.
	h := font.HintingNone
	labelDrawer := &font.Drawer{
		Dst: rgba,
		Src: fg,
		Face: truetype.NewFace(labelFont, &truetype.Options{
			Size:    fontSizeM,
			DPI:     dpi,
			Hinting: h,
		}),
	}

	defaultDrawer := &font.Drawer{
		Dst: rgba,
		Src: fg,
		Face: truetype.NewFace(defaultFont, &truetype.Options{
			Size:    fontSizeL,
			DPI:     dpi,
			Hinting: h,
		}),
	}

	smallDrawer := &font.Drawer{
		Dst: rgba,
		Src: fg,
		Face: truetype.NewFace(defaultFont, &truetype.Options{
			Size:    fontSizeS,
			DPI:     dpi,
			Hinting: h,
		}),
	}

	whiteSmallDrawer := &font.Drawer{
		Dst: rgba,
		Src: bg,
		Face: truetype.NewFace(defaultFont, &truetype.Options{
			Size:    fontSizeS,
			DPI:     dpi,
			Hinting: h,
		}),
	}
	cols := 2
	rows := len(measurements) / cols
	rowHeight := imgHeight / rows
	colWidth := imgWidth / cols
	for idx, m := range measurements {
		// calculate center for grid
		cX := colWidth / 2
		cY := rowHeight*(idx/2) + (rowHeight / 2) - (labelFontHeight / 2) - (defaultFontHeight / 2)
		if (idx+1)%2 == 0 {
			cX = colWidth + colWidth/2
		}

		label := m.FormatLabel()
		labelDrawer.Dot = fixed.Point26_6{
			X: fixed.I(cX) - labelDrawer.MeasureString(label)/2,
			Y: fixed.I(cY),
		}
		labelDrawer.DrawString(label)

		val := m.FormatValue()
		defaultDrawer.Dot = fixed.Point26_6{
			X: fixed.I(cX) - defaultDrawer.MeasureString(val)/2,
			Y: fixed.I(cY + defaultFontHeight),
		}
		defaultDrawer.DrawString(val)

		if !m.Empty {
			log.Debug().Msg("Drawing other things")
			labelDrawer.Dot = fixed.Point26_6{
				X: fixed.I(cX) + defaultDrawer.MeasureString(val)/2,
				Y: fixed.I(cY + (defaultFontHeight / 2)),
			}
			labelDrawer.DrawString("°C")

			margin := 8
			nY := int(idx/2)*rowHeight + rowHeight - margin
			log.Debug().Msgf("At %d calculated modulus %d and got nY %d", idx, (idx+1)%2, nY)

			nX := colWidth - margin
			if (idx+1)%2 == 0 {
				nX = colWidth + colWidth - margin
			}

			ageWidth := smallDrawer.MeasureString(m.FormatAge())
			if ageWidth.Ceil() > 1 {
				rect := image.Rect(nX-ageWidth.Ceil()-margin, nY-smallFontHeight, nX+margin, nY+margin)
				draw.Draw(rgba, rect, fg, image.Point{0, 0}, draw.Src)

				whiteSmallDrawer.Dot = fixed.Point26_6{
					X: fixed.I(nX) - ageWidth,
					Y: fixed.I(nY),
				}
				whiteSmallDrawer.DrawString(m.FormatAge())

				nX = nX - ageWidth.Ceil() - margin - (margin + 1)
			}

			slopeWidth := smallDrawer.MeasureString(m.FormatSlope())
			if slopeWidth.Ceil() > 1 {
				rect := image.Rect(nX-slopeWidth.Ceil()-margin, nY-smallFontHeight, nX+margin, nY+margin)
				draw.Draw(rgba, rect, fg, image.Point{0, 0}, draw.Src)

				whiteSmallDrawer.Dot = fixed.Point26_6{
					X: fixed.I(nX) - slopeWidth,
					Y: fixed.I(nY),
				}
				whiteSmallDrawer.DrawString(m.FormatSlope())
			}

		}

	}
	ruler := image.Black
	for i := 0; i < imgHeight; i++ {
		rgba.Set(imgWidth/2, i, ruler)
	}
	for y := 0; y < rows; y++ {
		for x := 0; x < imgWidth; x++ {
			rgba.Set(x, y*rowHeight, ruler)
		}
	}

	uX := 5
	uM := 5
	currentTime := time.Now()
	formattedTime := currentTime.Format("15:04")
	updatedText := fmt.Sprintf("%s", formattedTime)
	smallDrawer.Dot = fixed.Point26_6{
		//X: fixed.I(imgW) - do.MeasureString(updatedText) - fixed.I(10),
		X: fixed.I(uX),
		Y: fixed.I(smallFontHeight),
	}
	smallDrawer.DrawString(updatedText)
	updatedWidth := smallDrawer.MeasureString(updatedText)
	for i := 0; i < smallFontHeight+uM; i++ {
		rgba.Set(uX+updatedWidth.Ceil()+uM, i, ruler)
	}
	for i := 0; i < uX+updatedWidth.Ceil()+uM; i++ {
		rgba.Set(i, smallFontHeight+uM, ruler)
	}

	outFile, err := os.Create(imageConfiguration.Output)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to write to file %s", imageConfiguration.Output)
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

	cmd := exec.Command("convert", imageConfiguration.Output, "-gravity", "center", "-extent", fmt.Sprintf("%dx%d", imgWidth, imgHeight), "-colorspace", "gray", "-depth", "8", "-rotate", "-90", imageConfiguration.Output)
	_, err = cmd.Output()
	if err != nil {
		log.Error().Err(err).Msg("Failed to run 'convert' command")
	}
	log.Info().Msg("Successfully wrote image")
}
