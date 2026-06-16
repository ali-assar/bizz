// png2ico converts assets/beez-icon.png to assets/beez-icon.ico for Windows windres.
package main

import (
	"image"
	_ "image/png"
	"os"

	"github.com/fyne-io/image/ico"
)

const (
	pngPath = "assets/beez-icon.png"
	icoPath = "assets/beez-icon.ico"
)

func main() {
	f, err := os.Open(pngPath)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		panic(err)
	}

	out, err := os.Create(icoPath)
	if err != nil {
		panic(err)
	}
	defer out.Close()

	if err := ico.Encode(out, img); err != nil {
		panic(err)
	}
}
