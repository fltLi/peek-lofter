package post

import (
	"strings"

	"golang.org/x/net/html"
)

// ToMarkdown converts the subset of HTML used by LOFTER posts into Markdown.
func ToMarkdown(content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return strings.TrimSpace(content)
	}
	var b strings.Builder
	renderNode(&b, doc)
	return strings.TrimSpace(b.String())
}

// ExtractImageURLs returns the src values of img tags in post content.
func ExtractImageURLs(content string) []string {
	if strings.TrimSpace(content) == "" {
		return nil
	}
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return nil
	}
	var out []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "img" {
			for _, a := range n.Attr {
				if a.Key == "src" && a.Val != "" {
					out = append(out, a.Val)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return out
}

func renderNode(b *strings.Builder, n *html.Node) {
	if n.Type == html.TextNode {
		b.WriteString(n.Data)
		return
	}
	if n.Type != html.ElementNode {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			renderNode(b, c)
		}
		return
	}
	switch n.Data {
	case "script", "style":
		return
	case "p", "div", "section", "article":
		renderChildren(b, n)
		b.WriteString("\n\n")
	case "br":
		b.WriteString("\n")
	case "h1", "h2", "h3", "h4", "h5", "h6":
		level := int(n.Data[1] - '0')
		b.WriteString(strings.Repeat("#", level))
		b.WriteString(" ")
		renderChildren(b, n)
		b.WriteString("\n\n")
	case "strong", "b":
		b.WriteString("**")
		renderChildren(b, n)
		b.WriteString("**")
	case "em", "i":
		b.WriteString("*")
		renderChildren(b, n)
		b.WriteString("*")
	case "a":
		href := attr(n, "href")
		b.WriteString("[")
		renderChildren(b, n)
		b.WriteString("](")
		b.WriteString(href)
		b.WriteString(")")
	case "img":
		src := attr(n, "src")
		if src != "" {
			b.WriteString("![](")
			b.WriteString(src)
			b.WriteString(")")
		}
	case "ul", "ol":
		b.WriteString("\n")
		renderChildren(b, n)
		b.WriteString("\n")
	case "li":
		b.WriteString("- ")
		renderChildren(b, n)
		b.WriteString("\n")
	case "blockquote":
		b.WriteString("\n> ")
		renderChildren(b, n)
		b.WriteString("\n")
	case "pre":
		b.WriteString("\n\n")
		b.WriteString("\x60\x60\x60")
		b.WriteString("\n")
		renderChildren(b, n)
		b.WriteString("\n")
		b.WriteString("\x60\x60\x60")
		b.WriteString("\n\n")
	case "code":
		b.WriteString("\x60")
		renderChildren(b, n)
		b.WriteString("\x60")
	default:
		renderChildren(b, n)
	}
}

func renderChildren(b *strings.Builder, n *html.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		renderNode(b, c)
	}
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// ToText strips HTML into readable plain text while preserving paragraphs,
// line breaks and list items.
func ToText(content string) string {
	if strings.TrimSpace(content) == "" {
		return ""
	}
	doc, err := html.Parse(strings.NewReader(content))
	if err != nil {
		return strings.TrimSpace(content)
	}
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
			return
		}
		if n.Type != html.ElementNode {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c)
			}
			return
		}
		switch n.Data {
		case "script", "style", "noscript":
			return
		case "br":
			b.WriteString("\n")
		case "p", "div", "section", "article", "blockquote":
			walkChildren(n, walk)
			b.WriteString("\n\n")
		case "h1", "h2", "h3", "h4", "h5", "h6":
			walkChildren(n, walk)
			b.WriteString("\n\n")
		case "li":
			ensureLine(&b)
			b.WriteString("- ")
			walkChildren(n, walk)
			b.WriteString("\n")
		case "ul", "ol":
			ensureLine(&b)
			walkChildren(n, walk)
			b.WriteString("\n")
		case "pre":
			ensureLine(&b)
			walkChildren(n, walk)
			b.WriteString("\n\n")
		case "img":
			// Keep text exports readable; image URLs stay in the archive.
		default:
			walkChildren(n, walk)
		}
	}
	walk(doc)
	return normalizeText(b.String())
}

func walkChildren(n *html.Node, walk func(*html.Node)) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c)
	}
}

func ensureLine(b *strings.Builder) {
	if b.Len() == 0 {
		return
	}
	s := b.String()
	if !strings.HasSuffix(s, "\n") {
		b.WriteString("\n")
	}
}

func normalizeText(s string) string {
	lines := strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n")
	var out []string
	blank := 0
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			blank++
			if blank > 1 {
				continue
			}
		} else {
			blank = 0
		}
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
