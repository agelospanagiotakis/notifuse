package notifuse_mjml

import (
	"strings"
	"testing"
)

func TestFillViewInBrowserLink(t *testing.T) {
	in := `<td class="preheader-link"><a href="" target="_blank">View this email in your browser</a></td>`
	out := FillViewInBrowserLink(in, "/archive/kois-optics/x/")
	want := `<td class="preheader-link"><a href="/archive/kois-optics/x/" target="_blank">View this email in your browser</a></td>`
	if out != want {
		t.Fatalf("got %q want %q", out, want)
	}
	if FillViewInBrowserLink(in, "") != in {
		t.Fatal("empty url must not change the document")
	}
}

func TestRewriteMailchimpLinks(t *testing.T) {
	in := `<a href="https://us1.list-manage.com/unsubscribe?u=1&id=2&e=3">u</a>` +
		`<a href="https://us1.list-manage.com/profile?u=1&id=2&e=3">p</a>` +
		`<a href="https://kois-optics.gr/">k</a>`
	out := RewriteMailchimpLinks(in, "https://app/unsub", "https://app/prefs")
	if strings.Contains(out, "list-manage.com") {
		t.Fatalf("mailchimp links remain: %s", out)
	}
	if !strings.Contains(out, `href="https://app/unsub"`) || !strings.Contains(out, `href="https://app/prefs"`) {
		t.Fatalf("links not rewritten: %s", out)
	}
	if !strings.Contains(out, `href="https://kois-optics.gr/"`) {
		t.Fatal("an unrelated link changed")
	}
}
