package configurl

import "testing"

func TestRewrite(t *testing.T) {
	in := "MainUrl = \"http://www.team17.com/api\"\n" +
		"  URL_Internal = \"http://xom.team17.com/stage\"\n" +
		"// URL_Internal = \"http://xom.team17.com/kept\"\n" +
		"see http://www.google-analytics.com/collect\n"
	got, n := Rewrite(in)
	if n != 3 {
		t.Fatalf("changes = %d\n%s", n, got)
	}
	again, n2 := Rewrite(got)
	if again != got || n2 != 0 {
		t.Fatalf("rewrite is not stable: %d\n%s", n2, again)
	}
	if want := "MainUrl = \"https://www.team17.com/api\"\n  // DISABLED: URL_Internal = \"http://xom.team17.com/stage\"\n"; len(got) < len(want) || got[:len(want)] != want {
		t.Fatalf("got:\n%s", got)
	}
}

func FuzzRewrite(f *testing.F) {
	f.Add("MainUrl = \"http://www.team17.com\"\nURL_Internal=xom.team17.com\n")
	f.Add("plain")
	f.Fuzz(func(t *testing.T, in string) {
		once, _ := Rewrite(in)
		twice, n := Rewrite(once)
		if twice != once || n != 0 {
			t.Fatalf("not idempotent: %d", n)
		}
	})
}
