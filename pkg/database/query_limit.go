package database

import "regexp"

// limitRegex matches a trailing LIMIT [OFFSET] clause in a SELECT. It is shared
// by every driver's executeSelect and compiled once at package load — compiling
// it per query (the previous behavior) wasted CPU/allocations on the hottest
// path, since this runs for every SELECT across all engines.
var limitRegex = regexp.MustCompile(`(?i)\s+LIMIT\s+(\d+)(?:\s+OFFSET\s+(\d+))?`)
