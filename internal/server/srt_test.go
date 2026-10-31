package server

import "testing"

func TestVTTBecomesSRT(t *testing.T) {
	in := "WEBVTT\nKind: captions\n\nNOTE made up\n\nSTYLE\n::cue { color: lime }\n\n1\n00:00:01.000 --> 00:00:03.500 align:start position:10%\nHello\n<i>there</i>\n\n02:04.250 --> 02:06.000\nShort form\r\n\r\ncue-id\n1:02:03.004 --> 1:02:05.000\nOne-digit hour\n\n00:00:09.000 --> 00:00:10.000\n\n"
	want := "1\n00:00:01,000 --> 00:00:03,500\nHello\n<i>there</i>\n\n" +
		"2\n00:02:04,250 --> 00:02:06,000\nShort form\n\n" +
		"3\n01:02:03,004 --> 01:02:05,000\nOne-digit hour\n\n"
	if got := vttToSRT(in); got != want {
		t.Fatalf("got\n%q\nwant\n%q", got, want)
	}
}
