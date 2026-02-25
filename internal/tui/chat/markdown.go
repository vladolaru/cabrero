package chat

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"

	"github.com/vladolaru/cabrero/internal/tui/shared"
)

// Styles for markdown elements — intentionally minimal (bold + muted only).
var (
	mdBold    = lipgloss.NewStyle().Bold(true)
	mdHeading = lipgloss.NewStyle().Bold(true)
	mdCode    = lipgloss.NewStyle().Foreground(shared.ColorMuted)
)

// renderMarkdown parses markdown and returns styled terminal text
// that fits within the given width. Uses goldmark (pure Go, no unsafe).
func renderMarkdown(source string, width int) string {
	if width < 10 {
		width = 10
	}
	if strings.TrimSpace(source) == "" {
		return ""
	}

	src := []byte(source)
	doc := goldmark.DefaultParser().Parse(text.NewReader(src))

	r := &mdRenderer{src: src, width: width}
	r.walkBlocks(doc)

	return strings.TrimRight(r.buf.String(), "\n")
}

type mdRenderer struct {
	src   []byte
	width int
	buf   strings.Builder
}

// walkBlocks iterates over block-level children of the given node.
func (r *mdRenderer) walkBlocks(node ast.Node) {
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *ast.Heading:
			inline := r.collectInline(n)
			r.buf.WriteString(mdHeading.Render(inline))
			r.buf.WriteString("\n\n")

		case *ast.Paragraph:
			inline := r.collectInline(n)
			r.writeWrapped(inline, 0)
			r.buf.WriteString("\n")

		case *ast.TextBlock:
			inline := r.collectInline(n)
			r.writeWrapped(inline, 0)
			r.buf.WriteString("\n")

		case *ast.List:
			r.renderList(n, 0)
			r.buf.WriteString("\n")

		case *ast.FencedCodeBlock:
			r.renderCodeBlock(n)
			r.buf.WriteString("\n")

		case *ast.CodeBlock:
			r.renderCodeBlock(n)
			r.buf.WriteString("\n")

		case *ast.Blockquote:
			r.renderBlockquote(n)
			r.buf.WriteString("\n")

		case *ast.ThematicBreak:
			w := r.width
			if w > 40 {
				w = 40
			}
			r.buf.WriteString(strings.Repeat("─", w))
			r.buf.WriteString("\n\n")

		case *ast.HTMLBlock:
			r.renderHTMLBlock(n)

		default:
			r.walkBlocks(child)
		}
	}
}

// collectInline collects and styles all inline content under a node.
func (r *mdRenderer) collectInline(node ast.Node) string {
	var b strings.Builder
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		r.renderInline(&b, child)
	}
	return b.String()
}

// renderInline renders a single inline node to the builder.
func (r *mdRenderer) renderInline(b *strings.Builder, node ast.Node) {
	switch n := node.(type) {
	case *ast.Text:
		b.Write(n.Segment.Value(r.src))
		if n.SoftLineBreak() {
			b.WriteByte(' ')
		}
		if n.HardLineBreak() {
			b.WriteByte('\n')
		}

	case *ast.String:
		b.Write(n.Value)

	case *ast.CodeSpan:
		var code strings.Builder
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			if t, ok := c.(*ast.Text); ok {
				code.Write(t.Segment.Value(r.src))
			}
		}
		b.WriteString(mdCode.Render(code.String()))

	case *ast.Emphasis:
		inner := r.collectInline(n)
		if n.Level == 2 {
			b.WriteString(mdBold.Render(inner))
		} else {
			b.WriteString(inner)
		}

	case *ast.Link:
		b.WriteString(r.collectInline(n))

	case *ast.AutoLink:
		b.Write(n.URL(r.src))

	case *ast.Image:
		b.WriteString(r.collectInline(n))

	case *ast.RawHTML:
		if n.Segments != nil {
			for i := 0; i < n.Segments.Len(); i++ {
				seg := n.Segments.At(i)
				b.Write(seg.Value(r.src))
			}
		}

	default:
		// Unknown inline — render children.
		for c := node.FirstChild(); c != nil; c = c.NextSibling() {
			r.renderInline(b, c)
		}
	}
}

