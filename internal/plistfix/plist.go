package plistfix

import (
	"bytes"
	"context"
	"errors"
	"fmt"

	"howett.net/plist"
)

const (
	bundleID     = "com.team17.wormswmd"
	minSystem    = "10.13"
	legacyMin    = "10.8"
	keyBundleID  = "CFBundleIdentifier"
	keyHiDPI     = "NSHighResolutionCapable"
	keySwitch    = "NSSupportsAutomaticGraphicsSwitching"
	keyMinSystem = "LSMinimumSystemVersion"
	KeyVersion   = "CFBundleShortVersionString"
	bplistMagic  = "bplist"
)

func Fix(data []byte) ([]byte, []string, error) {
	root, err := decodeDict(data)
	if err != nil {
		return nil, nil, err
	}
	var changes []string
	if replaceString(root, keyBundleID, bundleID, func(s string) bool { return s == "" }) {
		changes = append(changes, keyBundleID)
	}
	if replaceTrue(root, keyHiDPI) {
		changes = append(changes, keyHiDPI)
	}
	if replaceTrue(root, keySwitch) {
		changes = append(changes, keySwitch)
	}
	if replaceString(root, keyMinSystem, minSystem, func(s string) bool {
		return s == "" || s == legacyMin
	}) {
		changes = append(changes, keyMinSystem)
	}
	out, err := marshalXML(root)
	if err != nil {
		return nil, nil, err
	}
	return out, changes, nil
}

func ShortVersion(data []byte) (string, error) {
	root, err := decodeDict(data)
	if err != nil {
		return "", err
	}
	value, ok := root[KeyVersion]
	if !ok {
		return "", nil
	}
	version, ok := value.(string)
	if !ok {
		return "", fmt.Errorf("plistfix: %s is %T; expected a string", KeyVersion, value)
	}
	return version, nil
}

func AsXML(ctx context.Context, data []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("plistfix: %w", err)
	}
	if !bytes.HasPrefix(data, []byte(bplistMagic)) {
		return data, nil
	}
	root, err := unmarshal(data)
	if err != nil {
		return nil, err
	}
	return marshalXML(root)
}

func replaceString(root map[string]any, key, text string, replace func(string) bool) bool {
	value, ok := root[key]
	if ok {
		got, isString := value.(string)
		if isString && !replace(got) {
			return false
		}
	}
	root[key] = text
	return true
}

func replaceTrue(root map[string]any, key string) bool {
	if value, ok := root[key].(bool); ok && value {
		return false
	}
	root[key] = true
	return true
}

func decodeDict(data []byte) (map[string]any, error) {
	root, err := unmarshal(data)
	if err != nil {
		return nil, err
	}
	dict, ok := root.(map[string]any)
	if !ok || dict == nil {
		return nil, errors.New("plistfix: plist has no dict")
	}
	return dict, nil
}

func unmarshal(data []byte) (root any, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			root = nil
			err = panicErr(rec)
		}
	}()
	if _, err = plist.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("plistfix: %w", err)
	}
	return root, nil
}

func marshalXML(value any) (out []byte, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			out = nil
			err = panicErr(rec)
		}
	}()
	out, err = plist.Marshal(value, plist.XMLFormat)
	if err != nil {
		return nil, fmt.Errorf("plistfix: %w", err)
	}
	return out, nil
}

func panicErr(rec any) error {
	err, ok := rec.(error)
	if !ok {
		err = fmt.Errorf("%v", rec)
	}
	return fmt.Errorf("plistfix: %w", err)
}
