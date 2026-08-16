// Package legacyhtml extracts GitBucket 4.7.1 legacy HTML fragments with
// golang.org/x/net/html. It is the single place that depends on x/net/html so
// the rest of bkt stays dependency-light.
package legacyhtml

import (
	"errors"
	"io"
	"strings"

	"golang.org/x/net/html"
)

var (
	ErrFormNotFound   = errors.New("legacyhtml: no form with matching action")
	ErrAmbiguousForm  = errors.New("legacyhtml: multiple forms match the action")
	ErrMissingField   = errors.New("legacyhtml: field not present in form")
	ErrAmbiguousField = errors.New("legacyhtml: field appears more than once")
)

// Form is the set of named controls inside one <form> element. It models the
// subset of HTML forms GitBucket 4.7.1 actually uses: <input>, <select>, and
// <textarea> controls indexed by name.
type Form struct {
	Action string
	Method string

	inputs map[string][]string
}

// Required reports whether a control with the given name exists in the form,
// regardless of its value. It returns ErrMissingField when no such control
// exists.
func (f Form) Required(name string) error {
	if _, ok := f.inputs[name]; !ok {
		return ErrMissingField
	}
	return nil
}

// Field returns the single value of the named control. It returns
// ErrMissingField when the control is absent and ErrAmbiguousField when it
// appears more than once.
func (f Form) Field(name string) (string, error) {
	values := f.FieldValues(name)
	switch len(values) {
	case 0:
		return "", ErrMissingField
	case 1:
		return values[0], nil
	default:
		return "", ErrAmbiguousField
	}
}

// FieldValues returns every value of the named control in document order.
// Textarea content, input values, and single/multiple select selections are
// all indexed by control name.
func (f Form) FieldValues(name string) []string {
	return append([]string(nil), f.inputs[name]...)
}

// Textarea returns the text content of the named <textarea> control. It is
// equivalent to Field(name), because textarea content is indexed under the
// control name.
func (f Form) Textarea(name string) (string, error) {
	return f.Field(name)
}

