package legacyhtml

import (
	"errors"
	"strings"
	"testing"
)

func parse(t *testing.T, page, suffix string) Form {
	t.Helper()
	form, err := ParseForm(strings.NewReader(page), suffix)
	if err != nil {
		t.Fatalf("ParseForm(%q): %v", suffix, err)
	}
	return form
}

func TestParseFormSignIn(t *testing.T) {
	form := parse(t, `<html>
<form action="/gitbucket/signin" method="POST">
  <input type="text" name="userName">
  <input type="password" name="password">
</form>
</html>`, "/signin")

	if form.Action != "/gitbucket/signin" {
		t.Fatalf("Action = %q", form.Action)
	}
	if form.Method != "post" {
		t.Fatalf("Method = %q", form.Method)
	}
	if err := form.Required("userName"); err != nil {
		t.Fatalf("Required(userName): %v", err)
	}
	if err := form.Required("password"); err != nil {
		t.Fatalf("Required(password): %v", err)
	}
	if err := form.Required("doesNotExist"); !errors.Is(err, ErrMissingField) {
		t.Fatalf("Required(doesNotExist) = %v, want ErrMissingField", err)
	}
}

func TestParseFormDefaultMethod(t *testing.T) {
	form := parse(t, `<form action="signin"><input name="a" value="1"></form>`, "signin")
	if form.Method != "get" {
		t.Fatalf("Method = %q, want default get", form.Method)
	}
}

func TestParseFormNotFound(t *testing.T) {
	_, err := ParseForm(strings.NewReader(`<form action="/x"><input name="a" value="1"></form>`), "/pulls/new")
	if !errors.Is(err, ErrFormNotFound) {
		t.Fatalf("err = %v, want ErrFormNotFound", err)
	}
}

func TestParseFormAmbiguous(t *testing.T) {
	_, err := ParseForm(strings.NewReader(`<html>
<form action="/a/signin"><input name="a" value="1"></form>
<form action="/b/signin"><input name="b" value="2"></form>
</html>`), "/signin")
	if !errors.Is(err, ErrAmbiguousForm) {
		t.Fatalf("err = %v, want ErrAmbiguousForm", err)
	}
}

func TestComparePageHiddenFields(t *testing.T) {
	form := parse(t, `<html>
<form action="/acme/repo/pulls/new" method="POST">
  <input type="hidden" name="targetUserName" value="acme">
  <input type="hidden" name="targetBranch" value="main">
  <input type="hidden" name="requestUserName" value="acme">
  <input type="hidden" name="requestRepositoryName" value="repo">
  <input type="hidden" name="requestBranch" value="feature/api">
  <input type="hidden" name="commitIdFrom" value="1111111111111111111111111111111111111111">
  <input type="hidden" name="commitIdTo" value="2222222222222222222222222222222222222222">
</form>
</html>`, "/pulls/new")

	checks := map[string]string{
		"targetUserName":        "acme",
		"targetBranch":          "main",
		"requestUserName":       "acme",
		"requestRepositoryName": "repo",
		"requestBranch":         "feature/api",
		"commitIdFrom":          "1111111111111111111111111111111111111111",
		"commitIdTo":            "2222222222222222222222222222222222222222",
	}
	for name, want := range checks {
		got, err := form.Field(name)
		if err != nil {
			t.Fatalf("Field(%s): %v", name, err)
		}
		if got != want {
			t.Fatalf("Field(%s) = %q, want %q", name, got, want)
		}
	}
}

func TestEntityDecoding(t *testing.T) {
	form := parse(t, `<form action="/pulls/new">
  <input type="hidden" name="targetBranch" value="feature&fix">
  <input type="hidden" name="label" value="&#34;quoted&#34;">
</form>`, "/pulls/new")

	v, err := form.Field("targetBranch")
	if err != nil {
		t.Fatalf("Field(targetBranch): %v", err)
	}
	if v != "feature&fix" {
		t.Fatalf("targetBranch = %q, want feature&fix", v)
	}
	v, err = form.Field("label")
	if err != nil {
		t.Fatalf("Field(label): %v", err)
	}
	if v != `"quoted"` {
		t.Fatalf("label = %q, want quoted", v)
	}
}

func TestSelectSelectedOption(t *testing.T) {
	form := parse(t, `<form action="/issues/new">
  <select name="milestoneId">
    <option value="1">v1</option>
    <option value="2" selected>v2</option>
    <option value="3">v3</option>
  </select>
</form>`, "/issues/new")

	values := form.FieldValues("milestoneId")
	if len(values) != 1 || values[0] != "2" {
		t.Fatalf("milestoneId = %v, want [2]", values)
	}
}

func TestSelectFirstOptionDefault(t *testing.T) {
	form := parse(t, `<form action="/issues/new">
  <select name="milestoneId">
    <option value="1">v1</option>
    <option value="2">v2</option>
  </select>
</form>`, "/issues/new")

	values := form.FieldValues("milestoneId")
	if len(values) != 1 || values[0] != "1" {
		t.Fatalf("milestoneId = %v, want [1]", values)
	}
}

