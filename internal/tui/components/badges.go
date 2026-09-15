package components

import (
	"strings"

	"github.com/mpyw/suve/internal/tui/styles"
)

// renderBadges renders trailing chips as "[a] [b]".
//
//declscope:package // shared by the list (list.go) and history (history.go) rows
func renderBadges(st styles.Styles, badges []string) string {
	if len(badges) == 0 {
		return ""
	}

	parts := make([]string, len(badges))
	for i, b := range badges {
		parts[i] = st.PageHint.Render("[" + b + "]")
	}

	return strings.Join(parts, " ")
}
