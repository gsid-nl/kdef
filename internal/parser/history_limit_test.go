package parser

import (
	"strings"
	"testing"
)

func TestParseCronJobHistoryLimits(t *testing.T) {
	src := `
cronjob "sweep" {
  namespace            = "jobs"
  image                = "busybox"
  schedule             = "* * * * *"
  history_limit        = 2
  failed_history_limit = 1
}
`
	result, diags := ParseBytes([]byte(src), "cj.kdef", nil)
	if diags.HasErrors() {
		t.Fatalf("parse errors: %v", diags)
	}
	cj := result.CronJobs[0]
	if cj.HistoryLimit == nil || *cj.HistoryLimit != 2 {
		t.Errorf("HistoryLimit: got %v, want 2", cj.HistoryLimit)
	}
	if cj.FailedHistoryLimit == nil || *cj.FailedHistoryLimit != 1 {
		t.Errorf("FailedHistoryLimit: got %v, want 1", cj.FailedHistoryLimit)
	}
}

func TestParseCronJobHistoryLimitInvalid(t *testing.T) {
	for _, val := range []string{"-1", "1.5", `"2"`} {
		t.Run(val, func(t *testing.T) {
			src := `
cronjob "sweep" {
  namespace     = "jobs"
  image         = "busybox"
  schedule      = "* * * * *"
  history_limit = ` + val + `
}
`
			_, diags := ParseBytes([]byte(src), "cj.kdef", nil)
			if !diags.HasErrors() || !strings.Contains(diags.Error(), "history_limit") {
				t.Fatalf("expected a history_limit error, got %v", diags)
			}
		})
	}
}

func rolloutSrc(attrs string) string {
	return `
deployment "web" {
  namespace = "prod"

  container "web" {
    image = "nginx"
  }

  rollout {
` + attrs + `
  }
}
`
}

func TestParseRolloutHistoryLimit(t *testing.T) {
	for _, attr := range []string{"history_limit = 2", "revision_history_limit = 2"} {
		t.Run(attr, func(t *testing.T) {
			result, diags := ParseBytes([]byte(rolloutSrc(attr)), "d.kdef", nil)
			if diags.HasErrors() {
				t.Fatalf("parse errors: %v", diags)
			}
			r := result.Deployments[0].Rollout
			if r == nil || r.RevisionHistoryLimit == nil || *r.RevisionHistoryLimit != 2 {
				t.Fatalf("RevisionHistoryLimit: want 2, got %+v", r)
			}
		})
	}
}

func TestParseRolloutHistoryLimitBothNamesRejected(t *testing.T) {
	src := rolloutSrc("history_limit = 2\nrevision_history_limit = 3")
	_, diags := ParseBytes([]byte(src), "d.kdef", nil)
	if !diags.HasErrors() || !strings.Contains(diags.Error(), "Duplicate history limit") {
		t.Fatalf("expected duplicate history limit error, got %v", diags)
	}
}
