// Package presentation shares literal terminal rendering across native commands.
package presentation

import (
	"io"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

type Palette struct {
	Reset, Bold, Dim, Yellow, Green, Red string
}

// Terminal observes the actual output descriptor, including redirected output.
// docs/adr/0099-native-factory-orchestration.md:139.
func Terminal(writer io.Writer) bool {
	file, ok := writer.(*os.File)
	if !ok {
		return false
	}
	_, err := unix.IoctlGetWinsize(int(file.Fd()), unix.TIOCGWINSZ)
	return err == nil
}

// PaletteFor retains the explicit caller terminal decision across capture.
// docs/adr/0099-native-factory-orchestration.md:145.
func PaletteFor(terminal bool, noColor string) Palette {
	if !terminal || noColor != "" {
		return Palette{}
	}
	return Palette{"\x1b[0m", "\x1b[1m", "\x1b[2m", "\x1b[33m", "\x1b[32m", "\x1b[31m"}
}

// Box renders caller-provided palette, title and line bytes literally.
// docs/adr/0099-native-factory-orchestration.md:140.
func Box(palette Palette, title string, lines []string) string {
	var b strings.Builder
	b.WriteString(palette.Yellow + palette.Bold + "┌─ " + title + " " + palette.Reset + "\n")
	for _, line := range lines {
		b.WriteString(palette.Yellow + "│" + palette.Reset + " " + line + "\n")
	}
	b.WriteString(palette.Yellow + "└─" + palette.Reset + "\n")
	return b.String()
}
