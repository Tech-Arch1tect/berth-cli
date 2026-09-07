package output

import "os"

const (
	ansiReset = "\x1b[0m"
	ansiGreen = "\x1b[32m"
	ansiRed   = "\x1b[31m"
)

type ColourFunc func(value any) (string, bool)

func ColourEnabled(isTTY bool, noColourFlag bool, noColourEnv string) bool {
	return isTTY && !noColourFlag && noColourEnv == ""
}

func ColourSupported(noColourFlag bool) bool {
	info, err := os.Stdout.Stat()
	isTTY := err == nil && info.Mode()&os.ModeCharDevice != 0
	return ColourEnabled(isTTY, noColourFlag, os.Getenv("NO_COLOR"))
}

func BoolColour(value any) (string, bool) {
	active, ok := value.(bool)
	if !ok {
		return "", false
	}
	if active {
		return ansiGreen, true
	}
	return ansiRed, true
}

func colourise(value string, code string, enabled bool) string {
	if !enabled || code == "" {
		return value
	}
	return code + value + ansiReset
}
