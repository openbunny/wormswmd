package backup

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/openbunny/wormswmd/internal/game"
	"github.com/openbunny/wormswmd/internal/safe"
)

const (
	testDirMode  = 0o755
	testFileMode = 0o644
	testExecMode = 0o755
)

func TestCreateVerifyRestore(t *testing.T) {
	ctx := t.Context()
	app, err := game.Scaffold(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	now := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	dir, err := Create(ctx, app, home, "", now)
	if err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.Join(home, "Documents", "WormsWMD-Backup-20200102-030405")
	if dir != wantDir {
		t.Fatalf("Create = %s, want %s", dir, wantDir)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != backupDirMode {
		t.Fatalf("backup mode = %o", info.Mode().Perm())
	}
	if err := Verify(ctx, dir); err != nil {
		t.Fatal(err)
	}
	meta, err := os.ReadFile(filepath.Join(dir, metadataName))
	if err != nil {
		t.Fatal(err)
	}
	text := string(meta)
	if !strings.HasPrefix(text, metadataHeaderV1+"\n") {
		t.Fatalf("metadata header = %q", text)
	}
	for _, line := range []string{
		keyAppPath + "\t" + app,
		keySource + "\t" + sourceUnknown,
		keyTreeComplete + "\t" + boolTrue,
		keySigPresent + "\t" + boolFalse,
	} {
		if !strings.Contains(text, line+"\n") {
			t.Fatalf("metadata missing %s\n%s", line, text)
		}
	}
	manifest, err := os.ReadFile(filepath.Join(dir, manifestName))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(manifest), manifestHeaderV1+"\n"+manifestColumnHeader+"\n") {
		t.Fatalf("manifest header = %q", manifest)
	}
	for line := range strings.SplitSeq(string(manifest), "\n") {
		if strings.HasSuffix(line, "\t"+manifestName) {
			t.Fatalf("manifest lists itself: %s", line)
		}
	}
	plistPath := filepath.Join(app, "Contents", fileInfoPlist)
	original, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatal(err)
	}
	putFile(t, plistPath, "changed")
	sig := filepath.Join(app, "Contents", dirCodeSignature)
	if err := os.Mkdir(sig, testDirMode); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(ctx, dir, app, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(plistPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("plist = %q, want %q", got, original)
	}
	assertAbsent(t, sig)
	assertBackupTrees(t, app, dir)
}

func TestRestoreUsesMetadataPathWhenAppEmpty(t *testing.T) {
	ctx := t.Context()
	app, dir := newBackup(t)
	exe := mutateExe(t, app)
	restored, err := Restore(ctx, dir, "", false)
	if err != nil {
		t.Fatal(err)
	}
	if restored != app {
		t.Fatalf("Restore = %q, want the recorded app %q", restored, app)
	}
	body, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(body, []byte("game")) {
		t.Fatalf("executable = %q", body)
	}
}

func TestRestoreRejectsEmptyPath(t *testing.T) {
	ctx := t.Context()
	app, dir := newBackup(t)
	setAppPath(t, dir, "")
	exe := mutateExe(t, app)
	t.Chdir(t.TempDir())
	if _, err := Restore(ctx, dir, "", true); err == nil {
		t.Fatal("Restore accepted an empty path")
	}
	assertAbsent(t, "Contents")
	body, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "mutated" {
		t.Fatalf("executable = %q", body)
	}
}

func TestRestoreForceWhenAppPathDiffers(t *testing.T) {
	ctx := t.Context()
	app, dir := newBackup(t)
	other := filepath.Join(t.TempDir(), "Elsewhere.app")
	setAppPath(t, dir, other)
	exe := mutateExe(t, app)
	_, err := Restore(ctx, dir, app, false)
	if err == nil || !strings.Contains(err.Error(), "pass --force") || !strings.Contains(err.Error(), app) || !strings.Contains(err.Error(), other) {
		t.Fatalf("Restore = %v", err)
	}
	body, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "mutated" {
		t.Fatalf("executable = %q", body)
	}
	if _, err := Restore(ctx, dir, app, true); err != nil {
		t.Fatal(err)
	}
	body, err = os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "game" {
		t.Fatalf("executable = %q", body)
	}
}

func TestRelativeSymlinkRoundTrip(t *testing.T) {
	ctx := t.Context()
	app, err := game.Scaffold(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	target := "QtCore.framework/Versions/5/QtCore"
	link := filepath.Join(app, "Contents", "Frameworks", "QtCore.link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	dir, err := Create(ctx, app, t.TempDir(), filepath.Join(t.TempDir(), "backup"), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(filepath.Join(dir, manifestName))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(target))
	line := fmt.Sprintf("symlink:%s\t%d\tFrameworks/QtCore.link", hex.EncodeToString(sum[:]), len(target))
	if !slices.Contains(strings.Split(string(manifest), "\n"), line) {
		t.Fatalf("manifest missing %s\n%s", line, manifest)
	}
	gotLink, err := os.Readlink(filepath.Join(dir, "Frameworks", "QtCore.link"))
	if err != nil || gotLink != target {
		t.Fatalf("backup link = %q, %v", gotLink, err)
	}
	if err := Verify(ctx, dir); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(ctx, dir, app, false); err != nil {
		t.Fatal(err)
	}
	gotLink, err = os.Readlink(link)
	if err != nil || gotLink != target {
		t.Fatalf("restored link = %q, %v", gotLink, err)
	}
}

func TestRestoreLeavesAppWhenLaterTreeCannotCopy(t *testing.T) {
	ctx := t.Context()
	app, dir := newBackup(t)
	exe := mutateExe(t, app)
	if err := os.Symlink("/tmp/outside", filepath.Join(dir, dirPlugIns, "abs")); err != nil {
		t.Fatal(err)
	}
	if err := writeManifest(ctx, dir); err != nil {
		t.Fatal(err)
	}
	_, err := Restore(ctx, dir, app, false)
	if err == nil {
		t.Fatal("Restore accepted an absolute symlink")
	}
	body, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "mutated" {
		t.Fatalf("executable = %q", body)
	}
}

func TestRestoreRejectsEscapingContents(t *testing.T) {
	ctx := t.Context()
	app, dir := newBackup(t)
	outside := t.TempDir()
	marker := filepath.Join(outside, dirFrameworks, "marker")
	if err := os.MkdirAll(filepath.Dir(marker), testDirMode); err != nil {
		t.Fatal(err)
	}
	putFile(t, marker, "keep")
	contents := filepath.Join(app, dirContents)
	if err := os.Rename(contents, contents+".held"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, contents); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(ctx, dir, app, false); err == nil {
		t.Fatal("Restore followed Contents")
	}
	body, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "keep" {
		t.Fatalf("marker = %q", body)
	}
}

func TestRestoreReplacesPlistSymlink(t *testing.T) {
	ctx := t.Context()
	app, dir := newBackup(t)
	outside := filepath.Join(t.TempDir(), "plist")
	putFile(t, outside, "outside")
	plist := filepath.Join(app, dirContents, fileInfoPlist)
	if err := os.Remove(plist); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, plist); err != nil {
		t.Fatal(err)
	}
	if _, err := Restore(ctx, dir, app, false); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(plist)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		t.Fatalf("plist mode = %v", info.Mode())
	}
	body, err := os.ReadFile(outside)
	if err != nil || string(body) != "outside" {
		t.Fatalf("outside plist = %q, %v", body, err)
	}
	got, err := os.ReadFile(filepath.Join(dir, fileInfoPlist))
	if err != nil {
		t.Fatal(err)
	}
	restored, err := os.ReadFile(plist)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(restored, got) {
		t.Fatalf("plist = %q, want %q", restored, got)
	}
}

func TestCreateRejects(t *testing.T) {
	for _, tc := range []struct {
		name string
		prep func(t *testing.T, app string) string
		want string
	}{
		{
			name: "optional file",
			prep: func(t *testing.T, app string) string {
				data := filepath.Join(app, dirContents, dirResources, dirDataOSX)
				if err := os.RemoveAll(data); err != nil {
					t.Fatal(err)
				}
				putFile(t, data, "file")
				return filepath.Join(t.TempDir(), "backup")
			},
			want: "not a directory",
		},
		{
			name: "absolute symlink",
			prep: func(t *testing.T, app string) string {
				if err := os.Symlink("/tmp/outside", filepath.Join(app, dirContents, dirFrameworks, "abs")); err != nil {
					t.Fatal(err)
				}
				return filepath.Join(t.TempDir(), "backup")
			},
		},
		{
			name: "backup inside app",
			prep: func(_ *testing.T, app string) string {
				return filepath.Join(app, dirContents, "backup")
			},
			want: "is inside",
		},
		{
			name: "backup behind a symlink into app",
			prep: func(t *testing.T, app string) string {
				link := filepath.Join(t.TempDir(), "link")
				if err := os.Symlink(app, link); err != nil {
					t.Fatal(err)
				}
				return filepath.Join(link, "backup")
			},
			want: "is inside",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, err := game.Scaffold(t.Context(), t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			dest := tc.prep(t, app)
			_, err = Create(t.Context(), app, t.TempDir(), dest, time.Time{})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Create = %v", err)
			}
			assertAbsent(t, dest)
		})
	}
}

