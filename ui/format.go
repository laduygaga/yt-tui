package ui

import (
	"strconv"
	"strings"
)

func formatTime(seconds float64) string {
	s := int(seconds)
	h := s / 3600
	m := (s % 3600) / 60
	s = s % 60
	if h > 0 {
		return formatHHMMSS(h, m, s)
	}
	return formatMMSS(m, s)
}

func formatHHMMSS(h, m, s int) string {
	var b strings.Builder
	b.Grow(8)
	writeInt2(&b, h)
	b.WriteByte(':')
	writeInt2(&b, m)
	b.WriteByte(':')
	writeInt2(&b, s)
	return b.String()
}

func formatMMSS(m, s int) string {
	var b strings.Builder
	b.Grow(5)
	writeInt2(&b, m)
	b.WriteByte(':')
	writeInt2(&b, s)
	return b.String()
}

func writeInt2(b *strings.Builder, n int) {
	if n < 10 {
		b.WriteByte('0')
		b.WriteByte(byte('0' + n))
		return
	}
	t := n / 10
	b.WriteByte(byte('0' + t))
	b.WriteByte(byte('0' + (n - t*10)))
}

func formatDuration(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "NA" {
		return raw
	}
	seconds, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return raw
	}
	return formatTime(seconds)
}

func formatViews(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "NA" {
		return raw
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		return raw
	}
	negative := n < 0
	if negative {
		n = -n
	}
	s := strconv.FormatInt(n, 10)
	pre := len(s) % 3
	var b strings.Builder
	b.Grow(len(s) + len(s)/3)
	if pre > 0 {
		b.WriteString(s[:pre])
		if len(s) > pre {
			b.WriteByte(',')
		}
	}
	for i := pre; i < len(s); i += 3 {
		b.WriteString(s[i : i+3])
		if i+3 < len(s) {
			b.WriteByte(',')
		}
	}
	if negative {
		return "-" + b.String()
	}
	return b.String()
}
