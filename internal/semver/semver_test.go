package semver

import (
	"sort"
	"testing"
)

func TestCompare(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "1.0.1", -1},
		{"1.0.1", "1.0.0", 1},

		// Numeric fields, not lexicographic ones.
		{"1.9.8", "1.10.0", -1},
		{"1.9.8", "1.16.0", -1},
		{"0.100.0", "0.99.0", 1},

		// Missing fields read as zero.
		{"1.2", "1.2.0", 0},
		{"1", "1.0.0", 0},
		{"1.2", "1.2.1", -1},

		// A leading v is tolerated so that tags and versions compare.
		{"v1.2.3", "1.2.3", 0},

		// Build metadata has no bearing on precedence.
		{"1.0.0+ent", "1.0.0", 0},
		{"1.0.0+ent.hsm", "1.0.0+ent", 0},

		// A release outranks its own prereleases.
		{"1.16.0", "1.16.0-beta1", 1},
		{"1.16.0-beta1", "1.16.0", -1},

		// Prerelease identifiers compare field by field.
		{"1.16.0-alpha20260708", "1.16.0-alpha20260715", -1},
		{"1.16.0-alpha1", "1.16.0-beta1", -1},
		{"1.16.0-beta1", "1.16.0-rc1", -1},
		{"1.16.0-rc1", "1.16.0-rc2", -1},
		{"1.1.0-alpha.1", "1.1.0-alpha.2", -1},

		// Numeric prerelease identifiers rank below alphanumeric ones, and a
		// longer run of equal identifiers outranks a shorter one.
		{"1.0.0-1", "1.0.0-alpha", -1},
		{"1.0.0-alpha", "1.0.0-alpha.1", -1},

		// A prerelease of a lower release still loses to the higher one.
		{"1.15.8", "1.16.0-beta1", -1},
	}

	for _, test := range tests {
		if got := Compare(test.a, test.b); got != test.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", test.a, test.b, got, test.want)
		}
		if got := Compare(test.b, test.a); got != -test.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", test.b, test.a, got, -test.want)
		}
	}
}

func TestIsPrerelease(t *testing.T) {
	tests := map[string]bool{
		"1.0.0":                false,
		"1.0.0+ent":            false,
		"1.16.0-beta1":         true,
		"1.16.0-alpha20260715": true,
		"1.1.0-alpha.1":        true,
	}

	for version, want := range tests {
		if got := IsPrerelease(version); got != want {
			t.Errorf("IsPrerelease(%q) = %t, want %t", version, got, want)
		}
	}
}

func TestAtLeast(t *testing.T) {
	tests := []struct {
		version, min string
		want         bool
	}{
		{"1.6.0", "1.6.0", true},
		{"1.6.1", "1.6.0", true},
		{"1.5.7", "1.6.0", false},
		{"1.6.0-rc1", "1.6.0", false},
		{"1.10.0", "1.9.0", true},
	}

	for _, test := range tests {
		if got := AtLeast(test.version, test.min); got != test.want {
			t.Errorf("AtLeast(%q, %q) = %t, want %t", test.version, test.min, got, test.want)
		}
	}
}

// Sorting is how the updater derives the manifest's version order, so the
// comparison has to yield a sensible total order over a realistic set.
func TestSortsRealReleaseSeries(t *testing.T) {
	versions := []string{
		"1.16.0-beta1",
		"1.15.8",
		"1.16.0-alpha20260715",
		"0.11.14",
		"1.9.8",
		"1.10.0",
		"1.16.0-alpha20260708",
		"1.0.0",
	}

	sort.Slice(versions, func(i, j int) bool { return Less(versions[i], versions[j]) })

	want := []string{
		"0.11.14",
		"1.0.0",
		"1.9.8",
		"1.10.0",
		"1.15.8",
		"1.16.0-alpha20260708",
		"1.16.0-alpha20260715",
		"1.16.0-beta1",
	}

	for i := range want {
		if versions[i] != want[i] {
			t.Fatalf("sorted = %v, want %v", versions, want)
		}
	}
}
