package service

import (
	"fmt"

	"golang.org/x/net/html"
)

// ValidateOfficialHeaderFields validates that the document DOM contains exactly one
// canonical metadata block and exactly one data-field node for each official header field.
func ValidateOfficialHeaderFields(root *html.Node) error {
	return validateOfficialHeaderFields(root)
}

func validateOfficialHeaderFields(root *html.Node) error {
	if root == nil {
		return fmt.Errorf("%w: root node is nil", ErrUnsafeHTML)
	}

	nodes := collectNodes(root)
	metadataBlock, err := findExactlyOne(nodes, "data-block-type", OfficialHeaderBlockType)
	if err != nil {
		return err
	}
	if metadataBlock == nil {
		return fmt.Errorf("%w: metadata block is missing", ErrUnsafeHTML)
	}

	metaNodes := collectNodes(metadataBlock)
	for _, field := range officialHeaderFields {
		node, err := findExactlyOne(metaNodes, "data-field", field)
		if err != nil {
			return err
		}
		if node == nil {
			return fmt.Errorf("%w: missing official header field %q", ErrUnsafeHTML, field)
		}
	}

	return nil
}