func TestCreateRejectsContentsSymlink(t *testing.T) {
	ctx := t.Context()
	app, err := game.Scaffold(ctx, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside")
	contents := filepath.Join(app, dirContents)
	if err := os.Rename(contents, outside); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, contents); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(outside, dirFrameworks, "marker")
	putFile(t, marker, "keep")
	dest := filepath.Join(t.TempDir(), "backup")
	_, err = Create(ctx, app, t.TempDir(), dest, time.Time{})
	if err == nil || !strings.Contains(err.Error(), "leaves") {
		t.Fatalf("Create = %v", err)
	}
	body, err := os.ReadFile(marker)
	if err != nil || string(body) != "keep" {
		t.Fatalf("marker = %q, %v", body, err)
	}
	assertAbsent(t, dest)
}

func TestVerifyRejectsMissingAndExtra(t *testing.T) {
	ctx := t.Context()
	_, dir := newBackup(t)
	man := filepath.Join(dir, manifestName)
	f, err := os.OpenFile(man, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fmt.Fprintf(f, "%s\t1\tmissing.txt\n", strings.Repeat("ab", sha256HexLen/2)); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	err = Verify(ctx, dir)
	if err == nil || !strings.Contains(err.Error(), "missing.txt") {
		t.Fatalf("Verify = %v", err)
	}

	_, dir = newBackup(t)
	putFile(t, filepath.Join(dir, "extra.txt"), "x")
	err = Verify(ctx, dir)
	if err == nil || !strings.Contains(err.Error(), "extra.txt") {
		t.Fatalf("Verify = %v", err)
	}
}

func TestVerifyRejectsHardLink(t *testing.T) {
	for _, name := range []string{fileInfoPlist, manifestName} {
		t.Run(name, func(t *testing.T) {
			_, dir := newBackup(t)
			if err := os.Link(filepath.Join(dir, name), filepath.Join(filepath.Dir(dir), "linked")); err != nil {
				t.Fatal(err)
			}
			err := Verify(t.Context(), dir)
			if err == nil || !strings.Contains(err.Error(), "hard link") {
				t.Fatalf("Verify = %v", err)
			}
		})
	}
}

func TestVerifyNamesFirstUnlistedPath(t *testing.T) {
	ctx := t.Context()
	_, dir := newBackup(t)
	const first = "a-unlisted.txt"
	const later = "m-unlisted.txt"
	for _, name := range []string{later, first} {
		putFile(t, filepath.Join(dir, name), "x")
	}
	err := Verify(ctx, dir)
	want := "backup: unlisted path " + first + ";"
	if err == nil || !strings.HasPrefix(err.Error(), want) {
		t.Fatalf("Verify = %v, want %s", err, want)
	}
}

func TestDuplicateMetadataKey(t *testing.T) {
	ctx := t.Context()
	_, dir := newBackup(t)
	meta := filepath.Join(dir, metadataName)
	f, err := os.OpenFile(meta, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(keySource + "\t" + sourceSteam + "\n"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	err = Verify(ctx, dir)
	if err == nil || !strings.Contains(err.Error(), "duplicate metadata key "+keySource) {
		t.Fatalf("Verify = %v", err)
	}
}

func TestUpstreamHeaders(t *testing.T) {
	ctx := t.Context()
	_, dir := newBackup(t)
	metaPath := filepath.Join(dir, metadataName)
	meta, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(meta)
	old := metadataHeaderV1 + "\n"
	if !strings.HasPrefix(text, old) {
		t.Fatalf("metadata header = %q", text)
	}
	text = metadataHeaderWormsV2 + "\n" + text[len(old):]
	if err := os.WriteFile(metaPath, []byte(text), backupFileMode); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(text))
	digest := hex.EncodeToString(sum[:])
	manPath := filepath.Join(dir, manifestName)
	man, err := os.ReadFile(manPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(man), "\n")
	lines[0] = manifestHeaderWormsV1
	found := false
	for i, line := range lines {
		if strings.HasSuffix(line, "\t"+metadataName) {
			lines[i] = fmt.Sprintf("%s\t%d\t%s", digest, len(text), metadataName)
			found = true
		}
	}
	if !found {
		t.Fatal("metadata row missing")
	}
	updated := strings.Join(lines, "\n")
	if !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}
	updated += "# comment\n\n"
	if err := os.WriteFile(manPath, []byte(updated), backupFileMode); err != nil {
		t.Fatal(err)
	}
	if err := Verify(ctx, dir); err != nil {
		t.Fatal(err)
	}
}

func TestParseMetadata(t *testing.T) {
	hash := strings.Repeat("ab", sha256HexLen/2)
	good := metadataHeaderWormsV1 + "\n" +
		keyAppPath + "\t/Games/Worms W.M.D.app\n" +
		keySource + "\t" + sourceGOG + "\n" +
		keyExeHash + "\t" + hash + "\n" +
		keyExeSize + "\t4\n" +
		"extra\tok\n"
	meta, err := parseMetadata([]byte(good))
	if err != nil {
		t.Fatal(err)
	}
	if meta.source != sourceGOG || meta.exeSize != 4 || meta.exeHash != hash || meta.sig.present || meta.appPath != "/Games/Worms W.M.D.app" {
		t.Fatalf("metadata = %+v", meta)
	}
	withSig := good + keySigPresent + "\t" + boolTrue + "\n" + keyTreeComplete + "\t" + boolTrue + "\n"
	meta, err = parseMetadata([]byte(withSig))
	if err != nil || !meta.sig.present || !meta.sig.value {
		t.Fatalf("sig metadata = %+v, %v", meta, err)
	}
	withFalse := good + keySigPresent + "\t" + boolFalse + "\n"
	meta, err = parseMetadata([]byte(withFalse))
	if err != nil || !meta.sig.present || meta.sig.value {
		t.Fatalf("false sig = %+v, %v", meta, err)
	}
	bad := []string{
		strings.Replace(good, metadataHeaderWormsV1+"\n", "", 1),
		strings.Replace(good, sourceGOG, "other", 1),
		strings.Replace(good, hash, "abc", 1),
		strings.Replace(good, "\t4\n", "\t-1\n", 1),
		good + keySource + "\t" + sourceSteam + "\n",
		good + keyTreeComplete + "\t" + boolFalse + "\n",
		good + keySigPresent + "\tyes\n",
		strings.Replace(good, keyAppPath+"\t/Games/Worms W.M.D.app\n", "", 1),
	}
	for _, text := range bad {
		if _, err := parseMetadata([]byte(text)); err == nil {
			t.Fatalf("accepted %q", text)
		}
	}
}

func FuzzParseMetadata(f *testing.F) {
	hash := strings.Repeat("ab", sha256HexLen/2)
	good := metadataHeaderWormsV1 + "\n" +
		keyAppPath + "\t/Games/Worms W.M.D.app\n" +
		keySource + "\t" + sourceGOG + "\n" +
		keyExeHash + "\t" + hash + "\n" +
		keyExeSize + "\t4\n" +
		"extra\tok\n"
	f.Add([]byte(good))
	f.Add([]byte(good + keySigPresent + "\t" + boolTrue + "\n" + keyTreeComplete + "\t" + boolTrue + "\n"))
	f.Add([]byte(good + keySigPresent + "\t" + boolFalse + "\n"))
	f.Add([]byte("# x"))
	f.Fuzz(func(t *testing.T, data []byte) {
		meta, err := parseMetadata(data)
		if err != nil {
			return
		}
		if err := knownSource(meta.source); err != nil {
			t.Fatalf("source %q: %v", meta.source, err)
		}
		if len(meta.exeHash) != sha256HexLen {
			t.Fatalf("hash %q", meta.exeHash)
		}
		if meta.exeSize < 0 {
			t.Fatalf("size %d", meta.exeSize)
		}
	})
}

func FuzzManifest(f *testing.F) {
	hash := strings.Repeat("ab", sha256HexLen/2)
	upper := strings.Repeat("AB", sha256HexLen/2)
	row := hash + "\t1\ta\n"
	f.Add([]byte(manifestHeaderV1 + "\n" + manifestColumnHeader + "\n"))
	f.Add([]byte(manifestHeaderWormsV1 + "\n" + manifestHeaderWormsV2 + "\n\n"))
	f.Add([]byte(""))
	f.Add([]byte("no-header\t1\tfile\n"))
	f.Add([]byte(manifestHeaderV1 + "\n" + hash + "\t0\tMacOS/Worms W.M.D\nsymlink:" + hash + "\t3\tFrameworks/link\n"))
	f.Add([]byte(manifestHeaderV1 + "\n" + upper + "\t0\tMacOS/Worms W.M.D\r\n"))
	f.Add([]byte(manifestHeaderV1 + "\n" + row + row))
	f.Add([]byte(manifestHeaderV1 + "\n" + hash + "\t1\tfoo/bar\n" + hash + "\t1\tfoo/./bar\n"))
	f.Add([]byte(manifestHeaderV1 + "\nnot-enough-fields\n"))
	f.Add([]byte{0xff, 0xfe, '\n', '\t'})
	f.Fuzz(func(t *testing.T, data []byte) {
		rows, err := parseManifest(data)
		if err != nil {
			return
		}
		seen := map[string]struct{}{}
		for _, row := range rows {
			if _, ok := seen[row.path]; ok {
				t.Fatalf("duplicate path %s", row.path)
			}
			seen[row.path] = struct{}{}
			if len(row.digest) != sha256HexLen {
				t.Fatalf("digest %q", row.digest)
			}
			for _, r := range row.digest {
				if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
					t.Fatalf("digest %q", row.digest)
				}
			}
			if row.size < 0 {
				t.Fatalf("size %d", row.size)
			}
			if _, err := safe.CleanRel(row.path); err != nil {
				t.Fatalf("path %s: %v", row.path, err)
			}
		}
	})
}

