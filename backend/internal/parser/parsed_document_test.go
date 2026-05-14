package parser

import "testing"

func TestParsedDocumentHasContentFromSectionBody(t *testing.T) {
	doc := &ParsedDocument{
		Text: "   ",
		Sections: []Section{
			{Heading: "Intro", Body: "actual content"},
		},
	}

	if !doc.HasContent() {
		t.Fatal("expected section body content to make the document non-empty")
	}
}

func TestParsedDocumentHasContentFromSectionHeading(t *testing.T) {
	doc := &ParsedDocument{
		Sections: []Section{
			{Heading: "Only Heading", Body: "   "},
		},
	}

	if !doc.HasContent() {
		t.Fatal("expected section heading content to make the document non-empty")
	}
}
