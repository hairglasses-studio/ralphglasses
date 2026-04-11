package enhancer

import (
	"regexp"
	"strings"
)

var (
	reArticles     = regexp.MustCompile(`(?i)\b(a|an|the)\b\s*`)
	reFiller       = regexp.MustCompile(`(?i)\b(just|really|basically|actually|simply|essentially|generally|kind\s+of|sort\s+of)\b\s*`)
	rePleasantries = regexp.MustCompile(`(?i)\b(sure|certainly|of\s+course|happy\s+to|I'd\s+be\s+happy\s+to|please|thank\s+you|thanks)\b[,.!\s]*`)
	reHedging      = regexp.MustCompile(`(?i)\b(it\s+might\s+be\s+worth|you\s+could\s+consider|it\s+would\s+be\s+good\s+to|I\s+suggest\s+that|I\s+recommend\s+that|maybe|perhaps)\b\s*`)
	reRedundant    = regexp.MustCompile(`(?i)\bin\s+order\s+to\b`)
	reMakeSure     = regexp.MustCompile(`(?i)\bmake\s+sure\s+to\b`)
)

// CompressCaveman applies token-saving compression based on the intensity level.
func CompressCaveman(text, level string) (string, []string) {
	if text == "" || level == "" {
		return text, nil
	}

	var imps []string
	original := text

	switch strings.ToLower(level) {
	case "lite":
		text, imps = compressLite(text)
	case "full":
		text, imps = compressFull(text)
	case "ultra":
		text, imps = compressUltra(text)
	case "wenyan-lite", "wenyan-full", "wenyan-ultra":
		text, imps = compressWenyan(text, level)
	}

	if text != original {
		imps = append(imps, "Applied caveman compression level: "+level)
	}

	return text, imps
}

func compressLite(text string) (string, []string) {
	var imps []string
	
	t := reFiller.ReplaceAllString(text, "")
	if t != text {
		imps = append(imps, "Removed filler words")
		text = t
	}

	t = rePleasantries.ReplaceAllString(text, "")
	if t != text {
		imps = append(imps, "Removed pleasantries")
		text = t
	}

	t = reHedging.ReplaceAllString(text, "")
	if t != text {
		imps = append(imps, "Removed hedging")
		text = t
	}

	return text, imps
}

func compressFull(text string) (string, []string) {
	text, imps := compressLite(text)

	t := reArticles.ReplaceAllString(text, "")
	if t != text {
		imps = append(imps, "Removed articles (a, an, the)")
		text = t
	}

	t = strings.ReplaceAll(text, "  ", " ")
	return strings.TrimSpace(t), imps
}

func compressUltra(text string) (string, []string) {
	text, imps := compressFull(text)

	// Ultra-compression: use arrows for causality and abbreviations
	// Using regex for word boundaries to handle beginning of string and punctuation
	replacements := []struct {
		re  *regexp.Regexp
		rep string
	}{
		{regexp.MustCompile(`(?i)\bbecause\b`), "->"},
		{regexp.MustCompile(`(?i)\btherefore\b`), "->"},
		{regexp.MustCompile(`(?i)\bso\b`), "->"},
		{regexp.MustCompile(`(?i)\band\b`), "+"},
		{regexp.MustCompile(`(?i)\bwith\b`), "w/"},
		{regexp.MustCompile(`(?i)\bfunction\b`), "fn"},
		{regexp.MustCompile(`(?i)\bimplementation\b`), "impl"},
		{regexp.MustCompile(`(?i)\bconfiguration\b`), "cfg"},
		{regexp.MustCompile(`(?i)\bdatabase\b`), "DB"},
		{regexp.MustCompile(`(?i)\bauthentication\b`), "auth"},
		{regexp.MustCompile(`(?i)\brequest\b`), "req"},
		{regexp.MustCompile(`(?i)\bresponse\b`), "res"},
	}

	for _, r := range replacements {
		text = r.re.ReplaceAllString(text, r.rep)
	}

	return strings.TrimSpace(text), imps
}

func compressWenyan(text, level string) (string, []string) {
	// For Wenyan, we use ultra-compression as a base and then apply some classical-style tokens
	text, imps := compressUltra(text)

	// Very basic pseudo-wenyan replacements for flavor and terseness
	replacements := []struct {
		re  *regexp.Regexp
		rep string
	}{
		{regexp.MustCompile(`(?i)\bis\b`), "為"},
		{regexp.MustCompile(`(?i)\bof\b`), "之"},
		{regexp.MustCompile(`(?i)\bcorrectly\b`), "宜"},
		{regexp.MustCompile(`(?i)\bmust\b`), "必"},
		{regexp.MustCompile(`(?i)\bshould\b`), "應"},
		{regexp.MustCompile(`(?i)\bnot\b`), "不"},
	}

	if level == "wenyan-full" || level == "wenyan-ultra" {
		for _, r := range replacements {
			text = r.re.ReplaceAllString(text, r.rep)
		}
	}

	return strings.TrimSpace(text), imps
}