func TestAbortRemovesFreshPastFailure(t *testing.T) {
	requireChflags(t)
	ctx := t.Context()
	app, dir := newBackup(t)
	releaseImmutable(t, app)
	exe := mutateExe(t, app)
	frameworks := filepath.Join(app, dirContents, dirFrameworks)
	setImmutable(t, frameworks)
	_, err := Restore(ctx, dir, app, false)
	if err == nil {
		t.Fatal("Restore succeeded")
	}
	if got := prefixedPaths(t, app, ".wormswmd-restore-"); len(got) != 0 {
		t.Fatalf("restore temps = %v", got)
	}
	if got := prefixedPaths(t, app, ".wormswmd-old-"); len(got) != 0 {
		t.Fatalf("old temps = %v", got)
	}
	body, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "mutated" {
		t.Fatalf("executable = %q", body)
	}
	qt, err := os.ReadFile(filepath.Join(frameworks, "QtCore.framework", "Versions", "5", "QtCore"))
	if err != nil {
		t.Fatal(err)
	}
	if string(qt) != "qtcore" {
		t.Fatalf("QtCore = %q", qt)
	}
}

func TestPriorRenameKeepsBothCopies(t *testing.T) {
	ctx := t.Context()
	app, dir := newBackup(t)
	frameworks := filepath.Join(app, dirContents, dirFrameworks)
	putFile(t, filepath.Join(frameworks, "previous.txt"), "previous")
	exe := mutateExe(t, app)
	orig := renamePath
	t.Cleanup(func() { renamePath = orig })
	renamePath = func(oldpath, newpath string) error {
		oldBase := filepath.Base(oldpath)
		if filepath.Base(newpath) != dirFrameworks {
			return orig(oldpath, newpath)
		}
		if strings.HasPrefix(oldBase, ".wormswmd-restore-") || strings.HasPrefix(oldBase, ".wormswmd-old-") {
			return fmt.Errorf("rename %s to %s blocked", oldpath, newpath)
		}
		return orig(oldpath, newpath)
	}
	_, restoreErr := Restore(ctx, dir, app, false)
	if restoreErr == nil {
		t.Fatal("Restore succeeded")
	}
	restores := prefixedPaths(t, app, ".wormswmd-restore-")
	priors := prefixedPaths(t, app, ".wormswmd-old-")
	if len(restores) != 1 || len(priors) != 1 {
		t.Fatalf("restores = %v, priors = %v", restores, priors)
	}
	assertAbsent(t, frameworks)
	prev, err := os.ReadFile(filepath.Join(priors[0], "previous.txt"))
	if err != nil || string(prev) != "previous" {
		t.Fatalf("previous = %q, %v", prev, err)
	}
	assertAbsent(t, filepath.Join(restores[0], "previous.txt"))
	if _, statErr := os.Lstat(filepath.Join(restores[0], "QtCore.framework")); statErr != nil {
		t.Fatalf("replacement = %v", statErr)
	}
	body, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "mutated" {
		t.Fatalf("executable = %q", body)
	}
	text := restoreErr.Error()
	if !strings.Contains(text, priors[0]) || !strings.Contains(text, frameworks) {
		t.Fatalf("Restore = %v", restoreErr)
	}
}

