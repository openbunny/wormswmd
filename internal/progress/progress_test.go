package progress

import (
	"bytes"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"
)

func capture(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey || a.Key == slog.LevelKey {
				return slog.Attr{}
			}
			return a
		},
	})))
	return &buf
}

func TestCounterLogsQuarters(t *testing.T) {
	cases := []struct {
		name  string
		total int
		adds  []int
		want  []string
	}{
		{name: "eight", total: 8, adds: []int{1, 1, 1, 1, 1, 1, 1, 1}, want: []string{"files: 2/8", "files: 4/8", "files: 6/8", "files: 8/8"}},
		{name: "two", total: 2, adds: []int{1, 1}, want: []string{"files: 1/2", "files: 2/2"}},
		{name: "jump", total: 8, adds: []int{8}, want: []string{"files: 8/8"}},
		{name: "uneven", total: 46, adds: slices.Repeat([]int{1}, 46), want: []string{"files: 12/46", "files: 23/46", "files: 35/46", "files: 46/46"}},
		{name: "grouped", total: 4824, adds: []int{4824}, want: []string{"files: 4,824/4,824"}},
		{name: "empty", total: 0, adds: []int{1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			buf := capture(t)
			c := Count("files", tc.total)
			for _, n := range tc.adds {
				c.Add(n)
			}
			var got []string
			for line := range strings.SplitSeq(strings.TrimSpace(buf.String()), "\n") {
				if line != "" {
					got = append(got, strings.Trim(strings.TrimPrefix(line, "msg="), `"`))
				}
			}
			if strings.Join(got, "|") != strings.Join(tc.want, "|") {
				t.Fatalf("lines = %q; want %q", got, tc.want)
			}
		})
	}
}

func TestBegin(t *testing.T) {
	buf := capture(t)
	done := Begin("Backing up the app")
	done("Backed up the app")
	text := buf.String()
	if !strings.Contains(text, "Backing up the app") || !strings.Contains(text, "Backed up the app in ") {
		t.Fatalf("log = %q", text)
	}
}

func TestFormat(t *testing.T) {
	cases := []struct {
		got, want string
	}{
		{Number(7), "7"},
		{Number(4824), "4,824"},
		{Number(1234567), "1,234,567"},
		{Bytes(512), "512 B"},
		{Bytes(1536), "1.5 KiB"},
		{Bytes(1288490188), "1.2 GiB"},
		{Duration(250 * time.Millisecond), "250 ms"},
		{Duration(3100 * time.Millisecond), "3.1 s"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("got %q; want %q", tc.got, tc.want)
		}
	}
}