// writeWrapped word-wraps text at the available width and writes with indent.
func (r *mdRenderer) writeWrapped(s string, indent int) {
	w := r.width - indent
	if w < 10 {
		w = 10
	}
	wrapped := lipgloss.NewStyle().Width(w).Render(s)
	if indent > 0 {
		pad := strings.Repeat(" ", indent)
		for i, line := range strings.Split(wrapped, "\n") {
			if i > 0 {
				r.buf.WriteByte('\n')
			}
			r.buf.WriteString(pad + line)
		}
	} else {
		r.buf.WriteString(wrapped)
	}
}

// renderList renders an ordered or unordered list.
func (r *mdRenderer) renderList(list *ast.List, indent int) {
	idx := list.Start
	if idx == 0 {
		idx = 1
	}

	for item := list.FirstChild(); item != nil; item = item.NextSibling() {
		li, ok := item.(*ast.ListItem)
		if !ok {
			continue
		}

		var prefix string
		if list.IsOrdered() {
			prefix = fmt.Sprintf("%d. ", idx)
			idx++
		} else {
			prefix = "• "
		}

		pad := strings.Repeat(" ", indent)
		contPad := strings.Repeat(" ", len(prefix))
		first := true

		for child := li.FirstChild(); child != nil; child = child.NextSibling() {
			switch cn := child.(type) {
			case *ast.Paragraph, *ast.TextBlock:
				inline := r.collectInline(cn)
				textW := r.width - indent - len(prefix)
				if textW < 10 {
					textW = 10
				}
				wrapped := lipgloss.NewStyle().Width(textW).Render(inline)
				for i, line := range strings.Split(wrapped, "\n") {
					if first && i == 0 {
						r.buf.WriteString(pad + prefix + line + "\n")
						first = false
					} else {
						r.buf.WriteString(pad + contPad + line + "\n")
					}
				}

			case *ast.List:
				r.renderList(cn, indent+len(prefix))

			case *ast.FencedCodeBlock, *ast.CodeBlock:
				sub := &mdRenderer{src: r.src, width: r.width - indent - len(prefix)}
				sub.renderCodeBlock(child)
				for _, line := range strings.Split(strings.TrimRight(sub.buf.String(), "\n"), "\n") {
					r.buf.WriteString(pad + contPad + line + "\n")
				}

			default:
				inline := r.collectInline(child)
				if inline != "" {
					r.buf.WriteString(pad + contPad + inline + "\n")
				}
			}
		}
	}
}

// renderCodeBlock renders a fenced or indented code block.
func (r *mdRenderer) renderCodeBlock(node ast.Node) {
	lines := node.Lines()
	var code strings.Builder
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		code.Write(seg.Value(r.src))
	}
	content := strings.TrimRight(code.String(), "\n")
	for _, line := range strings.Split(content, "\n") {
		r.buf.WriteString("  " + mdCode.Render(line) + "\n")
	}
}

// renderBlockquote renders a blockquote with "│ " prefix.
func (r *mdRenderer) renderBlockquote(node ast.Node) {
	sub := &mdRenderer{src: r.src, width: r.width - 2}
	sub.walkBlocks(node)
	content := strings.TrimRight(sub.buf.String(), "\n")
	prefix := mdCode.Render("│") + " "
	for _, line := range strings.Split(content, "\n") {
		r.buf.WriteString(prefix + line + "\n")
	}
}

// renderHTMLBlock renders raw HTML block content as plain text.
func (r *mdRenderer) renderHTMLBlock(node *ast.HTMLBlock) {
	lines := node.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		r.buf.Write(seg.Value(r.src))
	}
	if node.HasClosure() {
		cl := node.ClosureLine
		r.buf.Write(cl.Value(r.src))
	}
	r.buf.WriteString("\n")
}
