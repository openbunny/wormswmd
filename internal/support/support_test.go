package support

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/openbunny/wormswmd/internal/game"
)

const (
	payloadDraws = 20
	payloadLen   = 32
	payloadSeed  = 1
)

func TestWriteOmitsFileBodies(t *testing.T) {
	root := t.TempDir()
	app, err := game.Scaffold(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	const (
		marker          = "WMDFIXMARKER9k3Qm7pL2vX8nR4"
		analyticsMarker = "WMDFIXMARKERA4n8Qc1mR6tY3wB7"
		infoMarker      = "WMDFIXMARKERB5p9Rd2nS7uZ4xC8"
		qtBodyMarker    = "WMDFIXMARKERC6q0Se3pT8vA5yD9"
		qtBinMarker     = "WMDFIXMARKERD7r1Tf4qU9wB6zE0"
		steamMarker     = "WMDFIXMARKERE8s2Ug5rV0xC7aF1"
		qtVersion       = "5.3.2"
		steamUserID     = "1"
		steamAppID      = "327030"
	)
	output := filepath.Join(t.TempDir(), "report.tar")
	applications := t.TempDir()
	exe := game.Executable(app)
	cfg := filepath.Join(app, "Contents", "Resources", "DataOSX", "SteamConfig.txt")
	save := filepath.Join(root, "Library", "Application Support", "Team17", "slot.sav")
	analytics := filepath.Join(app, "Contents", "Resources", "CommonData", "AnalyticsConfig.txt")
	info := filepath.Join(app, "Contents", "Info.plist")
	qtBin := filepath.Join(app, "Contents", "Frameworks", "QtCore.framework", "Versions", "5", "QtCore")
	qtPlist := filepath.Join(filepath.Dir(qtBin), "Resources", "Info.plist")
	steamSave := filepath.Join(root, "Library", "Application Support", "Steam", "userdata", steamUserID, steamAppID, "slot.sav")
	markers := []string{marker, analyticsMarker, infoMarker, qtBodyMarker, qtBinMarker, steamMarker}
	paths := []string{root, app, output, applications, exe, cfg, save, analytics, info, qtBin, qtPlist, steamSave}
	for _, path := range paths {
		for _, m := range markers {
			if strings.Contains(path, m) {
				t.Fatalf("marker appears in path %s", path)
			}
		}
	}
	writes := []struct {
		path   string
		marker string
		mode   os.FileMode
	}{
		{exe, marker, 0o755},
		{cfg, marker, 0o644},
		{save, marker, 0o644},
		{analytics, analyticsMarker, 0o644},
		{info, infoMarker, 0o644},
		{qtBin, qtBinMarker, 0o755},
		{steamSave, steamMarker, 0o644},
	}
	for _, w := range writes {
		if err := os.MkdirAll(filepath.Dir(w.path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(w.path, []byte(w.marker), w.mode); err != nil {
			t.Fatal(err)
		}
	}
	plist, err := os.ReadFile(qtPlist)
	if err != nil {
		t.Fatal(err)
	}
	version := []byte(qtVersion)
	at := bytes.Index(plist, version)
	if at < 0 {
		t.Fatal("qt plist has no version")
	}
	versionClose := []byte("</string>")
	rest := at + len(version)
	if !bytes.HasPrefix(plist[rest:], versionClose) {
		t.Fatal("qt version is not a string value")
	}
	rest += len(versionClose)
	planted := make([]byte, 0, len(plist)+len(qtBodyMarker))
	planted = append(planted, plist[:rest]...)
	planted = append(planted, qtBodyMarker...)
	planted = append(planted, plist[rest:]...)
	if bytes.Index(planted, []byte(qtBodyMarker)) != rest {
		t.Fatal("qt plist marker is not beyond the version string")
	}
	if err := os.WriteFile(qtPlist, planted, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Write(t.Context(), app, root, applications, output); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range markers {
		if bytes.Contains(raw, []byte(m)) {
			t.Fatalf("marker %s present in archive bytes", m)
		}
	}
	if bytes.Contains(raw, []byte("PaxHeaders")) {
		t.Fatal("archive has a second header name")
	}
	tr := tar.NewReader(bytes.NewReader(raw))
	var names []string
	var mod time.Time
	var mode int64
	var size int64
	var body []byte
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, hdr.Name)
		mod = hdr.ModTime
		mode = hdr.Mode
		size = hdr.Size
		entry, err := io.ReadAll(tr)
		if err != nil {
			t.Fatal(err)
		}
		body = append(body, entry...)
	}
	if len(names) != 1 || names[0] != reportName {
		t.Fatalf("names = %#v", names)
	}
	if !mod.Equal(time.Unix(0, 0)) || mod.UnixNano() != 0 {
		t.Fatalf("modtime = %v", mod)
	}
	if mode != reportMode || size != int64(len(body)) {
		t.Fatalf("mode = %#o, size = %d, body = %d", mode, size, len(body))
	}
	if !bytes.Contains(body, []byte(app)) || !bytes.Contains(body, []byte(qtVersion)) || !bytes.Contains(body, []byte("valid: ok")) || !bytes.Contains(body, []byte("agl: absent")) {
		t.Fatalf("body = %s", body)
	}
	for _, m := range markers {
		if bytes.Contains(body, []byte(m)) {
			t.Fatalf("marker %s present in report body", m)
		}
	}
}

func TestWriteOmitsRandomFileBodies(t *testing.T) {
	home := t.TempDir()
	app, err := game.Scaffold(t.Context(), home)
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "report.tar")
	exe := game.Executable(app)
	cfg := filepath.Join(app, "Contents", "Resources", "DataOSX", "SteamConfig.txt")
	save := filepath.Join(home, "Library", "Application Support", "Team17", "slot.sav")
	if err := os.MkdirAll(filepath.Dir(save), 0o755); err != nil {
		t.Fatal(err)
	}
	applications := t.TempDir()
	rng := rand.New(rand.NewPCG(payloadSeed, payloadSeed))
	accepted := 0
	for range payloadDraws {
		payload := make([]byte, payloadLen)
		for i := range payload {
			payload[i] = byte(rng.Uint64())
		}
		if bytes.Contains([]byte(app), payload) || bytes.Contains([]byte(home), payload) || bytes.Contains([]byte(output), payload) || bytes.Contains([]byte(exe), payload) {
			continue
		}
		accepted++
		if err := os.WriteFile(exe, payload, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(cfg, payload, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(save, payload, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := Write(t.Context(), app, home, applications, output); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(output)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(raw, payload) {
			t.Fatal("payload present in archive bytes")
		}
	}
	if accepted == 0 {
		t.Fatal("no payload accepted")
	}
}

func TestWriteResolveFailureStillArchives(t *testing.T) {
	home := t.TempDir()
	output := filepath.Join(home, "out.tar")
	if err := Write(t.Context(), "", home, t.TempDir(), output); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(raw, []byte("not found")) {
		t.Fatalf("archive = %s", raw)
	}
	missing := filepath.Join(home, "missing", "out.tar")
	if err := Write(t.Context(), "", home, t.TempDir(), missing); err == nil {
		t.Fatal("missing output directory succeeded")
	}
}
