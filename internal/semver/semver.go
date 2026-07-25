// Package semver implements just enough of semantic versioning 2.0.0 to order
// HashiCorp release version strings.
//
// The standard library has no semver support, and the releases API returns
// versions in creation order rather than version order, so the updater needs
// its own comparison in order to pick the newest release of a product and to
// resolve license cut-over boundaries.
package semver

import (
	"strconv"
	"strings"
)

// Compare returns -1 if a orders before b, +1 if a orders after b, and 0 if
// they have equal precedence. Build metadata is ignored, as required by the
// specification.
func Compare(a, b string) int {
	aCore, aPre := split(a)
	bCore, bPre := split(b)

	if c := compareCore(aCore, bCore); c != 0 {
		return c
	}

	return comparePrerelease(aPre, bPre)
}

// Less reports whether a orders before b.
func Less(a, b string) bool {
	return Compare(a, b) < 0
}

// AtLeast reports whether v has precedence greater than or equal to min.
func AtLeast(v, min string) bool {
	return Compare(v, min) >= 0
}

// IsPrerelease reports whether v carries a prerelease suffix.
func IsPrerelease(v string) bool {
	_, pre := split(v)
	return pre != ""
}

// split separates a version into its core release identifier and its
// prerelease identifier, discarding any build metadata.
func split(v string) (core, prerelease string) {
	v = strings.TrimPrefix(v, "v")

	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}

	if i := strings.IndexByte(v, '-'); i >= 0 {
		return v[:i], v[i+1:]
	}

	return v, ""
}

// compareCore compares dot-separated release identifiers field by field,
// treating a missing field as zero so that "1.2" and "1.2.0" are equal.
func compareCore(a, b string) int {
	aFields := strings.Split(a, ".")
	bFields := strings.Split(b, ".")

	n := max(len(aFields), len(bFields))
	for i := range n {
		if c := compareCoreField(field(aFields, i), field(bFields, i)); c != 0 {
			return c
		}
	}

	return 0
}

func field(fields []string, i int) string {
	if i < len(fields) {
		return fields[i]
	}
	return "0"
}

func compareCoreField(a, b string) int {
	aNum, aOK := numeric(a)
	bNum, bOK := numeric(b)

	switch {
	case aOK && bOK:
		return cmpInt(aNum, bNum)
	case aOK:
		// A numeric field is more plausible as a real release than a
		// malformed one, and sorts higher so that oddities do not become
		// the "latest" release.
		return 1
	case bOK:
		return -1
	default:
		return strings.Compare(a, b)
	}
}

// comparePrerelease applies the precedence rules for prerelease identifiers: a
// version without a prerelease outranks one with, numeric identifiers compare
// numerically and rank below alphanumeric ones, and a longer run of otherwise
// equal identifiers outranks a shorter one.
func comparePrerelease(a, b string) int {
	switch {
	case a == "" && b == "":
		return 0
	case a == "":
		return 1
	case b == "":
		return -1
	}

	aFields := strings.Split(a, ".")
	bFields := strings.Split(b, ".")

	for i := range min(len(aFields), len(bFields)) {
		if c := comparePrereleaseField(aFields[i], bFields[i]); c != 0 {
			return c
		}
	}

	return cmpInt(len(aFields), len(bFields))
}

func comparePrereleaseField(a, b string) int {
	aNum, aOK := numeric(a)
	bNum, bOK := numeric(b)

	switch {
	case aOK && bOK:
		return cmpInt(aNum, bNum)
	case aOK:
		return -1
	case bOK:
		return 1
	default:
		return strings.Compare(a, b)
	}
}

func numeric(s string) (int, bool) {
	if s == "" {
		return 0, false
	}

	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0, false
	}

	return n, true
}

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	default:
		return 0
	}
}
