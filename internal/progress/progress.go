package progress

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

const (
	updates    = 4
	kibibyte   = 1024
	digitGroup = 3
)

func Begin(doing string) func(done string) {
	slog.Info(doing)
	start := time.Now()
	return func(done string) {
		slog.Info(fmt.Sprintf("%s in %s", done, Duration(time.Since(start))))
	}
}

type Counter struct {
	label string
	total int
	done  int
	mark  int
	next  int
}

func Count(label string, total int) *Counter {
	return &Counter{label: label, total: total, mark: 1, next: quarter(total, 1)}
}

func (c *Counter) Add(n int) {
	c.done += n
	if c.total <= 0 || c.done < c.next {
		return
	}
	slog.Info(fmt.Sprintf("%s: %s/%s", c.label, Number(min(c.done, c.total)), Number(c.total)))
	for c.mark < updates && c.next <= c.done {
		c.mark++
		c.next = quarter(c.total, c.mark)
	}
	if c.next <= c.done {
		c.next = c.total + 1
	}
}

func quarter(total, mark int) int {
	return (total*mark + updates - 1) / updates
}

func Number(n int) string {
	digits := strconv.Itoa(n)
	var b strings.Builder
	for i, r := range digits {
		if i > 0 && (len(digits)-i)%digitGroup == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func Bytes(n int64) string {
	if n < kibibyte {
		return fmt.Sprintf("%d B", n)
	}
	value := float64(n)
	unit := 0
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	for value /= kibibyte; value >= kibibyte && unit < len(units)-1; unit++ {
		value /= kibibyte
	}
	return fmt.Sprintf("%.1f %s", value, units[unit])
}

func Duration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%d ms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1f s", d.Seconds())
}
