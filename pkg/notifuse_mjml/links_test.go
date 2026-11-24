package notifuse_mjml

import "testing"

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
