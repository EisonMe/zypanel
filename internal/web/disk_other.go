//go:build !unix

package web

// readDisk is not implemented on this platform; the dashboard hides the card.
func readDisk(path string) (usedGB, totalGB int, pct int, ok bool) {
	return 0, 0, 0, false
}
