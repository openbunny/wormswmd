package backup

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/openbunny/wormswmd/internal/safe"
)

const (
	metadataHeaderV1      = "# wormswmd backup metadata v1"
	metadataHeaderWormsV1 = "# WormsWMD backup metadata v1"
	metadataHeaderWormsV2 = "# WormsWMD backup metadata v2"
	manifestHeaderV1      = "# wormswmd manifest v1"
	manifestHeaderWormsV1 = "# WormsWMD manifest v1"
	manifestHeaderWormsV2 = "# WormsWMD manifest v2"
	manifestColumnHeader  = "# sha256-or-symlink-digest\tsize\tpath"

	metadataFieldCount = 2
	manifestFieldCount = 3
	sha256HexLen       = sha256.Size * 2

	keyAppPath      = "game_app_path"
	keySource       = "game_source"
	keyExeHash      = "game_executable_sha256"
	keyExeSize      = "game_executable_size"
	keyTreeComplete = "macos_tree_complete"
	keySigPresent   = "code_signature_present"

	sourceSteam   = "steam"
	sourceGOG     = "gog"
	sourceUnknown = "unknown"

	symlinkPrefix = "symlink:"
	boolTrue      = "true"
	boolFalse     = "false"
)

type optionalBool struct {
	present bool
	value   bool
}

type metadata struct {
	appPath string
	source  string
	exeHash string
	exeSize int64
	sig     optionalBool
}

type manifestRow struct {
	digest  string
	size    int64
	path    string
	symlink bool
}

func parseMetadata(data []byte) (metadata, error) {
	vals, err := metadataFields(data)
	if err != nil {
		return metadata{}, err
	}
	return metadataFrom(vals)
}

func metadataFields(data []byte) (map[string]string, error) {
	vals := map[string]string{}
	header := false
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			if metadataHeader(line) {
				header = true
			}
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != metadataFieldCount {
			return nil, fmt.Errorf("backup: metadata line %q does not have two fields", line)
		}
		key, value := parts[0], parts[1]
		if key == "" {
			return nil, fmt.Errorf("backup: metadata line %q has an empty key", line)
		}
		if _, ok := vals[key]; ok {
			return nil, fmt.Errorf("backup: duplicate metadata key %s", key)
		}
		vals[key] = value
	}
	if !header {
		return nil, errors.New("backup: metadata header is missing")
	}
	return vals, nil
}

func metadataFrom(vals map[string]string) (metadata, error) {
	appPath, err := requiredValue(vals, keyAppPath)
	if err != nil {
		return metadata{}, err
	}
	source, err := requiredValue(vals, keySource)
	if err != nil {
		return metadata{}, err
	}
	if err := knownSource(source); err != nil {
		return metadata{}, err
	}
	hashText, err := requiredValue(vals, keyExeHash)
	if err != nil {
		return metadata{}, err
	}
	hash, err := normalizeSHA256(hashText)
	if err != nil {
		return metadata{}, err
	}
	sizeText, err := requiredValue(vals, keyExeSize)
	if err != nil {
		return metadata{}, err
	}
	size, err := parseDecimal(sizeText)
	if err != nil {
		return metadata{}, err
	}
	if _, err := fieldBool(vals, keyTreeComplete, true); err != nil {
		return metadata{}, err
	}
	sig, err := fieldBool(vals, keySigPresent, false)
	if err != nil {
		return metadata{}, err
	}
	return metadata{
		appPath: appPath,
		source:  source,
		exeHash: hash,
		exeSize: size,
		sig:     sig,
	}, nil
}

func requiredValue(vals map[string]string, key string) (string, error) {
	value, ok := vals[key]
	if !ok {
		return "", fmt.Errorf("backup: missing metadata key %s", key)
	}
	return value, nil
}

func knownSource(source string) error {
	switch source {
	case sourceSteam, sourceGOG, sourceUnknown:
		return nil
	default:
		return fmt.Errorf("backup: game_source %q is not steam, gog, or unknown", source)
	}
}

func normalizeSHA256(text string) (string, error) {
	if !isHex(text, sha256HexLen) {
		return "", errors.New("backup: game_executable_sha256 is not 64 hex characters")
	}
	return strings.ToLower(text), nil
}

func fieldBool(vals map[string]string, key string, trueOnly bool) (optionalBool, error) {
	value, ok := vals[key]
	if !ok {
		return optionalBool{}, nil
	}
	switch value {
	case boolTrue:
		return optionalBool{present: true, value: true}, nil
	case boolFalse:
		if trueOnly {
			return optionalBool{}, fmt.Errorf("backup: %s is not true", key)
		}
		return optionalBool{present: true, value: false}, nil
	default:
		if trueOnly {
			return optionalBool{}, fmt.Errorf("backup: %s is not true", key)
		}
		return optionalBool{}, fmt.Errorf("backup: %s is not true or false", key)
	}
}

func parseManifest(data []byte) ([]manifestRow, error) {
	var rows []manifestRow
	seen := map[string]struct{}{}
	header := false
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			if manifestHeader(line) {
				header = true
			}
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) != manifestFieldCount {
			return nil, fmt.Errorf("backup: manifest line %q does not have three fields", line)
		}
		digest, symlink := strings.CutPrefix(parts[0], symlinkPrefix)
		if !isHex(digest, sha256HexLen) {
			return nil, fmt.Errorf("backup: manifest digest %q is not 64 hex characters", parts[0])
		}
		size, err := parseDecimal(parts[1])
		if err != nil {
			return nil, err
		}
		path, err := safe.CleanRel(parts[2])
		if err != nil {
			return nil, fmt.Errorf("backup: %w", err)
		}
		path = filepath.ToSlash(path)
		if _, ok := seen[path]; ok {
			return nil, fmt.Errorf("backup: duplicate manifest path %s", path)
		}
		seen[path] = struct{}{}
		rows = append(rows, manifestRow{
			digest:  strings.ToLower(digest),
			size:    size,
			path:    path,
			symlink: symlink,
		})
	}
	if !header {
		return nil, errors.New("backup: manifest header is missing")
	}
	return rows, nil
}

func parseDecimal(text string) (int64, error) {
	if text == "" {
		return 0, errors.New("backup: size is empty")
	}
	for _, r := range text {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("backup: size %q is not a decimal integer", text)
		}
	}
	n, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("backup: size %q: %w", text, err)
	}
	return n, nil
}

func isHex(text string, width int) bool {
	if len(text) != width {
		return false
	}
	for _, r := range text {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
}

func metadataHeader(line string) bool {
	switch line {
	case metadataHeaderV1, metadataHeaderWormsV1, metadataHeaderWormsV2:
		return true
	default:
		return false
	}
}

func manifestHeader(line string) bool {
	switch line {
	case manifestHeaderV1, manifestHeaderWormsV1, manifestHeaderWormsV2:
		return true
	default:
		return false
	}
}
