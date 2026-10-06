package qt

import "testing"

func TestDownloadReportsQuarterMarks(t *testing.T) {
	cases := []struct {
		name  string
		total int64
		steps []int
		want  []int
	}{
		{"even chunks", 100, []int{10, 15, 25, 25, 25}, []int{0, 25, 50, 75, 100}},
		{"one chunk reports the mark reached", 100, []int{100}, []int{100}},
		{"unknown length", -1, []int{50, 50}, []int{0, 0}},
		{"past the end", 100, []int{100, 10}, []int{100, 0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			d := newDownload(tc.total)
			for i, n := range tc.steps {
				if got := d.add(n); got != tc.want[i] {
					t.Fatalf("step %d: add(%d) = %d, want %d", i, n, got, tc.want[i])
				}
			}
		})
	}
}
