package update

import "testing"

func TestDetectLicense(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		want     string
		found    bool
	}{
		{
			name: "BUSL as HashiCorp publishes it",
			contents: "License text copyright (c) 2020 MariaDB Corporation Ab, All Rights Reserved.\n" +
				"\"Business Source License\" is a trademark of MariaDB Corporation Ab.\n",
			want:  "bsl11",
			found: true,
		},
		{
			name:     "MPL with a HashiCorp copyright line",
			contents: "Copyright (c) 2014 HashiCorp, Inc.\n\nMozilla Public License, version 2.0\n",
			want:     "mpl20",
			found:    true,
		},
		{
			name:     "MPL with an IBM copyright line",
			contents: "Copyright IBM Corp. 2020, 2026\n\nMozilla Public License, version 2.0\n",
			want:     "mpl20",
			found:    true,
		},
		{
			name:     "unrecognised",
			contents: "All rights reserved. Use of this software requires a license.\n",
			found:    false,
		},
		{
			name:     "a 404 body is not a licence",
			contents: "404: Not Found",
			found:    false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, found := DetectLicense([]byte(test.contents))
			if found != test.found {
				t.Fatalf("found = %t, want %t", found, test.found)
			}
			if found && got != test.want {
				t.Errorf("license = %q, want %q", got, test.want)
			}
		})
	}
}

func TestRawURL(t *testing.T) {
	got, err := rawURL("https://github.com/hashicorp/terraform", "v1.9.8", "LICENSE")
	if err != nil {
		t.Fatalf("rawURL: %v", err)
	}

	want := "https://raw.githubusercontent.com/hashicorp/terraform/v1.9.8/LICENSE"
	if got != want {
		t.Errorf("rawURL = %q, want %q", got, want)
	}

	if _, err := rawURL("https://gitlab.com/hashicorp/terraform", "v1.9.8", "LICENSE"); err == nil {
		t.Error("expected an error for a non-GitHub repository")
	}
}
