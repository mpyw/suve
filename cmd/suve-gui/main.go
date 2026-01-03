//go:build production || dev

package main

import (
	"log"

	"github.com/mpyw/suve/internal/gui"
)

func main() {
	if err := gui.Run(); err != nil {
		log.Fatal("Error: ", err.Error())
	}
}
