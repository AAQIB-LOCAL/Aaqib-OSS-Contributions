package audit

import (
	"strings"
	"testing"
)

func TestExistingDescriptionsParsesCuratedRows(t *testing.T) {
	readme := "| [owner/repo](https://github.com/owner/repo) | [#8](https://github.com/owner/repo/pull/8) | Curated \\| wording | Merged |\n"
	descriptions := ExistingDescriptions(readme)
	if got := descriptions["owner/repo#8"]; got != `Curated \| wording` {
		t.Fatalf("got description %q", got)
	}
}

func TestEscapeMarkdownMatchesExistingRules(t *testing.T) {
	if got := EscapeMarkdown("  title | with\nnewline  "); got != `title \| with newline` {
		t.Fatalf("got %q", got)
	}
}

func TestRenderReadmeUpdatesDynamicSectionsOnly(t *testing.T) {
	template := `# Demo
<table>
  <tr>
    <td align="center"><strong>1</strong><br/>Merged upstream PRs</td>
    <td align="center"><strong>1</strong><br/>External repositories</td>
    <td align="center"><strong>3</strong><br/>Public authored PRs (high-water mark)</td>
  </tr>
</table>
Browse all 1 merged pull requests
Browse merged PR counts across 1 external repositories
The broader audit currently discovers **3 authored GitHub pull requests** for the author.

| Repository | PR | Technical contribution | Status |
|---|---:|---|---|
| [old/repo](https://github.com/old/repo) | [#1](https://github.com/old/repo/pull/1) | Old row | Merged |

</details>

| Repository | Merged PRs |
|---|---:|
| [old/repo](https://github.com/old/repo) | 1 |
| **Total** | **1** |

Keep this curated text.
`
	got, err := RenderReadme(template, 2, 2, 8,
		"| [new/repo](https://github.com/new/repo) | [#4](https://github.com/new/repo/pull/4) | Curated row | Merged |",
		"| [new/repo](https://github.com/new/repo) | 1 |\n| [old/repo](https://github.com/old/repo) | 1 |",
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"<strong>2</strong><br/>Merged upstream PRs",
		"<strong>2</strong><br/>External repositories",
		"<strong>8</strong><br/>Public authored PRs (high-water mark)",
		"Browse all 2 merged pull requests",
		"Browse merged PR counts across 2 external repositories",
		"discovers **8 authored GitHub pull requests**",
		"| **Total** | **2** |",
		"Keep this curated text.",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("generated README missing %q", want)
		}
	}
	if strings.Contains(got, "Old row") {
		t.Fatal("contribution section retained its stale generated row")
	}
}

func TestExistingDescriptionsIgnoresNonContributionLines(t *testing.T) {
	if got := ExistingDescriptions("not a row\n| broken | row |\n"); len(got) != 0 {
		t.Fatalf("expected no descriptions, got %#v", got)
	}
}

func TestRenderReadmeFailsWhenExpectedTemplateSectionsAreMissing(t *testing.T) {
	if _, err := RenderReadme("# missing", 1, 1, 1, "", ""); err == nil {
		t.Fatal("missing snapshot template should fail")
	}
}

func TestReplaceContentBetweenRejectsMissingMarkers(t *testing.T) {
	if _, err := replaceContentBetween("body", "start", "end", "replacement"); err == nil {
		t.Fatal("missing start marker should fail")
	}
	if _, err := replaceContentBetween("start body", "start", "end", "replacement"); err == nil {
		t.Fatal("missing end marker should fail")
	}
}

func TestReplaceFirstRegexRejectsMissingPattern(t *testing.T) {
	if _, err := replaceFirstRegex("body", `snapshot`, "new"); err == nil {
		t.Fatal("missing replacement pattern should fail")
	}
}