// ParseForm parses the single <form> whose action ends with actionSuffix.
// Trailing slashes are ignored on both the action and the suffix. Zero or
// multiple matches are errors.
func ParseForm(r io.Reader, actionSuffix string) (Form, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return Form{}, err
	}
	suffix := strings.TrimRight(actionSuffix, "/")
	var matches []Form
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "form" {
			action := strings.TrimRight(attr(n, "action"), "/")
			if strings.HasSuffix(action, suffix) {
				matches = append(matches, collectForm(n))
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	switch len(matches) {
	case 0:
		return Form{}, ErrFormNotFound
	case 1:
		return matches[0], nil
	default:
		return Form{}, ErrAmbiguousForm
	}
}

func collectForm(f *html.Node) Form {
	form := Form{
		Action: attr(f, "action"),
		inputs: map[string][]string{},
	}
	form.Method = strings.ToLower(attr(f, "method"))
	if form.Method == "" {
		form.Method = "get"
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type != html.ElementNode {
			return
		}
		switch n.Data {
		case "input":
			if name := attr(n, "name"); name != "" {
				form.inputs[name] = append(form.inputs[name], inputValue(n))
			}
		case "select":
			if name := attr(n, "name"); name != "" {
				form.inputs[name] = append(form.inputs[name], selectValues(n)...)
			}
		case "textarea":
			if name := attr(n, "name"); name != "" {
				form.inputs[name] = append(form.inputs[name], textContent(n))
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(f)
	return form
}

func inputValue(n *html.Node) string {
	if hasAttr(n, "value") {
		return attr(n, "value")
	}
	switch strings.ToLower(attr(n, "type")) {
	case "checkbox", "radio":
		return "on"
	}
	return ""
}

func selectValues(s *html.Node) []string {
	var opts []*html.Node
	collectOptions(s, &opts)
	if hasAttr(s, "multiple") {
		var vals []string
		for _, o := range opts {
			if hasAttr(o, "selected") {
				vals = append(vals, optionValue(o))
			}
		}
		return vals
	}
	for _, o := range opts {
		if hasAttr(o, "selected") {
			return []string{optionValue(o)}
		}
	}
	if len(opts) > 0 {
		return []string{optionValue(opts[0])}
	}
	return nil
}

func collectOptions(n *html.Node, opts *[]*html.Node) {
	if n.Type == html.ElementNode && n.Data == "option" {
		*opts = append(*opts, n)
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		collectOptions(child, opts)
	}
}

func optionValue(o *html.Node) string {
	if hasAttr(o, "value") {
		return attr(o, "value")
	}
	return strings.TrimSpace(textContent(o))
}

func hasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

// TableIssueLinks returns the hrefs of every <a class="issue-title"> inside a
// <table class="table-issues">. It is fail-closed: a malformed document
// returns an error.
func TableIssueLinks(r io.Reader) ([]string, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}
	var links []string
	tableFound := false
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "table" && hasClass(n, "table-issues") {
			tableFound = true
			collectIssueLinks(n, &links)
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	if !tableFound {
		return nil, errors.New("legacyhtml: table.table-issues not found")
	}
	return links, nil
}

func collectIssueLinks(n *html.Node, links *[]string) {
	if n.Type == html.ElementNode && n.Data == "a" && hasClass(n, "issue-title") {
		*links = append(*links, href(n))
	}
	for child := n.FirstChild; child != nil; child = child.NextSibling {
		collectIssueLinks(child, links)
	}
}

// BodyText returns the text content of the first element with id
// "issueContent". A missing element is an error.
func BodyText(r io.Reader) (string, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return "", err
	}
	var text string
	var walk func(*html.Node) bool
	walk = func(n *html.Node) bool {
		if n.Type == html.ElementNode && attr(n, "id") == "issueContent" {
			text = textContent(n)
			return true
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if walk(child) {
				return true
			}
		}
		return false
	}
	if !walk(doc) {
		return "", errors.New("legacyhtml: #issueContent not found")
	}
	return text, nil
}

// PageTitle returns the text content of the first <title> element.
func PageTitle(r io.Reader) (string, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return "", err
	}
	var walk func(*html.Node) bool
	walk = func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "title" {
			return true
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if walk(child) {
				return true
			}
		}
		return false
	}
	if !walk(doc) {
		return "", errors.New("legacyhtml: <title> not found")
	}
	// Re-walk to grab the title element itself.
	return titleText(doc), nil
}

func titleText(doc *html.Node) string {
	var text string
	var walk func(*html.Node) bool
	walk = func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "title" {
			text = textContent(n)
			return true
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			if walk(child) {
				return true
			}
		}
		return false
	}
	_ = walk(doc)
	return text
}

// State returns "open", "closed", or "" by scanning <span> elements whose
// trimmed text is exactly Open or Closed. Finding both states is an error.
func State(r io.Reader) (string, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return "", err
	}
	open := false
	closed := false
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "span" {
			switch strings.TrimSpace(textContent(n)) {
			case "Open":
				open = true
			case "Closed":
				closed = true
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	if open && closed {
		return "", errors.New("legacyhtml: both Open and Closed states found")
	}
	if open {
		return "open", nil
	}
	if closed {
		return "closed", nil
	}
	return "", nil
}

// Inputs returns every <input> element's name mapped to its value
// occurrences.
func Inputs(r io.Reader) (map[string][]string, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return nil, err
	}
	inputs := map[string][]string{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "input" {
			name := attr(n, "name")
			if name != "" {
				inputs[name] = append(inputs[name], attr(n, "value"))
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return inputs, nil
}

// TextareaByID returns the text content of the single <textarea> with the
// given id. Zero or multiple matches are errors.
func TextareaByID(r io.Reader, id string) (string, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return "", err
	}
	var found []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "textarea" && attr(n, "id") == id {
			found = append(found, textContent(n))
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	if len(found) != 1 {
		return "", errors.New("legacyhtml: expected exactly one textarea #" + id)
	}
	return found[0], nil
}

func hasClass(n *html.Node, want string) bool {
	for _, a := range n.Attr {
		if a.Key == "class" {
			for _, c := range strings.Fields(a.Val) {
				if c == want {
					return true
				}
			}
		}
	}
	return false
}

func href(n *html.Node) string {
	return attr(n, "href")
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func textContent(n *html.Node) string {
	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			sb.WriteString(n.Data)
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return sb.String()
}
