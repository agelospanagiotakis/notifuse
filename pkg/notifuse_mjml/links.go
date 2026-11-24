package notifuse_mjml

import "regexp"

var (
	// The "View this email in your browser" block: a td with the preheader-link class whose
	// anchor was saved with an empty href, as imported Mailchimp templates often are.
	preheaderHrefEmpty = regexp.MustCompile(`(?s)(<td[^>]*class="[^"]*preheader-link[^"]*"[^>]*>.*?)href=""`)
	viewInBrowserEmpty = regexp.MustCompile(`href=""([^>]*)>\s*View this email in your browser`)
)

// FillViewInBrowserLink points an empty "View this email in your browser" anchor at url: the
// campaign's archive permalink on an archive page, or the recipient's view-in-browser URL in a
// send. It leaves the document untouched when there is nothing to fill.
func FillViewInBrowserLink(htmlDoc, url string) string {
	if htmlDoc == "" || url == "" {
		return htmlDoc
	}
	out := preheaderHrefEmpty.ReplaceAllString(htmlDoc, `${1}href="`+url+`"`)
	out = viewInBrowserEmpty.ReplaceAllString(out, `href="`+url+`"${1}>View this email in your browser`)
	return out
}