func TestRestoreRemovesSignatureBeforeSpares(t *testing.T) {
	requireChflags(t)
	ctx := t.Context()
	app, dir := newBackup(t)
	releaseImmutable(t, app)
	exe := mutateExe(t, app)
	marker := filepath.Join(app, dirContents, dirFrameworks, "immutable.txt")
	putFile(t, marker, "old")
	setImmutable(t, marker)
	sig := filepath.Join(app, dirContents, dirCodeSignature)
	if err := os.Mkdir(sig, testDirMode); err != nil {
		t.Fatal(err)
	}
	_, err := Restore(ctx, dir, app, false)
	if err == nil || !strings.Contains(err.Error(), "remove previous") {
		t.Fatalf("Restore = %v", err)
	}
	assertAbsent(t, sig)
	body, err := os.ReadFile(exe)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "game" {
		t.Fatalf("executable = %q", body)
	}
	assertAbsent(t, marker)
	spares := prefixedPaths(t, app, ".wormswmd-old-")
	if len(spares) != 1 {
		t.Fatalf("spares = %v", spares)
	}
	kept, err := os.ReadFile(filepath.Join(spares[0], "immutable.txt"))
	if err != nil || string(kept) != "old" {
		t.Fatalf("spare marker = %q, %v", kept, err)
	}
	if got := prefixedPaths(t, app, ".wormswmd-restore-"); len(got) != 0 {
		t.Fatalf("restore temps = %v", got)
	}
}

