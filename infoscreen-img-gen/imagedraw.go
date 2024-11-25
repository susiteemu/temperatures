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
	"time"

	"github.com/disintegration/imaging"
	"github.com/golang/freetype/truetype"
	"github.com/rs/zerolog/log"
	"golang.org/x/image/font"
	"golang.org/x/image/math/fixed"
)

//go:embed resources/*.ttf
var resources embed.FS

//go:embed icons/*.png
var icons embed.FS

var (
	dpi           = float64(72)
	labelFontfile = "resources/BitterPro-Medium.ttf"
	fontfile      = "resources/BitterPro-Bold.ttf"
	spacing       = 1.1
	iconCache     = map[string]image.Image{}
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

func drawResult(measurements []Measurement, weather *Weather, imageConfiguration *GenerateImageConfiguration) {

	imgWidth := imageConfiguration.ImgW
	imgHeight := imageConfiguration.ImgH
	canvasImgWidth := imgWidth - 5
	canvasImgHeight := imgHeight - 5
	fontSizeL := imageConfiguration.FontL
	fontSizeLMinus := imageConfiguration.FontL - 30
	fontSizeM := imageConfiguration.FontM
	fontSizeMPlus := imageConfiguration.FontM + 10
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
	rgba := image.NewRGBA(image.Rect(0, 0, canvasImgWidth, canvasImgHeight))
	draw.Draw(rgba, rgba.Bounds(), bg, image.Point{0, 0}, draw.Src)

	defaultFontHeight := int(math.Ceil(fontSizeL * spacing * dpi / 72))
	defaultHalfCellFontHeight := int(math.Ceil(fontSizeLMinus * spacing * dpi / 72))
	labelFontHeight := int(math.Ceil(fontSizeM * spacing * dpi / 72))
	weatherFontHeight := int(math.Ceil(fontSizeMPlus * spacing * dpi / 72))
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

	weatherDrawer := &font.Drawer{
		Dst: rgba,
		Src: fg,
		Face: truetype.NewFace(defaultFont, &truetype.Options{
			Size:    fontSizeMPlus,
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
	rowHeight := canvasImgHeight / rows
	colWidth := canvasImgWidth / cols

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
			cellH := (canvasImgHeight / 3) * 2
			switch currentSide {
			case LEFT_SIDE:
				cellX = 0
			case RIGHT_SIDE:
				cellX = colWidth
			}

			drawDoubleCell(m, weather, cellX, cellY, cellW, cellH, labelFontHeight, defaultFontHeight, smallFontHeight, weatherFontHeight, labelDrawer, defaultDrawer, smallDrawer, whiteSmallDrawer, weatherDrawer, rgba, fg, imgWidth)

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

		if currentSide != RIGHT_SIDE && currentY+10 >= canvasImgHeight {
			currentSide = RIGHT_SIDE
			currentY = 0
		}

	}
	ruler := image.Black
	for i := 0; i < canvasImgHeight; i++ {
		rgba.Set(canvasImgWidth/2, i, ruler)
	}
	// Left side is divided to two rows: 2/3 and 1/3
	leftSideDivideIdx := 2
	leftSideStop := canvasImgWidth / 2
	for x := 0; x < leftSideStop; x++ {
		rgba.Set(x, leftSideDivideIdx*rowHeight, ruler)
	}
	// Right side is divided to 3x1/3 rows
	rightSideStart := canvasImgWidth / 2
	for y := 0; y < rows; y++ {
		for x := rightSideStart; x < canvasImgWidth; x++ {
			rgba.Set(x, y*rowHeight, ruler)
		}
	}
	// Right side bottom cell is divided to two equal parts
	rightBottomCellX := (canvasImgWidth / 4) * 3
	rightBottomCellY := (canvasImgHeight / 3) * 2
	for i := rightBottomCellY; i < canvasImgHeight; i++ {
		rgba.Set(rightBottomCellX, i, ruler)
	}

	uX := 5
	uM := 5
	currentTime := time.Now()
	formattedTime := currentTime.Format("15:04")
	updatedText := fmt.Sprintf("%s", formattedTime)
	smallDrawer.Dot = fixed.Point26_6{
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

	fRgba := image.NewRGBA(image.Rect(0, 0, imgWidth, imgHeight))
	draw.Draw(fRgba, fRgba.Bounds(), bg, image.Point{0, 0}, draw.Src)

	draw.Draw(fRgba, rgba.Bounds(), rgba, image.Point{0, 0}, draw.Src)

	outFile, err := os.Create(imageConfiguration.Output)
	if err != nil {
		log.Error().Err(err).Msgf("Failed to write to file %s", imageConfiguration.Output)
		os.Exit(1)
	}
	defer outFile.Close()
	b := bufio.NewWriter(outFile)
	err = png.Encode(b, fRgba)
	if err != nil {
		log.Error().Err(err)
		os.Exit(1)
	}
	err = b.Flush()
	if err != nil {
		log.Error().Err(err)
		os.Exit(1)
	}

	/*cmd := exec.Command("convert", imageConfiguration.Output, "-gravity", "center", "-extent", fmt.Sprintf("%dx%d", imgWidth, imgHeight), "-colorspace", "gray", "-depth", "8", "-rotate", "-90", imageConfiguration.Output)
	_, err = cmd.Output()
	if err != nil {
		log.Error().Err(err).Msg("Failed to run 'convert' command")
	}
	*/
	log.Info().Msg("Successfully wrote image")
}

func drawCell(m Measurement, x, y, w, h, lfh, dfh, sfh int, ld, dd, sd, wsd *font.Drawer, rgba *image.RGBA, fg *image.Uniform) {

	// calculate center for grid
	cX := x + (w / 2)
	cY := y + (h / 2) - (lfh / 2) - (dfh / 2)

	label := m.FormatLabel()
	ld.Dot = fixed.Point26_6{
		X: fixed.I(cX) - ld.MeasureString(label)/2,
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

func drawDoubleCell(m Measurement, weather *Weather, x, y, w, h, lfh, dfh, sfh, wfh int, ld, dd, sd, wsd, wd *font.Drawer, rgba *image.RGBA, fg *image.Uniform, imgW int) {

	// calculate center for grid
	cX := x + (w / 2)
	cY := y + (h / 4)

	log.Debug().Msgf("X=%d, Y=%d", x, y)

	val := m.FormatValue()
	tempStrW := dd.MeasureString(val)

	if weather != nil {

		size := "4x"
		if imgW > 800 {
			size = "4x"
		}

		cacheKey := fmt.Sprintf("%s-%s", weather.Icon, size)
		icon, cached := iconCache[cacheKey]
		if !cached {
			var err error
			icon, err = readImg(weather.Icon, size)
			if err == nil {
				iconCache[cacheKey] = icon
			}
		}

		if icon != nil {

			if imgW <= 800 {
				iconW := float64(icon.Bounds().Dx()) * 0.75
				resizedIcon := imaging.Resize(icon, int(iconW), 0, imaging.Lanczos)
				icon = resizedIcon
			}

			bounds := icon.Bounds()
			iY := cY + bounds.Dy()/4
			pos := image.Point{x + 10, iY}
			draw.Draw(rgba, bounds.Add(pos), icon, image.Point{0, 0}, draw.Over)
		}

	}

	dd.Dot = fixed.Point26_6{
		X: fixed.I(cX) - tempStrW/2,
		Y: fixed.I(cY),
	}
	dd.DrawString(val)

	if !m.Empty {
		log.Debug().Msg("Drawing other things")
		ld.Dot = fixed.Point26_6{
			X: fixed.I(cX) + tempStrW/2,
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

	if weather != nil {

		cY += dfh/2 + 20
		for _, h := range weather.Hourly {
			wX := x + (w / 2)
			dt := h.Dt
			hour := dt.Format("15")
			wd.Dot = fixed.Point26_6{
				X: fixed.I(wX),
				Y: fixed.I(cY),
			}
			val := hour
			wd.DrawString(val)

			size := "1x"
			cacheKey := fmt.Sprintf("%s-%s", h.Icon, size)
			icon, cached := iconCache[cacheKey]
			if !cached {
				var err error
				icon, err = readImg(h.Icon, size)
				iconW := float64(icon.Bounds().Dx()) * 1.25
				resizedIcon := imaging.Resize(icon, int(iconW), 0, imaging.Lanczos)
				icon = resizedIcon
				if err == nil {
					iconCache[cacheKey] = icon
				}
			}

			if imgW > 800 {
				wX += 50
			} else {
				wX += 40
			}
			if icon != nil {
				bounds := icon.Bounds()
				pos := image.Point{wX, cY - 43}
				draw.Draw(rgba, bounds.Add(pos), icon, image.Point{0, 0}, draw.Over)
			}

			sectionW := 70
			wX += sectionW
			wd.Dot = fixed.Point26_6{
				X: fixed.I(wX),
				Y: fixed.I(cY),
			}
			val = fmt.Sprintf("%.1f°", h.Temp)
			wd.DrawString(val)

			cY += wfh + 15
		}
	}
}

func readImg(name string, size string) (image.Image, error) {
	// Open the image to overlay (foreground image)
	path := ""
	if size != "1x" {
		path = fmt.Sprintf("icons/%s@%s.png", name, size)
	} else {
		path = fmt.Sprintf("icons/%s.png", name)
	}
	imgFile, err := icons.Open(path)
	if err != nil {
		log.Error().Err(err).Msgf("Error opening image from path %s", path)
		return nil, err
	}
	defer imgFile.Close()

	// Decode the foreground image
	img, _, err := image.Decode(imgFile)
	if err != nil {
		log.Error().Err(err).Msgf("Error decoding image from path %s", path)
		return nil, err
	}
	return img, nil
}
