// syncicon copies assets/beez-icon.png into the embedded assets package.
package main

import (
	"io"
	"os"
	"path/filepath"
)

func main() {
	if err := copyFile("assets/beez-icon.png", "internal/beez/assets/beez-icon.png"); err != nil {
		panic(err)
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