func TestRestoreUndoesSwapsWhenSignatureFails(t *testing.T) {
	requireChflags(t)
	for _, tc := range []struct {
		name   string
		inject func(t *testing.T, app, sig string)
	}{
		{
			name: "removal",
			inject: func(t *testing.T, app, sig string) {
				releaseImmutable(t, app)
				setImmutable(t, sig)
			},
		},
		{
			name: "rename",
			inject: func(t *testing.T, _, sig string) {
				orig := renamePath
				t.Cleanup(func() { renamePath = orig })
				renamePath = func(oldpath, newpath string) error {
					if oldpath == sig {
						return errors.New("rename blocked")
					}
					return orig(oldpath, newpath)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app, dir := newBackup(t)
			exe := mutateExe(t, app)
			sig := filepath.Join(app, dirContents, dirCodeSignature)
			putTree(t, sig, "CodeResources", "sig")
			tc.inject(t, app, sig)
			if _, err := Restore(t.Context(), dir, app, false); err == nil {
				t.Fatal("Restore succeeded")
			}
			body, err := os.ReadFile(exe)
			if err != nil {
				t.Fatal(err)
			}
			if string(body) != "mutated" {
				t.Fatalf("executable = %q", body)
			}
			got, err := os.ReadFile(filepath.Join(sig, "CodeResources"))
			if err != nil || string(got) != "sig" {
				t.Fatalf("signature = %q, %v", got, err)
			}
			if temps := prefixedPaths(t, app, ".wormswmd-restore-"); len(temps) != 0 {
				t.Fatalf("restore temps = %v", temps)
			}
			if temps := prefixedPaths(t, app, ".wormswmd-old-"); len(temps) != 0 {
				t.Fatalf("old temps = %v", temps)
			}
		})
	}
}

func TestUndoKeepsReplacementWhenPriorRenameFails(t *testing.T) {
	requireChflags(t)
	root := t.TempDir()
	releaseImmutable(t, root)
	dst := filepath.Join(root, "Frameworks")
	putTree(t, dst, "new.txt", "replacement")
	prior := filepath.Join(root, "previous")
	putTree(t, prior, "old.txt", "previous")
	setImmutable(t, prior)
	undoErr := undoStaged(t.Context(), []staged{{dst: dst, prior: prior, placed: true}}, nil)
	if undoErr == nil {
		t.Fatal("undo succeeded")
	}
	body, err := os.ReadFile(filepath.Join(dst, "new.txt"))
	if err != nil || string(body) != "replacement" {
		t.Fatalf("replacement = %q, %v", body, err)
	}
	body, err = os.ReadFile(filepath.Join(prior, "old.txt"))
	if err != nil || string(body) != "previous" {
		t.Fatalf("previous = %q, %v", body, err)
	}
	if got := prefixedPaths(t, root, ".wormswmd-old-"); len(got) != 0 {
		t.Fatalf("held temps = %v", got)
	}
	text := undoErr.Error()
	if !strings.Contains(text, prior) || !strings.Contains(text, dst) {
		t.Fatalf("undo = %v", undoErr)
	}
}

func TestUndoKeepsBothCopiesWhenRenamesFail(t *testing.T) {
	root := t.TempDir()
	dst := filepath.Join(root, "Frameworks")
	putTree(t, dst, "new.txt", "replacement")
	prior := filepath.Join(root, "previous")
	putTree(t, prior, "old.txt", "previous")
	orig := renamePath
	t.Cleanup(func() { renamePath = orig })
	renamePath = func(oldpath, newpath string) error {
		if newpath == dst {
			return fmt.Errorf("rename %s to %s blocked", oldpath, newpath)
		}
		return orig(oldpath, newpath)
	}
	undoErr := undoStaged(t.Context(), []staged{{dst: dst, prior: prior, placed: true}}, nil)
	if undoErr == nil {
		t.Fatal("undo succeeded")
	}
	assertAbsent(t, dst)
	body, err := os.ReadFile(filepath.Join(prior, "old.txt"))
	if err != nil || string(body) != "previous" {
		t.Fatalf("previous = %q, %v", body, err)
	}
	helds := prefixedPaths(t, root, ".wormswmd-old-")
	if len(helds) != 1 {
		t.Fatalf("held = %v", helds)
	}
	body, err = os.ReadFile(filepath.Join(helds[0], "new.txt"))
	if err != nil || string(body) != "replacement" {
		t.Fatalf("replacement = %q, %v", body, err)
	}
	text := undoErr.Error()
	if !strings.Contains(text, prior) || !strings.Contains(text, dst) || !strings.Contains(text, helds[0]) {
		t.Fatalf("undo = %v", undoErr)
	}
	if !strings.Contains(text, "pre-restore copy") || !strings.Contains(text, "backup copy") {
		t.Fatalf("undo = %v", undoErr)
	}
}

func TestUndoStopsWhenContextIsCanceled(t *testing.T) {
	root := t.TempDir()
	dst := filepath.Join(root, dirFrameworks)
	putTree(t, dst, "new.txt", "backup")
	prior := filepath.Join(root, "previous")
	putTree(t, prior, "old.txt", "pre")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	undoErr := undoStaged(ctx, []staged{{dst: dst, prior: prior, placed: true}}, nil)
	if !errors.Is(undoErr, context.Canceled) {
		t.Fatalf("undo = %v", undoErr)
	}
	body, err := os.ReadFile(filepath.Join(dst, "new.txt"))
	if err != nil || string(body) != "backup" {
		t.Fatalf("backup copy = %q, %v", body, err)
	}
	if !strings.Contains(undoErr.Error(), dst) || !strings.Contains(undoErr.Error(), "backup copy") {
		t.Fatalf("undo = %v", undoErr)
	}
}

func TestUndoStopsAfterRetainCopies(t *testing.T) {
	root := t.TempDir()
	frameworks := filepath.Join(root, dirFrameworks)
	plugins := filepath.Join(root, dirPlugIns)
	for _, dir := range []string{frameworks, plugins} {
		putTree(t, dir, "new.txt", "backup")
	}
	priorFrameworks := filepath.Join(root, "previous-frameworks")
	priorPlugins := filepath.Join(root, "previous-plugins")
	for _, dir := range []string{priorFrameworks, priorPlugins} {
		putTree(t, dir, "old.txt", "pre")
	}
	orig := renamePath
	t.Cleanup(func() { renamePath = orig })
	renamePath = func(oldpath, newpath string) error {
		if newpath == plugins {
			return errors.New("rename blocked")
		}
		return orig(oldpath, newpath)
	}
	undoErr := undoStaged(t.Context(), []staged{
		{dst: frameworks, prior: priorFrameworks, placed: true},
		{dst: plugins, prior: priorPlugins, placed: true},
	}, nil)
	if undoErr == nil {
		t.Fatal("undo succeeded")
	}
	body, err := os.ReadFile(filepath.Join(frameworks, "new.txt"))
	if err != nil || string(body) != "backup" {
		t.Fatalf("frameworks = %q, %v", body, err)
	}
	body, err = os.ReadFile(filepath.Join(priorFrameworks, "old.txt"))
	if err != nil || string(body) != "pre" {
		t.Fatalf("pre-restore frameworks = %q, %v", body, err)
	}
	assertAbsent(t, plugins)
	if !strings.Contains(undoErr.Error(), "pre-restore copy") || !strings.Contains(undoErr.Error(), "backup copy") {
		t.Fatalf("undo = %v", undoErr)
	}
}

func assertBackupTrees(t *testing.T, app, dir string) {
	t.Helper()
	pairs := []struct{ appRel, backupRel string }{
		{filepath.Join(dirContents, dirFrameworks), dirFrameworks},
		{filepath.Join(dirContents, dirPlugIns), dirPlugIns},
		{filepath.Join(dirContents, dirMacOS), dirMacOS},
		{filepath.Join(dirContents, fileInfoPlist), fileInfoPlist},
		{filepath.Join(dirContents, dirResources, dirDataOSX), dirDataOSX},
		{filepath.Join(dirContents, dirResources, dirCommonData), dirCommonData},
	}
	for _, pair := range pairs {
		sameTree(t, filepath.Join(app, pair.appRel), filepath.Join(dir, pair.backupRel))
	}
}

func sameTree(t *testing.T, left, right string) {
	t.Helper()
	got := treeEntries(t, left)
	want := treeEntries(t, right)
	for _, rel := range slices.Sorted(maps.Keys(got)) {
		if want[rel] != got[rel] {
			t.Fatalf("%s: %q, backup %q", rel, got[rel], want[rel])
		}
	}
	for _, rel := range slices.Sorted(maps.Keys(want)) {
		if _, ok := got[rel]; !ok {
			t.Fatalf("missing %s", rel)
		}
	}
}

func treeEntries(t *testing.T, root string) map[string]string {
	t.Helper()
	info, err := os.Lstat(root)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	if !info.IsDir() {
		out["."] = entryText(t, root, info)
		return out
	}
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		st, err := d.Info()
		if err != nil {
			return err
		}
		out[rel] = entryText(t, path, st)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func entryText(t *testing.T, path string, info os.FileInfo) string {
	t.Helper()
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		if err != nil {
			t.Fatal(err)
		}
		return "link:" + target
	}
	if info.IsDir() {
		return "dir"
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return "file:" + string(body)
}

func mutateExe(t *testing.T, app string) string {
	t.Helper()
	exe := game.Executable(app)
	if err := os.WriteFile(exe, []byte("mutated"), testExecMode); err != nil {
		t.Fatal(err)
	}
	return exe
}

func putFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), testFileMode); err != nil {
		t.Fatal(err)
	}
}

