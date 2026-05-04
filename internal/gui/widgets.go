package gui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
)

var textPaneBg = color.NRGBA{R: 0xe2, G: 0xe2, B: 0xe6, A: 0xff}

func withTextPaneBackground(obj fyne.CanvasObject) fyne.CanvasObject {
	return container.NewStack(canvas.NewRectangle(textPaneBg), obj)
}
