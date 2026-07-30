package markdown

import "strings"

type markdownLink struct {
	label string
	dest  string
}

func rewriteMarkdownLinks(markdown string) (string, []markdownLink) {
	if !strings.Contains(markdown, "](") {
		return markdown, nil
	}

	var links []markdownLink
	var b strings.Builder
	b.Grow(len(markdown))
	for i := 0; i < len(markdown); {
		if markdown[i] != '[' {
			b.WriteByte(markdown[i])
			i++
			continue
		}

		labelEnd := strings.Index(markdown[i+1:], "](")
		if labelEnd == -1 {
			b.WriteByte(markdown[i])
			i++
			continue
		}
		labelEnd += i + 1
		destStart := labelEnd + 2
		destEnd := strings.IndexByte(markdown[destStart:], ')')
		if destEnd == -1 {
			b.WriteByte(markdown[i])
			i++
			continue
		}
		destEnd += destStart

		label := markdown[i+1 : labelEnd]
		dest := markdown[destStart:destEnd]
		switch {
		case strings.HasPrefix(dest, "http://") || strings.HasPrefix(dest, "https://"):
			links = append(links, markdownLink{label: label, dest: dest})
			b.WriteString(label)
		case isRelativeMarkdownLink(dest):
			b.WriteString(label)
		default:
			b.WriteString(markdown[i : destEnd+1])
		}
		i = destEnd + 1
	}

	return b.String(), links
}

func makeMarkdownLinksClickable(rendered string, links []markdownLink) string {
	for _, link := range links {
		if link.label == "" || link.dest == "" {
			continue
		}
		rendered = strings.Replace(rendered, link.label, osc8Open+link.dest+osc8ST+link.label+osc8Close, 1)
	}
	return rendered
}

func isRelativeMarkdownLink(dest string) bool {
	if dest == "" {
		return false
	}
	if strings.HasPrefix(dest, "#") || strings.HasPrefix(dest, "/") || strings.HasPrefix(dest, "./") || strings.HasPrefix(dest, "../") {
		return true
	}
	return !strings.Contains(dest, ":")
}
