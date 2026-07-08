package main

import "testing"

func TestIsShellCompletion(t *testing.T) {
	t.Parallel()

	tests := map[string]struct {
		args []string
		want bool
	}{
		"present": {
			args: []string{"suve", "param", "show", "--generate-shell-completion"},
			want: true,
		},
		"present as only arg after program": {
			args: []string{"suve", "--generate-shell-completion"},
			want: true,
		},
		"absent": {
			args: []string{"suve", "param", "show", "/my/param"},
			want: false,
		},
		"empty": {
			args: nil,
			want: false,
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			if got := isShellCompletion(tt.args); got != tt.want {
				t.Errorf("isShellCompletion(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}
