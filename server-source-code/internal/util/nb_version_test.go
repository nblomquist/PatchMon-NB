package util

import (
	"context"
	"testing"
)

func TestNBBuildVersions(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"2.1.6~nb1", "2.1.6", 1},
		{"2.1.6~nb2", "2.1.6~nb1", 1},
		{"2.1.6~nb10", "2.1.6~nb2", 1},
		{"2.1.6~nb1", "2.1.7", -1},
		{"2.1.7-rc.1~nb1", "2.1.7", -1},
		{"2.1.6~nb1", "2.1.6~nb1", 0},
	} {
		if got := CompareVersions(tc.a, tc.b); got != tc.want {
			t.Errorf("%s vs %s: %d", tc.a, tc.b, got)
		}
		if got := CompareVersions(tc.b, tc.a); got != -tc.want {
			t.Errorf("reverse: %d", got)
		}
	}
	if !ValidConfigVersion("2.1.6~nb1") {
		t.Fatal("NB config version rejected")
	}
	for _, version := range []string{"2.1.6~nb1", "2.1.7-rc.3~nb12"} {
		m := agentVersionRe.FindStringSubmatch("patchmon-agent version " + version)
		if len(m) < 2 || m[1] != version {
			t.Fatalf("truncated version: %v", m)
		}
		path := buildFixtureAgent(t, "linux", foreignArch(), version)
		info, err := GetAgentBinaryInfo(context.Background(), path)
		if err != nil || info.Version != version {
			t.Fatalf("binary metadata: %+v %v", info, err)
		}
	}
}
