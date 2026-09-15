package components

import "strings"

// maskBullet is the character a masked value is rendered with. A masked line is
// a run of these, so a revealed value never reaches the screen (or a golden).
const maskBullet = "•"

// maxMaskWidth caps how many bullets a masked line shows, so a very long secret
// does not paint an enormous bar (and its length is not leaked verbatim).
const maxMaskWidth = 24

// MaskValue masks a (possibly multi-line) value: each line becomes a run of
// bullets capped at maxMaskWidth, so neither the content nor (beyond the cap)
// the length reaches the screen. Shared by the value pane and the diff page so a
// secret diff is masked identically on both sides.
func MaskValue(raw string) string {
	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		lines[i] = strings.Repeat(maskBullet, maskWidth(line))
	}

	return strings.Join(lines, "\n")
}

// maskWidth returns the bullet count for a masked line: the rune length capped
// at maxMaskWidth, and at least one bullet for a non-empty line so an emptyish
// value still reads as "present".
func maskWidth(line string) int {
	n := len([]rune(line))
	if n == 0 {
		return 0
	}

	return min(n, maxMaskWidth)
}
