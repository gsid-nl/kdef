package lsp

import "testing"

func TestIsVariableFile(t *testing.T) {
	cases := map[string]struct {
		src  string
		want bool
	}{
		"variable block": {
			src: `
variable "sites" {
  type    = "list"
  default = [{ name = "a" }]
}
`,
			want: true,
		},
		"imports only": {
			src:  `import = ["defaults.kdef"]`,
			want: false,
		},
		"definition file": {
			src: `
deployment "web" {
  container "web" {
    image = "nginx"
  }
}
`,
			want: false,
		},
		"syntax error": {
			src:  `variable "sites" {`,
			want: false,
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := isVariableFile([]byte(tc.src), "sites.kdef"); got != tc.want {
				t.Errorf("isVariableFile = %v, want %v", got, tc.want)
			}
		})
	}
}

// A variable file that is not called vars.kdef must not be checked against the
// definition schema — that produced "Blocks of type \"variable\" are not
// expected here" on every vars/*.kdef file.
func TestDiagnoseFile_ImportedVariableFile(t *testing.T) {
	s := &Server{documents: NewDocumentStore()}
	src := `
variable "sites" {
  type = "list"

  default = [
    { name = "cor-it-nl", hosts = ["cor-it.nl", "www.cor-it.nl"] },
  ]
}
`
	diags := s.diagnoseFile("file:///project/vars/sites.kdef", src)
	if len(diags) != 0 {
		t.Fatalf("expected no diagnostics, got %+v", diags)
	}
}
