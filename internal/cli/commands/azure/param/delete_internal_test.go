// White-box tests of delete.go.
//declscope:namespace delete

package param

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	paramusecase "github.com/mpyw/suve/internal/usecase/param"
)

func TestDeleteRunner(t *testing.T) {
	t.Parallel()

	var deleted string

	store := &providermock.Store{
		DeleteFunc: func(_ context.Context, name string, _ ...provider.DeleteOption) error {
			deleted = name

			return nil
		},
	}

	var buf, errBuf bytes.Buffer

	r := &deleteRunner{
		useCase: &paramusecase.DeleteUseCase{Store: store},
		stdout:  &buf,
		stderr:  &errBuf,
	}
	require.NoError(t, r.run(t.Context(), deleteOptions{name: "app/timeout"}))
	assert.Equal(t, "app/timeout", deleted)
	assert.Contains(t, buf.String(), "Deleted setting app/timeout")
}
