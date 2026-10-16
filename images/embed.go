package images

import (
	"embed"
	"io/fs"
)

//go:embed php
var embedded embed.FS

func PHP() fs.FS {
	sub, err := fs.Sub(embedded, "php")
	if err != nil {
		panic(err)
	}
	return sub
}
