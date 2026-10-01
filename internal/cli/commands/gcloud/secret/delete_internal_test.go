// White-box tests of delete.go.
//declscope:namespace delete

package secret

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	secretusecase "github.com/mpyw/suve/internal/usecase/secret"
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
		useCase: &secretusecase.DeleteUseCase{Store: store},
		stdout:  &buf,
		stderr:  &errBuf,
	}
	require.NoError(t, r.run(t.Context(), deleteOptions{name: "my-secret"}))
	assert.Equal(t, "my-secret", deleted)
	assert.Contains(t, buf.String(), "Permanently deleted secret my-secret")
}