func putTree(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.Mkdir(dir, testDirMode); err != nil {
		t.Fatal(err)
	}
	putFile(t, filepath.Join(dir, name), body)
}

func assertAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("%s lstat error = %v", path, err)
	}
}

func newBackup(t *testing.T) (string, string) {
	t.Helper()
	app, err := game.Scaffold(t.Context(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dir, err := Create(t.Context(), app, t.TempDir(), filepath.Join(t.TempDir(), "backup"), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	return app, dir
}

func setAppPath(t *testing.T, dir, path string) {
	t.Helper()
	metaPath := filepath.Join(dir, metadataName)
	data, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(string(data), "\n")
	found := false
	prefix := keyAppPath + "\t"
	for i, line := range lines {
		if strings.HasPrefix(line, prefix) {
			lines[i] = prefix + path
			found = true
		}
	}
	if !found {
		t.Fatal("game_app_path missing")
	}
	if err := os.WriteFile(metaPath, []byte(strings.Join(lines, "\n")), backupFileMode); err != nil {
		t.Fatal(err)
	}
	if err := writeManifest(t.Context(), dir); err != nil {
		t.Fatal(err)
	}
}

func prefixedPaths(t *testing.T, root, prefix string) []string {
	t.Helper()
	var found []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !strings.HasPrefix(d.Name(), prefix) {
			return nil
		}
		found = append(found, path)
		if d.IsDir() {
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return found
}
