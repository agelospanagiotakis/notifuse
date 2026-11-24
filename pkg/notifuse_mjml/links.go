package notifuse_mjml

import (
	"regexp"
	"strings"
)

var (
	// The "View this email in your browser" block: a td with the preheader-link class whose
	// anchor was saved with an empty href, as imported Mailchimp templates often are.
	preheaderHrefEmpty = regexp.MustCompile(`(?s)(<td[^>]*class="[^"]*preheader-link[^"]*"[^>]*>.*?)href=""`)
	viewInBrowserEmpty = regexp.MustCompile(`href=""([^>]*)>\s*View this email in your browser`)
	// Mailchimp's per-list links (us1.list-manage.com/unsubscribe…, /profile…, mailchi.mp).
	mailchimpHref = regexp.MustCompile(`(?i)href="(https?://[^"]*(?:list-manage\.com|mailchi\.mp)[^"]*)"`)
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

// RewriteMailchimpLinks points Mailchimp's static unsubscribe/preferences links at the
// platform's own per-recipient URLs. Imported campaigns carry those Mailchimp links, which
// act on Mailchimp's lists rather than on this workspace's contacts.
func RewriteMailchimpLinks(htmlDoc, unsubscribeURL, preferencesURL string) string {
	if htmlDoc == "" {
		return htmlDoc
	}
	return mailchimpHref.ReplaceAllStringFunc(htmlDoc, func(m string) string {
		lower := strings.ToLower(m)
		switch {
		case strings.Contains(lower, "/unsubscribe"):
			if unsubscribeURL != "" {
				return `href="` + unsubscribeURL + `"`
			}
		case strings.Contains(lower, "/profile"), strings.Contains(lower, "/about"), strings.Contains(lower, "/manage"):
			if preferencesURL != "" {
				return `href="` + preferencesURL + `"`
			}
		}
		return m
	})
}
