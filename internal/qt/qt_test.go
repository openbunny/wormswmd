package qt

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/openbunny/wormswmd/internal/safe"
)

const maxFixtureBytes = 1 << 20

type tarEntry struct {
	name string
	body []byte
	link string
	typ  byte
	size int64
}

func TestExtract(t *testing.T) {
	t.Run("relative symlink", func(t *testing.T) {
		data, err := gzipTar([]tarEntry{{
			name: "dir/link",
			typ:  tar.TypeSymlink,
			link: "file",
		}})
		if err != nil {
			t.Fatal(err)
		}
		dest := t.TempDir()
		if err := Extract(t.Context(), bytes.NewReader(data), dest); err != nil {
			t.Fatal(err)
		}
		got, err := os.Readlink(filepath.Join(dest, "dir", "link"))
		if err != nil {
			t.Fatal(err)
		}
		if got != "file" {
			t.Fatalf("Readlink = %q", got)
		}
	})

	tests := []struct {
		name  string
		entry tarEntry
		raw   []byte
		want  string
	}{
		{
			name:  "absolute symlink",
			entry: tarEntry{name: "link", typ: tar.TypeSymlink, link: "/tmp/outside"},
			want:  "absolute",
		},
		{
			name:  "dotdot",
			entry: tarEntry{name: "dir/../../outside", body: []byte("x")},
			want:  "escapes",
		},
		{
			name:  "oversize",
			entry: tarEntry{name: "big", size: maxExtractBytes + 1},
			want:  "exceeds",
		},
		{
			name:  "hard link",
			entry: tarEntry{name: "link", typ: tar.TypeLink, link: "target"},
			want:  "unsupported",
		},
		{
			name: "not gzip",
			raw:  []byte("not-gzip"),
			want: "gzip",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := tt.raw
			if data == nil {
				var err error
				data, err = gzipTar([]tarEntry{tt.entry})
				if err != nil {
					t.Fatal(err)
				}
			}
			if tt.entry.size > maxExtractBytes && len(data) > maxFixtureBytes {
				t.Fatalf("fixture length = %d", len(data))
			}
			err := Extract(t.Context(), bytes.NewReader(data), t.TempDir())
			if err == nil {
				t.Fatal("extract returned nil")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Fatal(err)
			}
		})
	}
}

func escapeChain() []tarEntry {
	return []tarEntry{
		{name: "d", typ: tar.TypeDir},
		{name: "d/e", typ: tar.TypeDir},
		{name: "d/e/a", typ: tar.TypeSymlink, link: "../.."},
		{name: "f", typ: tar.TypeSymlink, link: "d/e/a/../.."},
		{name: "f/pwned.txt", body: []byte("x")},
	}
}

func TestExtractRejectsSymlinkChainEscape(t *testing.T) {
	data, err := gzipTar(escapeChain())
	if err != nil {
		t.Fatal(err)
	}
	parent := t.TempDir()
	dest := filepath.Join(parent, "one", "two")
	if err := Extract(t.Context(), bytes.NewReader(data), dest); err == nil {
		t.Fatal("symlink chain escape extracted")
	}
	for _, escaped := range []string{filepath.Join(parent, "pwned.txt"), filepath.Join(parent, "one", "pwned.txt")} {
		if _, err := os.Stat(escaped); err == nil {
			t.Fatalf("%s written outside the destination", escaped)
		}
	}
}

func FuzzExtract(f *testing.F) {
	f.Add([]byte{})
	seed, err := gzipTar([]tarEntry{
		{name: "a", body: []byte("b")},
		{name: "dir/link", typ: tar.TypeSymlink, link: "file"},
	})
	if err != nil {
		f.Fatal(err)
	}
	f.Add(seed)
	escape, err := gzipTar(escapeChain())
	if err != nil {
		f.Fatal(err)
	}
	f.Add(escape)
	f.Fuzz(func(t *testing.T, data []byte) {
		dest := t.TempDir()
		err := Extract(t.Context(), bytes.NewReader(data), dest)
		if !gzipMagic(data) {
			if err == nil {
				t.Fatal("non-gzip accepted")
			}
			return
		}
		if err != nil {
			return
		}
		walkErr := filepath.Walk(dest, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink == 0 {
				return nil
			}
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			resolved := filepath.Clean(filepath.Join(filepath.Dir(path), target))
			return safe.InRoot(dest, resolved)
		})
		if walkErr != nil {
			t.Fatal(walkErr)
		}
	})
}

