package plistfix

import (
	"bytes"
	_ "embed"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"howett.net/plist"
)

var (
	//go:embed testdata/identity.plist
	identityPlist []byte
	//go:embed testdata/version.plist
	versionPlist []byte
	//go:embed testdata/seed.plist
	seedPlist []byte
)

func TestFixSetsMissingKeysAndKeepsIdentity(t *testing.T) {
	out, changes, err := Fix(identityPlist)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 4 {
		t.Fatalf("changes = %#v", changes)
	}
	if !bytes.Contains(out, []byte("Worms W.M.D")) || !bytes.Contains(out, []byte(bundleID)) || !bytes.Contains(out, []byte(minSystem)) {
		t.Fatalf("out = %s", out)
	}
	if !bytes.Contains(out, []byte("<string>keep</string>")) || !bytes.Contains(out, []byte("<true/>")) {
		t.Fatalf("out = %s", out)
	}
	_, changes, err = Fix(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 0 {
		t.Fatalf("second pass changed %#v", changes)
	}
	ver, err := ShortVersion(out)
	if err != nil || ver != "" {
		t.Fatalf("version = %q, %v", ver, err)
	}
}

func TestShortVersion(t *testing.T) {
	got, err := ShortVersion(versionPlist)
	if err != nil || got != "5.15.19" {
		t.Fatalf("version = %q, %v", got, err)
	}
}

func TestBinaryPlist(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("plutil is part of macOS")
	}
	if _, err := exec.LookPath("plutil"); err != nil {
		t.Fatal("plutil is absent")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "Info.plist")
	if err := os.WriteFile(path, versionPlist, 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.CommandContext(t.Context(), "plutil", "-convert", "binary1", path).CombinedOutput(); err != nil {
		t.Fatalf("plutil: %v\n%s", err, out)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	converted, err := AsXML(t.Context(), data)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ShortVersion(converted)
	if err != nil || got != "5.15.19" {
		t.Fatalf("version = %q, %v", got, err)
	}
}

func FuzzFix(f *testing.F) {
	f.Add(seedPlist)
	f.Add([]byte("not a plist"))
	f.Add([]byte("bplist00"))
	f.Fuzz(func(t *testing.T, data []byte) {
		out, _, err := Fix(data)
		if err != nil {
			return
		}
		again, changes, err := Fix(out)
		if err != nil {
			t.Fatal(err)
		}
		if len(changes) != 0 {
			t.Fatalf("second pass changed %#v\n%s", changes, again)
		}
	})
}

func TestShortVersionType(t *testing.T) {
	cases := []struct {
		name    string
		root    map[string]any
		want    string
		wantErr bool
	}{
		{name: "absent", root: map[string]any{"CFBundleName": "QtCore"}},
		{name: "string", root: map[string]any{KeyVersion: "5.15.19"}, want: "5.15.19"},
		{name: "integer", root: map[string]any{KeyVersion: 5}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			data, err := plist.Marshal(tc.root, plist.XMLFormat)
			if err != nil {
				t.Fatal(err)
			}
			got, err := ShortVersion(data)
			if (err != nil) != tc.wantErr || got != tc.want {
				t.Fatalf("ShortVersion = %q, %v", got, err)
			}
		})
	}
}
