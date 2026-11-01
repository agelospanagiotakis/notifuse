package notifuse_mjml

import (
	"testing"

	"github.com/preslavrachev/gomjml/mjml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// gomjml decodes an entity before it lexes an attribute, so the two entities that
// decode to a delimiter — &quot; and &lt; — break the parse whenever they appear
// inside an attribute value. Mailchimp escapes the quotes in a font stack that way
// in every campaign before about 2021, so this is not a hypothetical input: it is
// the shape of roughly a decade of real newsletters.
func TestPreprocessMjmlForXMLSurvivesQuoteAndLtInAttributes(t *testing.T) {
	tests := []struct {
		name  string
		body  string
		plain string
	}{
		{
			name:  "quot in a style attribute (the Mailchimp font stack)",
			body:  `<td align="left" style="font-family: Arial, &quot;Helvetica Neue&quot;, Helvetica, sans-serif;">Hello</td>`,
			plain: "Hello",
		},
		{
			name:  "quot in a title attribute",
			body:  `<td title="the &quot;best&quot; one">Hello</td>`,
			plain: "Hello",
		},
		{
			name:  "lt in an attribute",
			body:  `<td title="a &lt; b">Hello</td>`,
			plain: "Hello",
		},
		{
			name:  "both, several times",
			body:  `<a href="https://example.test/?a=1&amp;b=2" title="&quot;x&quot; &lt; &quot;y&quot;">go</a>`,
			plain: "go",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mjmlSource := `<mjml><mj-body><mj-section><mj-column><mj-raw>` + tc.body + `</mj-raw></mj-column></mj-section></mj-body></mjml>`

			html, err := mjml.Render(preprocessMjmlForXML(mjmlSource))
			require.NoError(t, err, "this HTML must compile; it is ordinary newsletter markup")
			assert.Contains(t, html, tc.plain)
		})
	}
}

// The rewrite must be an encoding change only. A quote in body text still has to
// render as a quote — if the fix turned &quot; into &apos; the page would quietly
// start printing apostrophes where the author wrote quotation marks.
func TestPreprocessMjmlForXMLPreservesEntityMeaning(t *testing.T) {
	preprocessed := preprocessMjmlForXML(`<mj-text>He said &quot;hello&quot; and 1 &lt; 2</mj-text>`)

	assert.Contains(t, preprocessed, "&#34;", "quot must become its numeric form")
	assert.Contains(t, preprocessed, "&#60;", "lt must become its numeric form")
	assert.NotContains(t, preprocessed, "&quot;")
	assert.NotContains(t, preprocessed, "&apos;", "an apostrophe is not a substitute for a quote")

	html, err := mjml.Render(preprocessed)
	require.NoError(t, err)
	assert.Contains(t, html, "&#34;hello&#34;")
	assert.Contains(t, html, "&#60;")
}

// A bare & in an attribute value already parses; the rewrite must not touch it.
func TestPreprocessMjmlForXMLLeavesAmpAlone(t *testing.T) {
	src := `<mj-text><a href="https://example.test/?a=1&amp;b=2">x</a></mj-text>`

	html, err := mjml.Render(preprocessMjmlForXML(src))
	require.NoError(t, err)
	assert.Contains(t, html, "a=1&amp;b=2")
}
