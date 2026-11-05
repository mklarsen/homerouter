package main

import (
	"fmt"
	"strings"

	"rsc.io/qr"
)

// qrSVG renders text as a self-contained SVG QR code with a quiet zone.
func qrSVG(text string) (string, error) {
	code, err := qr.Encode(text, qr.M)
	if err != nil {
		return "", err
	}
	const quiet = 4
	size := code.Size + 2*quiet

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" shape-rendering="crispEdges">`, size, size)
	b.WriteString(`<rect width="100%" height="100%" fill="#fff"/><path fill="#000" d="`)
	for y := 0; y < code.Size; y++ {
		for x := 0; x < code.Size; x++ {
			if code.Black(x, y) {
				fmt.Fprintf(&b, "M%d %dh1v1h-1z", x+quiet, y+quiet)
			}
		}
	}
	b.WriteString(`"/></svg>`)
	return b.String(), nil
}
