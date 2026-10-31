package server

import (
	"regexp"
	"strconv"
	"strings"
)

// vttTime matches a WebVTT cue timing line: hours are optional in WebVTT,
// required in SRT; the decimal point becomes a comma; cue settings after the
// end time (position, align) have no SRT form and are dropped.
var vttTime = regexp.MustCompile(`^((?:\d+:)?\d{2}:\d{2})\.(\d{3})\s+-->\s+((?:\d+:)?\d{2}:\d{2})\.(\d{3})`)

func srtTime(hms, ms string) string {
	if strings.Count(hms, ":") == 1 {
		hms = "00:" + hms
	}
	if i := strings.Index(hms, ":"); i == 1 {
		hms = "0" + hms
	}
	return hms + "," + ms
}

// vttToSRT turns the WebVTT Movies and Jellyfin serve (Stream.vtt) into SRT,
// the format DLNA TVs take for external subtitles (roadmap 16). Headers,
// NOTE, STYLE and REGION blocks are dropped; cues are numbered from 1.
func vttToSRT(vtt string) string {
	vtt = strings.ReplaceAll(strings.ReplaceAll(vtt, "\r\n", "\n"), "\r", "\n")
	var b strings.Builder
	n := 0
	for _, block := range strings.Split(vtt, "\n\n") {
		lines := strings.Split(strings.Trim(block, "\n"), "\n")
		for i, l := range lines {
			m := vttTime.FindStringSubmatch(strings.TrimSpace(l))
			if m == nil {
				continue
			}
			text := strings.TrimSpace(strings.Join(lines[i+1:], "\n"))
			if text == "" {
				break
			}
			n++
			b.WriteString(strconv.Itoa(n) + "\n" + srtTime(m[1], m[2]) + " --> " + srtTime(m[3], m[4]) + "\n" + text + "\n\n")
			break
		}
	}
	return b.String()
}
