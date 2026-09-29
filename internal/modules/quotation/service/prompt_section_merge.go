package service

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"golang.org/x/net/html"
)

// MergePromptSection validates section identity with a DOM and then performs
// a token-preserving replacement. This keeps BA-authored non-target HTML byte
// stable while avoiding regex-based document merges.
func MergePromptSection(document, target, replacement string) (string, error) {
	if _, err := singlePromptSection(document, target); err != nil {
		return "", err
	}
	if _, err := singlePromptSection(replacement, target); err != nil {
		return "", fmt.Errorf("replacement %w", err)
	}
	return replacePromptSectionTokens(document, target, replacement)
}

func promptSectionMarkup(document, target string) (string, error) {
	node, err := singlePromptSection(document, target)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	if err := html.Render(&out, node); err != nil {
		return "", err
	}
	return out.String(), nil
}

func singlePromptSection(document, target string) (*html.Node, error) {
	root, err := html.Parse(strings.NewReader(document))
	if err != nil {
		return nil, fmt.Errorf("parse prompt document: %w", err)
	}
	var matches []*html.Node
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.ElementNode && node.Data == "section" && promptSectionAttribute(node, "data-quotation-section") == target {
			matches = append(matches, node)
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
	}
	visit(root)
	if len(matches) != 1 {
		return nil, fmt.Errorf("prompt section %q must occur exactly one time, found %d", target, len(matches))
	}
	return matches[0], nil
}

func promptSectionAttribute(node *html.Node, name string) string {
	for _, attr := range node.Attr {
		if attr.Key == name {
			return attr.Val
		}
	}
	return ""
}

func replacePromptSectionTokens(document, target, replacement string) (string, error) {
	z := html.NewTokenizer(strings.NewReader(document))
	var out strings.Builder
	skipDepth := 0
	replaced := false
	for {
		tokenType := z.Next()
		if tokenType == html.ErrorToken {
			if z.Err() != nil && z.Err() != io.EOF {
				return "", z.Err()
			}
			break
		}
		raw := string(z.Raw())
		if skipDepth > 0 {
			switch tokenType {
			case html.StartTagToken:
				if !promptVoidElement(z.Token().Data) {
					skipDepth++
				}
			case html.EndTagToken:
				skipDepth--
				if skipDepth == 0 {
					out.WriteString(replacement)
				}
			}
			continue
		}
		if tokenType == html.StartTagToken {
			token := z.Token()
			if token.Data == "section" && tokenAttribute(token, "data-quotation-section") == target {
				skipDepth = 1
				replaced = true
				continue
			}
		}
		out.WriteString(raw)
	}
	if !replaced || skipDepth != 0 {
		return "", fmt.Errorf("prompt section %q could not be replaced", target)
	}
	return out.String(), nil
}

func promptVoidElement(name string) bool {
	switch name {
	case "area", "base", "br", "col", "embed", "hr", "img", "input", "link", "meta", "param", "source", "track", "wbr":
		return true
	default:
		return false
	}
}

func tokenAttribute(token html.Token, name string) string {
	for _, attr := range token.Attr {
		if attr.Key == name {
			return attr.Val
		}
	}
	return ""
}