func TestFetch(t *testing.T) {
	body := []byte("qt-bytes")
	sum := sha256.Sum256(body)
	pin := hex.EncodeToString(sum[:])
	badPin := strings.Repeat("0", sha256HexLen)
	if badPin == pin {
		t.Fatal("pin collision")
	}
	tests := []struct {
		name     string
		statuses []int
		pin      string
		prewrite bool
		wantN    int
		wantErr  bool
		checksum bool
	}{
		{name: "retry", statuses: []int{http.StatusInternalServerError, http.StatusOK}, pin: pin, wantN: 2},
		{name: "checksum", statuses: []int{http.StatusOK}, pin: badPin, wantN: 1, wantErr: true, checksum: true},
		{name: "not found", statuses: []int{http.StatusNotFound}, pin: pin, wantN: 1, wantErr: true},
		{name: "cached", pin: pin, prewrite: true, wantN: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var n atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				i := int(n.Add(1)) - 1
				code := http.StatusOK
				if i < len(tt.statuses) {
					code = tt.statuses[i]
				}
				if code != http.StatusOK {
					w.WriteHeader(code)
					return
				}
				if _, err := w.Write(body); err != nil {
					t.Errorf("write body: %v", err)
				}
			}))
			t.Cleanup(srv.Close)
			dest := filepath.Join(t.TempDir(), "qt.tar.gz")
			if tt.prewrite {
				if err := os.WriteFile(dest, body, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			err := Fetch(t.Context(), dest, srv.URL, tt.pin)
			switch {
			case tt.wantErr && err == nil:
				t.Fatal("fetch returned nil")
			case tt.wantErr && tt.checksum && !errors.Is(err, errChecksum) && !strings.Contains(err.Error(), "checksum"):
				t.Fatal(err)
			case !tt.wantErr && err != nil:
				t.Fatal(err)
			case !tt.wantErr:
				got, err := os.ReadFile(dest)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, body) {
					t.Fatalf("dest = %q", got)
				}
			}
			if got := int(n.Load()); got != tt.wantN {
				t.Fatalf("requests = %d", got)
			}
		})
	}
}

func TestNormalizePin(t *testing.T) {
	valid := strings.Repeat("ab", 32)
	tests := []struct {
		name string
		pin  string
		want string
		ok   bool
	}{
		{name: "empty", pin: ""},
		{name: "short", pin: strings.Repeat("a", 63)},
		{name: "long", pin: strings.Repeat("a", 65)},
		{name: "nonhex", pin: strings.Repeat("g", 64)},
		{name: "valid", pin: valid, want: valid, ok: true},
		{name: "upper", pin: strings.ToUpper(valid), want: valid, ok: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizePin(tt.pin)
			if tt.ok {
				if err != nil || got != tt.want {
					t.Fatalf("NormalizePin(%q) = %q, %v", tt.pin, got, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("NormalizePin(%q) = %q", tt.pin, got)
			}
		})
	}
}

func TestSameVersion(t *testing.T) {
	tests := []struct {
		version string
		want    bool
	}{
		{version: "5.15", want: true},
		{version: "5.15.19", want: true},
		{version: "5.3.2", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			if got := SameVersion(tt.version); got != tt.want {
				t.Fatalf("SameVersion(%q) = %t", tt.version, got)
			}
		})
	}
}

