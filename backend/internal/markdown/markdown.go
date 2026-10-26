// Package markdown turns the coach's Markdown answers into safe HTML.
//
// Two libraries, two jobs:
//
//  1. goldmark converts Markdown to HTML (**bold**, lists, tables, ...).
//     Raw HTML inside the Markdown is NOT passed through (goldmark's default).
//  2. bluemonday cleans the result with an allow-list: only harmless tags and
//     attributes survive. No <script>, no onclick=, no javascript: links.
//
// Why both? The model's output is untrusted - a user could trick it into
// writing HTML. goldmark alone already drops raw HTML, but "defence in depth"
// means a second, independent layer catches anything the first one misses.
// The Content-Security-Policy header is the third layer.
package markdown

import (
	"bytes"
	"regexp"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// converter and policy are created once and reused: both are safe to use
// from many goroutines at the same time.
var (
	converter = goldmark.New(
		// The GitHub Flavored Markdown extras that models like to use.
		goldmark.WithExtensions(
			// Tables. Column alignment as align="right" instead of the
			// default style="text-align:right": inline styles are removed by
			// the sanitizer and blocked by our Content-Security-Policy.
			extension.NewTable(extension.WithTableCellAlignMethod(extension.TableCellAlignAttribute)),
			extension.Strikethrough, // ~~text~~
			extension.Linkify,       // bare https://... becomes a link
			extension.TaskList,      // - [x] done
		),
	)

	policy = newPolicy()
)

// newPolicy lists exactly which tags and attributes may appear in an answer.
// Everything else is removed. An explicit list is easier to review than a
// preset - and it leaves out <img>, so answers can't load images from other
// websites (which could be used to track readers).
func newPolicy() *bluemonday.Policy {
	p := bluemonday.NewPolicy()

	p.AllowElements(
		"p", "br", "hr", "h1", "h2", "h3", "h4", "h5", "h6",
		"strong", "em", "b", "i", "del", "s", "blockquote",
		"ul", "ol", "li", "code", "pre",
		"table", "thead", "tbody", "tr", "th", "td",
	)
	p.AllowAttrs("start").Matching(regexp.MustCompile(`^[0-9]+$`)).OnElements("ol")
	p.AllowAttrs("align").Matching(regexp.MustCompile(`^(left|right|center)$`)).OnElements("th", "td")
	p.AllowAttrs("class").Matching(regexp.MustCompile(`^language-[a-zA-Z0-9_-]+$`)).OnElements("code")

	// Links: only http, https and mailto (no javascript:, data:, ...),
	// always rel="nofollow", and outside links open in a new tab with
	// rel="noopener", so the other site can't control our tab.
	p.AllowAttrs("href").OnElements("a")
	p.AllowStandardURLs() // http/https/mailto + parseable URLs + nofollow
	p.AddTargetBlankToFullyQualifiedLinks(true)

	// Task-list checkboxes ("- [x] done") render as disabled inputs. Only
	// type="checkbox" is allowed, and never a name: the chat history travels
	// in hidden <input name="role"> fields, and an answer must never be able
	// to add fake ones.
	p.AllowElements("input")
	p.AllowAttrs("type").Matching(regexp.MustCompile(`^checkbox$`)).OnElements("input")
	p.AllowAttrs("checked", "disabled").OnElements("input")

	return p
}

// Render converts Markdown to sanitized HTML. It never fails: if goldmark
// returns an error (it practically doesn't for in-memory text), the cleaned
// partial output is still returned.
func Render(src string) string {
	var buf bytes.Buffer
	_ = converter.Convert([]byte(src), &buf)
	return policy.Sanitize(buf.String())
}
