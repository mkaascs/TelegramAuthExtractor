package device

import (
	"context"
	"encoding/xml"
	"fmt"
)

const dumpPath = "/sdcard/window_dump.xml"

type xmlHierarchy struct {
	XMLName xml.Name  `xml:"hierarchy"`
	Nodes   []xmlNode `xml:"node"`
}

type xmlNode struct {
	Text        string    `xml:"text,attr"`
	ResourceID  string    `xml:"resource-id,attr"`
	Class       string    `xml:"class,attr"`
	ContentDesc string    `xml:"content-desc,attr"`
	Clickable   string    `xml:"clickable,attr"`
	Bounds      string    `xml:"bounds,attr"`
	Children    []xmlNode `xml:"node"`
}

func parseUIXML(data []byte) ([]Element, error) {
	var h xmlHierarchy
	if err := xml.Unmarshal(data, &h); err != nil {
		return nil, fmt.Errorf("parse ui xml: %w", err)
	}

	var out []Element
	for _, n := range h.Nodes {
		flattenNode(n, &out)
	}

	return out, nil
}

func flattenNode(n xmlNode, out *[]Element) {
	el := Element{
		Text:        n.Text,
		ResourceID:  n.ResourceID,
		Class:       n.Class,
		ContentDesc: n.ContentDesc,
		Clickable:   n.Clickable == "true",
	}

	if b, err := ParseBounds(n.Bounds); err == nil {
		el.Bounds = b
	}

	*out = append(*out, el)

	for _, c := range n.Children {
		flattenNode(c, out)
	}
}

func dumpUI(ctx context.Context, adb *ADB) ([]Element, error) {
	if _, err := adb.Shell(ctx, "uiautomator", "dump", dumpPath); err != nil {
		return nil, fmt.Errorf("uiautomator dump: %w", err)
	}

	data, err := adb.ShellRaw(ctx, "cat", dumpPath)
	if err != nil {
		return nil, fmt.Errorf("read ui dump: %w", err)
	}

	return parseUIXML(data)
}
