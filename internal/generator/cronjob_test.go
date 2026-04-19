package generator

import (
	"testing"

	"github.com/gsid-nl/kdef/internal/types"
)

func TestGenerateCronJob_Suspend(t *testing.T) {
	tru, fls := true, false

	cases := []struct {
		name    string
		suspend *bool
		want    *bool // nil means field should be omitted
	}{
		{"omitted", nil, nil},
		{"false", &fls, &fls},
		{"true", &tru, &tru},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cj := GenerateCronJob(types.CronJobConfig{
				Name:     "x",
				Image:    "busybox",
				Schedule: "* * * * *",
				Suspend:  tc.suspend,
			})
			got := cj.Spec.Suspend
			switch {
			case tc.want == nil && got != nil:
				t.Fatalf("expected Suspend to be nil, got %v", *got)
			case tc.want != nil && got == nil:
				t.Fatalf("expected Suspend %v, got nil", *tc.want)
			case tc.want != nil && got != nil && *got != *tc.want:
				t.Fatalf("Suspend: got %v, want %v", *got, *tc.want)
			}
		})
	}
}
