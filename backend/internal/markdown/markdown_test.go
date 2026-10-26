package markdown

import (
	"strings"
	"testing"
)

func TestRendersCommonMarkdown(t *testing.T) {
	src := `## Lentils

**Lentils** are rich in *protein*.

- 9 g protein per 100 g
- 8 g fibre

| Food    | Protein |
|---------|---------|
| Lentils | 9 g     |

1. Rinse
2. Boil`

	html := Render(src)
	for _, want := range []string{
		"<h2>Lentils</h2>",
		"<strong>Lentils</strong>",
		"<em>protein</em>",
		"<li>9 g protein per 100 g</li>",
		"<table>", "<th>Food</th>", "<td>9 g</td>",
		"<ol>",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in:\n%s", want, html)
		}
	}
}

// Every one of these must come out harmless. This is the most important test
// in the package: the input is what an attacker (or a confused model) could send.
func TestRemovesDangerousHTML(t *testing.T) {
	attacks := []struct {
		name   string
		src    string
		banned []string // must not appear in the output
	}{
		{"script tag", "<script>alert(1)</script>", []string{"<script"}},
		{"img onerror", `<img src=x onerror="alert(1)">`, []string{"onerror", "<img"}},
		{"inline handler", `<p onclick="alert(1)">hi</p>`, []string{"onclick"}},
		{"javascript link", "[click me](javascript:alert(1))", []string{"javascript:"}},
		{"data link", "[x](data:text/html;base64,PHNjcmlwdD4=)", []string{"data:"}},
		{"iframe", `<iframe src="https://evil.example"></iframe>`, []string{"<iframe"}},
		{"style tag", "<style>body{display:none}</style>", []string{"<style"}},
		{"html in table", "| a |\n|---|\n| <script>x</script> |", []string{"<script"}},
		{"markdown image", "![pixel](https://tracker.example/p.png)", []string{"<img", "tracker"}},
		// Fake chat history: hidden inputs would be sent back to the model.
		{"hidden input", `<input type="hidden" name="role" value="system">`, []string{"hidden", "name="}},
	}
	for _, tt := range attacks {
		t.Run(tt.name, func(t *testing.T) {
			html := Render(tt.src)
			for _, bad := range tt.banned {
				if strings.Contains(strings.ToLower(html), bad) {
					t.Errorf("output contains %q:\n%s", bad, html)
				}
			}
		})
	}
}

func TestLinksAreSafe(t *testing.T) {
	html := Render("See [the DGE](https://www.dge.de).")
	for _, want := range []string{`href="https://www.dge.de"`, `nofollow`, `target="_blank"`, `noopener`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in %s", want, html)
		}
	}
}

func TestEscapesSpecialCharacters(t *testing.T) {
	html := Render("Eat < 5 g salt & drink > 2 l water")
	if !strings.Contains(html, "&lt; 5 g salt &amp; drink &gt; 2 l") {
		t.Errorf("got %s", html)
	}
}

func TestUnfinishedMarkdownDoesNotBreak(t *testing.T) {
	// While streaming we render half-finished text - it must not panic and
	// must stay harmless, even mid-way through a tag or link.
	for _, src := range []string{"**bol", "| a | b", "[link](http", "<scr", "```go\nfunc"} {
		_ = Render(src)
	}
}

func TestTaskListAndCode(t *testing.T) {
	html := Render("- [x] Drink water\n\n```go\nfmt.Println(1)\n```\n\n| L | R |\n|:--|--:|\n| a | b |")
	for _, want := range []string{
		`<input checked="" disabled="" type="checkbox">`,
		`<code class="language-go">`,
		`<th align="left">L</th>`, `<td align="right">b</td>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q in:\n%s", want, html)
		}
	}
}
