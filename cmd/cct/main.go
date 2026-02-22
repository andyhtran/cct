//go:build darwin || linux

package main

import (
	"os"

	"github.com/andyhtran/cct/internal/app"
)

var version = "dev"

func main() {
	os.Exit(app.Run(version))
}
