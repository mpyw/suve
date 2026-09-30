// White-box tests of log.go.
//declscope:namespace log

package param

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/cli/commands/generic"
	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider/azure/appconfig"
	"github.com/mpyw/suve/internal/provider/providermock"
)

// TestLogPresenter_AcidTest is the acid test: App Configuration has no version
// history, so the log presenter's Fetch surfaces a clean error and never
// crashes.
func TestLogPresenter_AcidTest(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		HistoryFunc: func(_ context.Context, _ string) ([]domain.Version, error) {
			// Mirror the appconfig adapter's degradation.
			return nil, appconfig.ErrVersioningUnsupported
		},
	}

	presenter := newLogPresenter(store, generic.LogRequest{Name: "my-key"})
	err := presenter.Fetch(t.Context())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not support versions")
	// Render methods are safe to call even after a failed fetch (no panic).
	assert.Equal(t, 0, presenter.Len())
}

// TestLogPresenter_RenderStubs covers the App Configuration log presenter's
// render stubs. App Configuration has no version history, so these methods only
// exist to satisfy the generic.Presenter interface and must be safe no-ops.
func TestLogPresenter_RenderStubs(t *testing.T) {
	t.Parallel()

	presenter := newLogPresenter(&providermock.Store{}, generic.LogRequest{Name: "my-key"})

	var buf, errBuf bytes.Buffer

	assert.Equal(t, 0, presenter.Len())
	require.NoError(t, presenter.RenderJSON(&buf))
	presenter.RenderOneline(&buf, 0, 0)
	presenter.RenderHeader(&buf, 0)
	presenter.RenderValue(&buf, 0, 0)
	presenter.RenderPatch(&buf, &errBuf, 0, false, false)

	assert.Empty(t, buf.String())
	assert.Empty(t, errBuf.String())
}
