package device

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

type Bounds struct {
	X1, Y1, X2, Y2 int
}

func (b Bounds) Center() (x, y int) {
	return (b.X1 + b.X2) / 2, (b.Y1 + b.Y2) / 2
}

var boundsRe = regexp.MustCompile(`\[(\d+),(\d+)\]\[(\d+),(\d+)\]`)

func ParseBounds(s string) (Bounds, error) {
	match := boundsRe.FindStringSubmatch(s)
	if match == nil {
		return Bounds{}, fmt.Errorf("parse bounds: bad format %q", s)
	}

	x1, _ := strconv.Atoi(match[1])
	y1, _ := strconv.Atoi(match[2])
	x2, _ := strconv.Atoi(match[3])
	y2, _ := strconv.Atoi(match[4])
	return Bounds{X1: x1, Y1: y1, X2: x2, Y2: y2}, nil
}

type Element struct {
	Text        string
	ResourceID  string
	Class       string
	ContentDesc string
	Clickable   bool
	Bounds      Bounds
}

func (e Element) Center() (x, y int) {
	return e.Bounds.Center()
}

type Matcher func(Element) bool

func ByText(text string) Matcher {
	return func(element Element) bool {
		return element.Text == text
	}
}

func ByTextContains(sub string) Matcher {
	sub = strings.ToLower(sub)
	return func(element Element) bool {
		return strings.Contains(strings.ToLower(element.Text), sub)
	}
}

func ByResourceID(id string) Matcher {
	return func(element Element) bool {
		return element.ResourceID == id
	}
}

func ByContentDesc(desc string) Matcher {
	return func(element Element) bool {
		return element.ContentDesc == desc
	}
}

func And(matchers ...Matcher) Matcher {
	return func(element Element) bool {
		for _, matcher := range matchers {
			if !matcher(element) {
				return false
			}
		}

		return true
	}
}

func Or(matchers ...Matcher) Matcher {
	return func(element Element) bool {
		for _, matcher := range matchers {
			if matcher(element) {
				return true
			}
		}

		return false
	}
}

func Find(elements []Element, matcher Matcher) (Element, bool) {
	for _, element := range elements {
		if matcher(element) {
			return element, true
		}
	}

	return Element{}, false
}
