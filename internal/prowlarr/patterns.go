package prowlarr

import "regexp"

var (
	garbageRe = regexp.MustCompile(`(?i)hdts|ts|tc|telecine|telesync|screener|scr|webscreener`)
	res4kRe   = regexp.MustCompile(`(?i)2160p|4k|uhd`)
	res1080Re = regexp.MustCompile(`(?i)1080p`)
	res720Re  = regexp.MustCompile(`(?i)720p`)
)
