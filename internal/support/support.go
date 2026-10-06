package support

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/openbunny/wormswmd/internal/bundle"
	"github.com/openbunny/wormswmd/internal/check"
	"github.com/openbunny/wormswmd/internal/game"
	"github.com/openbunny/wormswmd/internal/plistfix"
	"github.com/openbunny/wormswmd/internal/run"
)

const (
	reportName = "report.txt"
	reportMode = 0o644
	aglName    = "AGL"
	qtName     = "QtCore"
)

type Env struct {
	Version string
	Probes  check.Probes
}

func Write(ctx context.Context, app, home, applications, output string, env Env) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("support: %w", err)
	}
	body, err := report(ctx, app, home, applications, env)
	if err != nil {
		return fmt.Errorf("support: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("support: %w", err)
	}
	if err := writeTar(ctx, output, body); err != nil {
		return err
	}
	slog.Debug("wrote the support report", "output", output, "bytes", len(body))
	slog.Info("Wrote the support report to " + output)
	return nil
}

func report(ctx context.Context, app, home, applications string, env Env) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var b strings.Builder
	if err := writeHost(ctx, &b, env); err != nil {
		return nil, err
	}
	if app == "" {
		resolved, err := game.Resolve(ctx, "", home, applications)
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, errors.Join(ctxErr, err)
			}
			fmt.Fprintf(&b, "resolve: %s\n", err)
			return []byte(b.String()), nil
		}
		app = resolved
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	fmt.Fprintf(&b, "app: %s\n", app)
	section := func(label, value string, err error) error {
		if err == nil {
			fmt.Fprintf(&b, "%s: %s\n", label, value)
			return nil
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return errors.Join(ctxErr, err)
		}
		fmt.Fprintf(&b, "%s: %s\n", label, err)
		return nil
	}
	if err := section("valid", "ok", game.Valid(ctx, app)); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	source, err := game.Source(ctx, app)
	if err := section("source", source, err); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	found, err := aglFound(ctx, app)
	agl := "absent"
	if found {
		agl = "found"
	}
	if err := section("agl", agl, err); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	version, err := qtShortVersion(ctx, app)
	if err := section("qtcore", version, err); err != nil {
		return nil, err
	}
	if err := writeCheck(ctx, &b, app, home, applications, env); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}

func writeHost(ctx context.Context, b *strings.Builder, env Env) error {
	version := env.Version
	if version == "" {
		version = "unknown"
	}
	fmt.Fprintf(b, "wormswmd: %s\n", version)
	macos := env.Probes.MacOS
	if macos == "" {
		out, err := run.Or(env.Probes.Exec)(ctx, "sw_vers", "-productVersion")
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return errors.Join(ctxErr, err)
			}
			fmt.Fprintf(b, "macos: %s\n", err)
		} else {
			macos = strings.TrimSpace(string(out))
		}
	}
	if macos != "" {
		fmt.Fprintf(b, "macos: %s\n", macos)
	}
	arch := env.Probes.Arch
	if arch == "" {
		arch = runtime.GOARCH
	}
	fmt.Fprintf(b, "arch: %s\n", arch)
	return nil
}

func writeCheck(ctx context.Context, b *strings.Builder, app, home, applications string, env Env) error {
	result, err := check.Evaluate(ctx, app, home, applications, env.Probes)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return errors.Join(ctxErr, err)
		}
		fmt.Fprintf(b, "check: %s\n", err)
		return nil
	}
	fmt.Fprintf(b, "check: %s\n", check.Status(result.Exit))
	for _, problem := range result.Problems {
		fmt.Fprintf(b, "problem: %s\n", problem)
	}
	for _, note := range result.Notes {
		fmt.Fprintf(b, "note: %s\n", note)
	}
	return nil
}

func aglFound(ctx context.Context, app string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, fmt.Errorf("support: %w", err)
	}
	dir := filepath.Join(app, "Contents", "Frameworks", aglName+".framework")
	if _, err := bundle.Binary(dir, aglName); err != nil {
		if errors.Is(err, bundle.ErrNoBinary) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func qtShortVersion(ctx context.Context, app string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	dir := filepath.Join(app, "Contents", "Frameworks", qtName+".framework")
	bin, err := bundle.Binary(dir, qtName)
	if err != nil {
		return "", err
	}
	path := filepath.Join(filepath.Dir(bin), "Resources", "Info.plist")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	version, err := plistfix.ShortVersion(data)
	if err != nil {
		return "", err
	}
	if version == "" {
		return "", fmt.Errorf("%s has no %s", path, plistfix.KeyVersion)
	}
	return version, nil
}

func writeTar(ctx context.Context, output string, body []byte) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("support: %w", err)
	}
	f, err := os.Create(output)
	if err != nil {
		return fmt.Errorf("support: %w", err)
	}
	tw := tar.NewWriter(f)
	hdr := &tar.Header{
		Name:     reportName,
		Mode:     reportMode,
		Size:     int64(len(body)),
		ModTime:  time.Unix(0, 0).UTC(),
		Typeflag: tar.TypeReg,
		Format:   tar.FormatPAX,
	}
	writeErr := writeEntry(tw, hdr, body)
	closeErr := tw.Close()
	fileErr := f.Close()
	if writeErr != nil || closeErr != nil || fileErr != nil {
		return fmt.Errorf("support: %w", errors.Join(writeErr, closeErr, fileErr))
	}
	return nil
}

func writeEntry(tw *tar.Writer, hdr *tar.Header, body []byte) error {
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(body)
	return err
}
