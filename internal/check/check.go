package check

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/openbunny/wormswmd/internal/bundle"
	"github.com/openbunny/wormswmd/internal/game"
	"github.com/openbunny/wormswmd/internal/plistfix"
	"github.com/openbunny/wormswmd/internal/qt"
	"github.com/openbunny/wormswmd/internal/run"
)

const (
	ExitReady         = 0
	ExitMissing       = 1
	ExitNotReady      = 2
	MinMajor          = 26
	exitCodeNotExited = -1
	systemAGLPrefix   = "/System/Library/Frameworks/AGL.framework/"
	WindowDomain      = "com.team17.Worms W.M.D"
	scanChunk         = 32 << 10
)

type Report struct {
	App      string   `json:"app,omitempty"`
	Ready    bool     `json:"ready"`
	Exit     int      `json:"exit"`
	Problems []string `json:"problems"`
	Notes    []string `json:"notes"`
}

type Probes struct {
	Exec       run.Exec
	MacOS      string
	Arch       string
	Rosetta    func(context.Context) (bool, error)
	Compiler   func(context.Context) error
	Signature  func(context.Context, string) (bool, error)
	Quarantine func(context.Context, string) (bool, error)
	Window     func(context.Context) (bool, error)
}

func Major(version string) (int, error) {
	version = strings.TrimSpace(version)
	if version == "" {
		return 0, errors.New("check: macOS version is empty")
	}
	head, _, _ := strings.Cut(version, ".")
	n, err := strconv.Atoi(head)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("check: macOS version %q has no numeric major", version)
	}
	return n, nil
}

func Status(code int) string {
	switch code {
	case ExitReady:
		return "ready"
	case ExitMissing:
		return "missing"
	case ExitNotReady:
		return "not-ready"
	default:
		return fmt.Sprintf("exit status %d", code)
	}
}

func Evaluate(ctx context.Context, explicit, home, applications string, probes Probes) (Report, error) {
	if err := ctx.Err(); err != nil {
		return Report{}, fmt.Errorf("check: %w", err)
	}
	app, missing, err := resolve(ctx, explicit, home, applications)
	if err != nil || missing.Exit == ExitMissing || missing.Exit == ExitNotReady {
		return missing, err
	}
	return inspect(ctx, app, probes)
}

func resolve(ctx context.Context, explicit, home, applications string) (string, Report, error) {
	if explicit != "" {
		_, err := os.Lstat(explicit)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return "", missingReport(explicit, explicit+" not found"), nil
			}
			return "", Report{}, fmt.Errorf("check: %w", err)
		}
		if err := game.Valid(ctx, explicit); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return "", Report{}, fmt.Errorf("check: %w", err)
			}
			return "", Report{
				App:      explicit,
				Exit:     ExitNotReady,
				Problems: []string{err.Error()},
				Notes:    []string{},
			}, nil
		}
		return explicit, Report{}, nil
	}
	found, err := game.Resolve(ctx, "", home, applications)
	if err == nil {
		return found, Report{}, nil
	}
	var ambiguous *game.AmbiguousError
	if errors.As(err, &ambiguous) || errors.Is(err, game.ErrNotFound) {
		return "", missingReport("", err.Error()), nil
	}
	return "", Report{}, fmt.Errorf("check: %w", err)
}

func missingReport(app, problem string) Report {
	return Report{
		App:      app,
		Exit:     ExitMissing,
		Problems: []string{problem},
		Notes:    []string{},
	}
}

