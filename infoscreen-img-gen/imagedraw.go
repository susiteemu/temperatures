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
	fontSizeLMinus := imageConfiguration.FontL - 30
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
	defaultHalfCellFontHeight := int(math.Ceil(fontSizeLMinus * spacing * dpi / 72))
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

	halfCellDefaultDrawer := &font.Drawer{
		Dst: rgba,
		Src: fg,
		Face: truetype.NewFace(defaultFont, &truetype.Options{
			Size:    fontSizeLMinus,
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

	const (
		NORMAL_CELL = 0
		DOUBLE_CELL = 1
		HALF_CELL   = 2
		LEFT_SIDE   = 0
		RIGHT_SIDE  = 1
	)
	// indexing starts from top left, does down the left side then continues from top left and goes down
	indexTypes := []int{
		DOUBLE_CELL, NORMAL_CELL, NORMAL_CELL, NORMAL_CELL, HALF_CELL, HALF_CELL,
	}
	currentSide := LEFT_SIDE
	currentY := 0
	for idx := 0; idx < len(measurements); idx++ {

		m := measurements[idx]
		cellType := indexTypes[idx]

		switch cellType {
		case NORMAL_CELL:
			cellX := 0
			cellY := currentY
			cellW := colWidth
			cellH := rowHeight
			switch currentSide {
			case LEFT_SIDE:
				cellX = 0
			case RIGHT_SIDE:
				cellX = colWidth
			}

			drawCell(m, cellX, cellY, cellW, cellH, labelFontHeight, defaultFontHeight, smallFontHeight, labelDrawer, defaultDrawer, smallDrawer, whiteSmallDrawer, rgba, fg)

			currentY += cellH
		case DOUBLE_CELL:
			cellX := 0
			cellY := currentY
			cellW := colWidth
			cellH := (imgHeight / 3) * 2
			switch currentSide {
			case LEFT_SIDE:
				cellX = 0
			case RIGHT_SIDE:
				cellX = colWidth
			}

			drawDoubleCell(m, cellX, cellY, cellW, cellH, labelFontHeight, defaultFontHeight, smallFontHeight, labelDrawer, defaultDrawer, smallDrawer, whiteSmallDrawer, rgba, fg)

			currentY += cellH
		case HALF_CELL:
			cellX := 0
			cellY := currentY
			cellW := colWidth / 2
			cellH := rowHeight
			switch currentSide {
			case LEFT_SIDE:
				cellX = 0
			case RIGHT_SIDE:
				cellX = colWidth
			}

			drawCell(m, cellX, cellY, cellW, cellH, labelFontHeight, defaultHalfCellFontHeight, smallFontHeight, labelDrawer, halfCellDefaultDrawer, smallDrawer, whiteSmallDrawer, rgba, fg)

			if idx+1 < len(measurements) && indexTypes[idx+1] == HALF_CELL {
				idx++
				nextM := measurements[idx]
				cellX += colWidth / 2

				drawCell(nextM, cellX, cellY, cellW, cellH, labelFontHeight, defaultHalfCellFontHeight, smallFontHeight, labelDrawer, halfCellDefaultDrawer, smallDrawer, whiteSmallDrawer, rgba, fg)

			}

			currentY += cellH

		}

		if currentSide != RIGHT_SIDE && currentY+10 >= imgHeight {
			currentSide = RIGHT_SIDE
			currentY = 0
		}

	}
	ruler := image.Black
	for i := 0; i < imgHeight; i++ {
		rgba.Set(imgWidth/2, i, ruler)
	}
	// Left side is divided to two rows: 2/3 and 1/3
	leftSideDivideIdx := 2
	leftSideStop := imgWidth / 2
	for x := 0; x < leftSideStop; x++ {
		rgba.Set(x, leftSideDivideIdx*rowHeight, ruler)
	}
	// Right side is divided to 3x1/3 rows
	rightSideStart := imgWidth / 2
	for y := 0; y < rows; y++ {
		for x := rightSideStart; x < imgWidth; x++ {
			rgba.Set(x, y*rowHeight, ruler)
		}
	}
	// Right side bottom cell is divided to two equal parts
	rightBottomCellX := (imgWidth / 4) * 3
	rightBottomCellY := (imgHeight / 3) * 2
	for i := rightBottomCellY; i < imgHeight; i++ {
		rgba.Set(rightBottomCellX, i, ruler)
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

func drawCell(m Measurement, x, y, w, h, lfh, dfh, sfh int, ld, dd, sd, wsd *font.Drawer, rgba *image.RGBA, fg *image.Uniform) {

	// calculate center for grid
	cX := x + (w / 2)
	cY := y + (h / 2) - (lfh / 2) - (dfh / 2)

	label := m.FormatLabel()
	ld.Dot = fixed.Point26_6{
		X: fixed.I(x + 8),
		Y: fixed.I(y + lfh),
	}
	ld.DrawString(label)

	val := m.FormatValue()
	dd.Dot = fixed.Point26_6{
		X: fixed.I(cX) - dd.MeasureString(val)/2,
		Y: fixed.I(cY + dfh),
	}
	dd.DrawString(val)

	if !m.Empty {
		log.Debug().Msg("Drawing other things")
		ld.Dot = fixed.Point26_6{
			X: fixed.I(cX) + dd.MeasureString(val)/2,
			Y: fixed.I(cY + (dfh / 2)),
		}
		ld.DrawString("°C")

		margin := 8
		nY := y + h - margin
		nX := x + w - margin

		ageWidth := sd.MeasureString(m.FormatAge())
		if ageWidth.Ceil() > 1 {
			rect := image.Rect(nX-ageWidth.Ceil()-margin, nY-sfh, nX+margin, nY+margin)
			draw.Draw(rgba, rect, fg, image.Point{0, 0}, draw.Src)

			wsd.Dot = fixed.Point26_6{
				X: fixed.I(nX) - ageWidth,
				Y: fixed.I(nY),
			}
			wsd.DrawString(m.FormatAge())

			nX = nX - ageWidth.Ceil() - margin - (margin + 1)
		}

		slopeWidth := sd.MeasureString(m.FormatSlope())
		if slopeWidth.Ceil() > 1 {
			rect := image.Rect(nX-slopeWidth.Ceil()-margin, nY-sfh, nX+margin, nY+margin)
			draw.Draw(rgba, rect, fg, image.Point{0, 0}, draw.Src)

			wsd.Dot = fixed.Point26_6{
				X: fixed.I(nX) - slopeWidth,
				Y: fixed.I(nY),
			}
			wsd.DrawString(m.FormatSlope())
		}

	}
}

func drawDoubleCell(m Measurement, x, y, w, h, lfh, dfh, sfh int, ld, dd, sd, wsd *font.Drawer, rgba *image.RGBA, fg *image.Uniform) {

	// calculate center for grid
	cX := x + (w / 2)
	cY := y + (h / 4)

	val := m.FormatValue()
	dd.Dot = fixed.Point26_6{
		X: fixed.I(cX) - dd.MeasureString(val)/2,
		Y: fixed.I(cY),
	}
	dd.DrawString(val)

	if !m.Empty {
		log.Debug().Msg("Drawing other things")
		ld.Dot = fixed.Point26_6{
			X: fixed.I(cX) + dd.MeasureString(val)/2,
			Y: fixed.I(cY - (dfh / 2)),
		}
		ld.DrawString("°C")

		margin := 8
		nY := y + h - margin
		nX := x + w - margin

		ageWidth := sd.MeasureString(m.FormatAge())
		if ageWidth.Ceil() > 1 {
			rect := image.Rect(nX-ageWidth.Ceil()-margin, nY-sfh, nX+margin, nY+margin)
			draw.Draw(rgba, rect, fg, image.Point{0, 0}, draw.Src)

			wsd.Dot = fixed.Point26_6{
				X: fixed.I(nX) - ageWidth,
				Y: fixed.I(nY),
			}
			wsd.DrawString(m.FormatAge())

			nX = nX - ageWidth.Ceil() - margin - (margin + 1)
		}

		slopeWidth := sd.MeasureString(m.FormatSlope())
		if slopeWidth.Ceil() > 1 {
			rect := image.Rect(nX-slopeWidth.Ceil()-margin, nY-sfh, nX+margin, nY+margin)
			draw.Draw(rgba, rect, fg, image.Point{0, 0}, draw.Src)

			wsd.Dot = fixed.Point26_6{
				X: fixed.I(nX) - slopeWidth,
				Y: fixed.I(nY),
			}
			wsd.DrawString(m.FormatSlope())
		}

	}
}
