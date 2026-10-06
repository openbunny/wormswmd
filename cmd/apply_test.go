package cmd

import (
	"errors"
	"testing"
)

func TestApplyCache(t *testing.T) {
	cacheErr := errors.New("cache unavailable")
	cases := []struct {
		name    string
		qt      string
		env     string
		prefix  string
		cache   string
		err     error
		want    string
		wantErr bool
	}{
		{name: "cache", cache: "/cache", want: "/cache"},
		{name: "empty cache", want: ""},
		{name: "cache error", err: cacheErr, wantErr: true},
		{name: "qt allows empty", qt: "/qt.tar", err: cacheErr},
		{name: "env allows empty", env: "/from-env.tar", err: cacheErr},
		{name: "prefix allows empty", prefix: "/qt", err: cacheErr},
		{name: "qt keeps cache", qt: "/qt.tar", cache: "/cache", want: "/cache"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := applyCache(tc.qt, tc.env, tc.prefix, tc.cache, tc.err)
			if tc.wantErr {
				if !errors.Is(err, cacheErr) {
					t.Fatalf("applyCache = %q, %v", got, err)
				}
				return
			}
			if err != nil || got != tc.want {
				t.Fatalf("applyCache = %q, %v", got, err)
			}
		})
	}
}