func inspect(ctx context.Context, app string, probes Probes) (Report, error) {
	report := Report{App: app, Problems: []string{}, Notes: []string{}}
	version := probes.MacOS
	if version == "" {
		out, err := run.Or(probes.Exec)(ctx, "sw_vers", "-productVersion")
		if err != nil {
			return Report{}, fmt.Errorf("check: %w", err)
		}
		version = strings.TrimSpace(string(out))
	}
	major, err := Major(version)
	if err != nil {
		return Report{}, err
	}
	if major < MinMajor {
		report.Notes = append(report.Notes, fmt.Sprintf("macOS %s is below major version %d", version, MinMajor))
	}
	aglOK, err := frameworkReady(app, "AGL")
	if err != nil {
		return Report{}, err
	}
	if !aglOK {
		report.Problems = append(report.Problems, "AGL.framework has no binary")
	}
	qtOK, err := qtReady(ctx, app)
	if err != nil {
		return Report{}, err
	}
	if !qtOK {
		report.Problems = append(report.Problems, "QtCore is not version "+qt.Series)
	}
	gaps, err := BundleGaps(ctx, app, probes.Exec)
	if err != nil {
		return Report{}, err
	}
	report.Problems = append(report.Problems, gaps...)
	arch := probes.Arch
	if arch == "" {
		arch = runtime.GOARCH
	}
	if arch == "arm64" {
		ok, err := rosetta(ctx, probes)
		if err != nil {
			return Report{}, err
		}
		if !ok {
			report.Problems = append(report.Problems, "Rosetta is absent")
		}
	}
	if err := compiler(ctx, probes); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return Report{}, fmt.Errorf("check: %w", err)
		}
		report.Notes = append(report.Notes, "clang: "+err.Error())
	}
	signed, err := signature(ctx, app, probes)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			report.Notes = append(report.Notes, "codesign is absent")
		} else {
			return Report{}, fmt.Errorf("check: %w", err)
		}
	} else if !signed {
		report.Problems = append(report.Problems, "code signature does not verify")
	}
	keys, err := WindowDefaults(ctx, probes)
	if err != nil {
		return Report{}, err
	}
	if keys {
		report.Problems = append(report.Problems, "Qt window defaults remain")
	}
	quarantined, err := quarantine(ctx, app, probes)
	if err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			report.Notes = append(report.Notes, "xattr is absent")
		} else {
			return Report{}, fmt.Errorf("check: %w", err)
		}
	} else if quarantined {
		report.Notes = append(report.Notes, "com.apple.quarantine is present")
	}
	if len(report.Problems) == 0 {
		report.Ready = true
		report.Exit = ExitReady
		return report, nil
	}
	report.Exit = ExitNotReady
	return report, nil
}

func frameworkReady(app, name string) (bool, error) {
	dir := appFramework(app, name)
	_, err := bundle.Binary(dir, name)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, bundle.ErrNoBinary) || errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("check: %w", err)
}

func qtReady(ctx context.Context, app string) (bool, error) {
	dir := appFramework(app, "QtCore")
	bin, err := bundle.Binary(dir, "QtCore")
	if err != nil {
		if errors.Is(err, bundle.ErrNoBinary) || errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("check: %w", err)
	}
	data, err := os.ReadFile(plistBeside(bin))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("check: %w", err)
	}
	xml, err := plistfix.AsXML(ctx, data)
	if err != nil {
		return false, fmt.Errorf("check: %w", err)
	}
	version, err := plistfix.ShortVersion(xml)
	if err != nil {
		return false, fmt.Errorf("check: %w", err)
	}
	return qt.SameVersion(version), nil
}

func appFramework(app, name string) string {
	return filepath.Join(app, "Contents", "Frameworks", name+".framework")
}

func plistBeside(bin string) string {
	return filepath.Join(filepath.Dir(bin), "Resources", "Info.plist")
}

func rosetta(ctx context.Context, probes Probes) (bool, error) {
	if probes.Rosetta != nil {
		ok, err := probes.Rosetta(ctx)
		if err != nil {
			return false, fmt.Errorf("check: %w", err)
		}
		return ok, nil
	}
	_, err := run.Or(probes.Exec)(ctx, "arch", "-x86_64", "/usr/bin/true")
	if err != nil {
		if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, exec.ErrNotFound) {
			return false, fmt.Errorf("check: %w", err)
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() != exitCodeNotExited {
			return false, nil
		}
		return false, fmt.Errorf("check: %w", err)
	}
	return true, nil
}

func compiler(ctx context.Context, probes Probes) error {
	if probes.Compiler != nil {
		return probes.Compiler(ctx)
	}
	_, err := run.Or(probes.Exec)(ctx, "xcrun", "--find", "clang")
	return err
}

func signature(ctx context.Context, app string, probes Probes) (bool, error) {
	if probes.Signature != nil {
		return probes.Signature(ctx, app)
	}
	_, err := run.Or(probes.Exec)(ctx, "codesign", "--verify", "--deep", "--strict", app)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, exec.ErrNotFound) {
		return false, err
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() != exitCodeNotExited {
		return false, nil
	}
	return false, err
}