func TestSelectEmptyValueAttr(t *testing.T) {
	form := parse(t, `<form action="/issues/new">
  <select name="empty"><option value="" selected></option></select>
</form>`, "/issues/new")

	values := form.FieldValues("empty")
	if len(values) != 1 || values[0] != "" {
		t.Fatalf("empty = %v, want [\"\"]", values)
	}
}

func TestSelectMultipleNoneSelected(t *testing.T) {
	form := parse(t, `<form action="/issues/new">
  <select name="labels" multiple>
    <option value="a">a</option>
    <option value="b">b</option>
  </select>
</form>`, "/issues/new")

	values := form.FieldValues("labels")
	if len(values) != 0 {
		t.Fatalf("labels = %v, want no default submissions", values)
	}
}

func TestSelectOptionTextFallback(t *testing.T) {
	form := parse(t, `<form action="/issues/new">
  <select name="pick"><option selected>fallback</option></select>
</form>`, "/issues/new")

	values := form.FieldValues("pick")
	if len(values) != 1 || values[0] != "fallback" {
		t.Fatalf("pick = %v, want [fallback]", values)
	}
}

func TestTextareaRawContent(t *testing.T) {
	form := parse(t, `<form action="/issues/edit/12" method="POST">
  <input type="hidden" name="issueId" value="12">
  <textarea name="content">Asana: https://app.asana.com/0/123/987654321

## Summary
- fix</textarea>
</form>`, "/issues/edit/12")

	content, err := form.Textarea("content")
	if err != nil {
		t.Fatalf("Textarea(content): %v", err)
	}
	if content != "Asana: https://app.asana.com/0/123/987654321\n\n## Summary\n- fix" {
		t.Fatalf("content = %q", content)
	}
	v, err := form.Field("issueId")
	if err != nil {
		t.Fatalf("Field(issueId): %v", err)
	}
	if v != "12" {
		t.Fatalf("issueId = %q", v)
	}
}

func TestFieldMissing(t *testing.T) {
	form := parse(t, `<form action="/pulls/new"><input type="hidden" name="a" value="1"></form>`, "/pulls/new")
	_, err := form.Field("missing")
	if !errors.Is(err, ErrMissingField) {
		t.Fatalf("Field(missing) = %v, want ErrMissingField", err)
	}
}

func TestFieldDuplicateRejected(t *testing.T) {
	// Duplicate occurrences with identical values are still ambiguous for
	// scalar extraction.
	form := parse(t, `<form action="/pulls/new">
  <input type="hidden" name="x" value="dup">
  <input type="hidden" name="x" value="dup">
</form>`, "/pulls/new")

	if _, err := form.Field("x"); !errors.Is(err, ErrAmbiguousField) {
		t.Fatalf("Field(x) = %v, want ErrAmbiguousField", err)
	}
	values := form.FieldValues("x")
	if len(values) != 2 || values[0] != "dup" || values[1] != "dup" {
		t.Fatalf("FieldValues(x) = %v, want [dup dup]", values)
	}
}

func TestFieldDuplicateDistinctRejected(t *testing.T) {
	form := parse(t, `<form action="/pulls/new">
  <input type="hidden" name="x" value="a">
  <input type="hidden" name="x" value="b">
</form>`, "/pulls/new")

	if _, err := form.Field("x"); !errors.Is(err, ErrAmbiguousField) {
		t.Fatalf("Field(x) = %v, want ErrAmbiguousField", err)
	}
}

func TestFieldListValues(t *testing.T) {
	// Repeated checkbox names are legitimate list fields.
	form := parse(t, `<form action="/issues/new">
  <input type="checkbox" name="labelIds[]" value="1">
  <input type="checkbox" name="labelIds[]" value="2">
  <input type="checkbox" name="labelIds[]" value="3">
</form>`, "/issues/new")

	values := form.FieldValues("labelIds[]")
	if len(values) != 3 || values[0] != "1" || values[2] != "3" {
		t.Fatalf("labelIds[] = %v", values)
	}
}

func TestActionSuffixTrimsSlash(t *testing.T) {
	form := parse(t, `<form action="/acme/repo/pulls/new/"><input name="a" value="1"></form>`, "/pulls/new")
	if form.Action != "/acme/repo/pulls/new/" {
		t.Fatalf("Action = %q", form.Action)
	}
}

func TestTextareaAmbiguous(t *testing.T) {
	form := parse(t, `<form action="/x">
  <textarea name="t">a</textarea>
  <textarea name="t">b</textarea>
</form>`, "/x")

	if _, err := form.Textarea("t"); !errors.Is(err, ErrAmbiguousField) {
		t.Fatalf("Textarea(t) = %v, want ErrAmbiguousField", err)
	}
}

func TestEmptyDocumentNoForm(t *testing.T) {
	// html.Parse is lenient and accepts an empty document as an empty tree,
	// so a page without the target form fails closed with ErrFormNotFound.
	form, err := ParseForm(strings.NewReader(""), "/signin")
	_ = form
	if err == nil || !errors.Is(err, ErrFormNotFound) {
		t.Fatalf("err = %v, want ErrFormNotFound", err)
	}
}
