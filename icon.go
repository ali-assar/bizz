package main

import (
	"bytes"
	"image"
	"image/png"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

// rasterizeSVG turns an SVG into a PNG resource. Fyne's built-in SVG loader
// only paints simple shapes, so complex icons (ellipses, paths) come out as a
// flat colour block without this step.
func rasterizeSVG(name string, svg []byte, size int) fyne.Resource {
	icon, err := oksvg.ReadIconStream(bytes.NewReader(svg))
	if err != nil {
		return fyne.NewStaticResource(name, svg)
	}
	icon.SetTarget(0, 0, float64(size), float64(size))

	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	scanner := rasterx.NewScannerGV(size, size, img, img.Bounds())
	raster := rasterx.NewDasher(size, size, scanner)
	icon.Draw(raster, 1)

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return fyne.NewStaticResource(name, svg)
	}
	return fyne.NewStaticResource(name, buf.Bytes())
}

// tappableIcon shows the bee without a themed button frame on top of it.
type tappableIcon struct {
	widget.BaseWidget
	img   *canvas.Image
	onTap func()
}

func newTappableIcon(res fyne.Resource, size float32, onTap func()) *tappableIcon {
	t := &tappableIcon{onTap: onTap}
	t.img = canvas.NewImageFromResource(res)
	t.img.FillMode = canvas.ImageFillContain
	t.img.SetMinSize(fyne.NewSize(size, size))
	t.ExtendBaseWidget(t)
	return t
}

func (t *tappableIcon) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(t.img)
}

func (t *tappableIcon) Tapped(*fyne.PointEvent) {
	if t.onTap != nil {
		t.onTap()
	}
}

func (t *tappableIcon) TappedSecondary(*fyne.PointEvent) {}