func quarantine(ctx context.Context, app string, probes Probes) (bool, error) {
	if probes.Quarantine != nil {
		return probes.Quarantine(ctx, app)
	}
	out, err := run.Or(probes.Exec)(ctx, "xattr", "-p", "com.apple.quarantine", app)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, exec.ErrNotFound) {
		return false, err
	}
	if bytes.Contains(out, []byte("No such xattr")) {
		return false, nil
	}
	return false, err
}

func Verified(ctx context.Context, app string, ex run.Exec) (bool, error) {
	return signature(ctx, app, Probes{Exec: ex})
}

func DefaultsMissing(out []byte) bool {
	return bytes.Contains(out, []byte("does not exist")) ||
		bytes.Contains(out, []byte("not found")) ||
		bytes.Contains(out, []byte("Could not find"))
}

func WindowKeys() []string {
	return []string{"QtSystem_GameWindow.geometry", "QtSystem_GameWindow.windowState"}
}

func WindowDefaults(ctx context.Context, probes Probes) (bool, error) {
	if probes.Window != nil {
		return probes.Window(ctx)
	}
	ex := run.Or(probes.Exec)
	for _, key := range WindowKeys() {
		out, err := ex(ctx, "defaults", "read", WindowDomain, key)
		if err == nil {
			return true, nil
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || errors.Is(err, exec.ErrNotFound) {
			return false, fmt.Errorf("check: %w", err)
		}
		if DefaultsMissing(out) {
			continue
		}
		return false, fmt.Errorf("check: %w", err)
	}
	return false, nil
}

func BundleGaps(ctx context.Context, app string, ex run.Exec) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("check: %w", err)
	}
	var problems []string
	plistProblems, err := plistGaps(ctx, app)
	if err != nil {
		return nil, err
	}
	problems = append(problems, plistProblems...)
	for _, rel := range []string{
		filepath.Join("Contents", "Frameworks"),
		filepath.Join("Contents", "PlugIns"),
	} {
		root := filepath.Join(app, rel)
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				if errors.Is(walkErr, fs.ErrNotExist) {
					return nil
				}
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if !d.Type().IsRegular() {
				return nil
			}
			hit, err := fileContains(ctx, path, systemAGLPrefix)
			if err != nil {
				return err
			}
			if hit {
				relPath, err := filepath.Rel(app, path)
				if err != nil {
					return err
				}
				problems = append(problems, "system AGL load command in "+relPath)
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("check: %w", err)
		}
	}
	return problems, nil
}

func plistGaps(ctx context.Context, app string) ([]string, error) {
	path := filepath.Join(app, "Contents", "Info.plist")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []string{"Info.plist is absent"}, nil
		}
		return nil, fmt.Errorf("check: %w", err)
	}
	xml, err := plistfix.AsXML(ctx, data)
	if err != nil {
		return nil, fmt.Errorf("check: %w", err)
	}
	_, keys, err := plistfix.Fix(xml)
	if err != nil {
		return nil, fmt.Errorf("check: %w", err)
	}
	var problems []string
	for _, key := range keys {
		problems = append(problems, "Info.plist needs "+key)
	}
	return problems, nil
}

var openFile = func(name string) (fs.File, error) { return os.Open(name) }

func fileContains(ctx context.Context, path, needle string) (found bool, err error) {
	f, err := openFile(path)
	if err != nil {
		return false, err
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			err = errors.Join(err, fmt.Errorf("check: close %s: %w", path, cerr))
		}
	}()
	buf := make([]byte, scanChunk)
	overlap := max(len(needle)-1, 0)
	var tail []byte
	for {
		if err := ctx.Err(); err != nil {
			return false, fmt.Errorf("check: %w", err)
		}
		n, err := f.Read(buf)
		chunk := make([]byte, 0, len(tail)+n)
		chunk = append(chunk, tail...)
		chunk = append(chunk, buf[:n]...)
		if bytes.Contains(chunk, []byte(needle)) {
			return true, nil
		}
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		tail = bytes.Clone(chunk[max(len(chunk)-overlap, 0):])
	}
}
