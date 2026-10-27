package pkgversion

import "testing"

func TestNBBuildOrdering(t *testing.T) {
	versions := []string{"2.1.6", "2.1.6~nb1", "2.1.6~nb2", "2.1.6~nb10", "2.1.7-rc.1~nb1", "2.1.7"}
	for i, a := range versions {
		for j, b := range versions {
			want := 0
			if i < j {
				want = -1
			} else if i > j {
				want = 1
			}
			if got := Compare(a, b); got != want {
				t.Errorf("%s vs %s: %d", a, b, got)
			}
		}
	}
}
