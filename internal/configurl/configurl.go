package configurl

import (
	"regexp"
	"strings"
)

const (
	DataOSXName    = "DataOSX"
	CommonDataName = "CommonData"
	team17HTTP     = "http://www.team17.com"
	team17HTTPS    = "https://www.team17.com"
	analyticsHTTP  = "http://www.google-analytics.com"
	analyticsHTTPS = "https://www.google-analytics.com"
)

var (
	dataOSXFiles = []string{
		"SteamConfig.txt",
		"SteamConfigDemo.txt",
		"GOGConfig.txt",
		"PcLanConfig.txt",
		"SwitchConfig.txt",
		"SwitchConfigGOG.txt",
	}
	commonDataFiles = []string{
		"AnalyticsConfig.txt",
		"HttpConfig.txt",
	}
	internalURL = regexp.MustCompile(`(?m)^([ \t]*URL_Internal.*xom\.team17\.com.*)$`)
	mainURL     = regexp.MustCompile(`MainUrl[[:space:]]*=[[:space:]]*"http://`)
)

func DataOSX() []string {
	return append([]string(nil), dataOSXFiles...)
}

func CommonData() []string {
	return append([]string(nil), commonDataFiles...)
}

func Rewrite(content string) (string, int) {
	steps := []func(string) string{
		func(c string) string { return strings.ReplaceAll(c, team17HTTP, team17HTTPS) },
		func(c string) string { return strings.ReplaceAll(c, analyticsHTTP, analyticsHTTPS) },
		func(c string) string { return mainURL.ReplaceAllString(c, `MainUrl = "https://`) },
		func(c string) string {
			return internalURL.ReplaceAllStringFunc(c, func(line string) string {
				trim := strings.TrimLeft(line, " \t")
				if strings.HasPrefix(trim, "//") {
					return line
				}
				indent := line[:len(line)-len(trim)]
				return indent + "// DISABLED: " + trim
			})
		},
	}
	n := 0
	for _, step := range steps {
		if next := step(content); next != content {
			n++
			content = next
		}
	}
	return content, n
}