func TestFrameworks(t *testing.T) {
	bins := []string{"QtCore", "QtDBus", "QtGui", "QtOpenGL", "QtPrintSupport", "QtSvg", "QtWidgets"}
	tests := []struct {
		name string
		game []string
		want []string
	}{
		{
			name: "qtcore",
			game: []string{"QtCore.framework"},
			want: []string{"QtCore", "QtDBus", "QtSvg"},
		},
		{
			name: "empty",
			want: []string{"QtCore", "QtDBus", "QtGui", "QtOpenGL", "QtPrintSupport", "QtSvg", "QtWidgets"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			game := filepath.Join(root, "game", "Frameworks")
			if err := os.MkdirAll(game, 0o755); err != nil {
				t.Fatal(err)
			}
			for _, name := range tt.game {
				if err := os.MkdirAll(filepath.Join(game, name), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			prefix := filepath.Join(root, "prefix")
			for _, name := range bins {
				bin := filepath.Join(prefix, "Frameworks", name+".framework", "Versions", "5", name)
				if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(bin, []byte(name), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			got, err := Frameworks(t.Context(), game, prefix)
			if err != nil {
				t.Fatal(err)
			}
			want := slices.Clone(tt.want)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Fatalf("got %v want %v", got, want)
			}
		})
	}
}

func TestDependencyDylibs(t *testing.T) {
	common := []string{
		"libqjpeg.dylib",
		"libwebp.1.dylib",
		"libwebpdemux.2.dylib",
		"libpcre.1.dylib",
		"libz.1.dylib",
		"notes.txt",
	}
	tests := []struct {
		name     string
		sharpyuv bool
		want     []string
	}{
		{
			name: "without sharpyuv",
			want: []string{"libpcre.1.dylib", "libz.1.dylib"},
		},
		{
			name:     "with sharpyuv",
			sharpyuv: true,
			want: []string{
				"libpcre.1.dylib",
				"libsharpyuv.0.dylib",
				"libwebp.1.dylib",
				"libwebpdemux.2.dylib",
				"libz.1.dylib",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			names := slices.Clone(common)
			if tt.sharpyuv {
				names = append(names, "libsharpyuv.0.dylib")
			}
			prefix := t.TempDir()
			dir := filepath.Join(prefix, "Frameworks")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			for _, name := range names {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(name), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			got, err := DependencyDylibs(t.Context(), prefix)
			if err != nil {
				t.Fatal(err)
			}
			want := slices.Clone(tt.want)
			slices.Sort(want)
			if !slices.Equal(got, want) {
				t.Fatalf("got %v want %v", got, want)
			}
		})
	}
}

func gzipTar(entries []tarEntry) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(zw)
	for _, entry := range entries {
		typ := entry.typ
		if typ == 0 {
			typ = tar.TypeReg
		}
		hdr := &tar.Header{
			Name:     entry.name,
			Mode:     0o644,
			Typeflag: typ,
			Linkname: entry.link,
		}
		if typ == tar.TypeDir {
			hdr.Mode = 0o755
		}
		reg := typ == tar.TypeReg
		if reg {
			hdr.Size = int64(len(entry.body))
			if entry.size != 0 {
				hdr.Size = entry.size
			}
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, err
		}
		if reg && hdr.Size == int64(len(entry.body)) {
			if _, err := tw.Write(entry.body); err != nil {
				return nil, err
			}
			continue
		}
		if reg && hdr.Size != int64(len(entry.body)) {
			if err := zw.Close(); err != nil {
				return nil, err
			}
			return buf.Bytes(), nil
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func gzipMagic(data []byte) bool {
	return len(data) >= 2 && data[0] == 0x1f && data[1] == 0x8b
}

func TestVerifyFileMissingIsNotExist(t *testing.T) {
	err := VerifyFile(t.Context(), filepath.Join(t.TempDir(), "absent.tar.gz"), PinSHA256)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("VerifyFile = %v", err)
	}
}

var errClose = errors.New("close failed")

type closeFailFile struct{ *os.File }

func (f closeFailFile) Close() error { return errors.Join(f.File.Close(), errClose) }

func TestVerifyFileReturnsCloseError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "archive.tar.gz")
	body := []byte("archive")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	prev := openFile
	t.Cleanup(func() { openFile = prev })
	openFile = func(name string) (fs.File, error) {
		f, err := os.Open(name)
		return closeFailFile{f}, err
	}
	sum := sha256.Sum256(body)
	if err := VerifyFile(t.Context(), path, hex.EncodeToString(sum[:])); !errors.Is(err, errClose) {
		t.Fatalf("VerifyFile = %v", err)
	}
}
