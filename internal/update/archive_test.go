package update

import "testing"

func TestArchiveURL(t *testing.T) {
	got := ArchiveURL("terraform", "1.9.8", "darwin", "arm64")
	want := "https://releases.hashicorp.com/terraform/1.9.8/terraform_1.9.8_darwin_arm64.zip"

	if got != want {
		t.Errorf("ArchiveURL = %q, want %q", got, want)
	}
}

func TestParseSHASums(t *testing.T) {
	const contents = `be591e8c59c49d0cfbc7664d24910a4b43840b89d0a4bbca662149bbf0397e91  terraform_1.9.8_darwin_amd64.zip
873d7b925d08578fb6bb9c12c7cd92ae73e289e07c360f2fdd69f9036b7baaab  terraform_1.9.8_darwin_arm64.zip
186e0145f5e5f2eb97cbd785bc78f21bae4ef15119349f6ad4fa535b83b10df8 *terraform_1.9.8_linux_amd64.zip

`

	sums, err := ParseSHASums([]byte(contents))
	if err != nil {
		t.Fatalf("ParseSHASums: %v", err)
	}
	if len(sums) != 3 {
		t.Fatalf("parsed %d checksums, want 3", len(sums))
	}

	want := "873d7b925d08578fb6bb9c12c7cd92ae73e289e07c360f2fdd69f9036b7baaab"
	if got := sums["terraform_1.9.8_darwin_arm64.zip"]; got != want {
		t.Errorf("darwin_arm64 = %q, want %q", got, want)
	}
	// Binary-mode lines carry a "*" before the filename.
	if _, ok := sums["terraform_1.9.8_linux_amd64.zip"]; !ok {
		t.Error("binary-mode line was not parsed")
	}
}

func TestParseSHASumsRejectsMalformedInput(t *testing.T) {
	tests := map[string]string{
		"empty":           "",
		"no separator":    "be591e8c59c49d0cfbc7664d24910a4b43840b89d0a4bbca662149bbf0397e91\n",
		"short hash":      "be591e8c  terraform_1.9.8_darwin_amd64.zip\n",
		"non-hex hash":    "ZZ591e8c59c49d0cfbc7664d24910a4b43840b89d0a4bbca662149bbf0397e91  terraform.zip\n",
		"no filename":     "be591e8c59c49d0cfbc7664d24910a4b43840b89d0a4bbca662149bbf0397e91  \n",
		"HTML error page": "<!DOCTYPE html><html><body>404</body></html>\n",
	}

	for name, contents := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseSHASums([]byte(contents)); err == nil {
				t.Error("expected an error")
			}
		})
	}
}
