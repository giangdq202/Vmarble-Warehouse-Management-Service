package httpkit

import (
	"context"
	"strings"
	"testing"
)

func TestEstimateRowCount_RejectsUnsafeTableNames(t *testing.T) {
	cases := []string{
		"",                       // empty
		"Users",                  // capital
		"users; DROP TABLE x",    // injection
		"users--",                // sql comment
		"public.users",           // dotted
		"_internal",              // leading underscore (not [a-z])
		"1_users",                // leading digit
		"users with space",       // space
		"\"users\"",              // quoted
	}
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			// Pass nil pool — the validation step must reject before any DB call.
			_, err := EstimateRowCount(context.Background(), nil, name)
			if err == nil {
				t.Errorf("EstimateRowCount(%q) returned nil error; want validation failure", name)
				return
			}
			if !strings.Contains(err.Error(), "invalid table name") {
				t.Errorf("err = %q, want it to mention 'invalid table name'", err)
			}
		})
	}
}
