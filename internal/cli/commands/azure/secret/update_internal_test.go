// White-box tests of update.go.
//declscope:namespace update

package secret

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	secretusecase "github.com/mpyw/suve/internal/usecase/secret"
)

func TestUpdateRunner_NotFound(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return nil, provider.ErrNotFound
		},
	}

	var buf, errBuf bytes.Buffer

	r := &updateRunner{
		useCase: &secretusecase.UpdateUseCase{Store: store},
		stdout:  &buf,
		stderr:  &errBuf,
	}
	err := r.run(t.Context(), updateOptions{name: "missing", value: "new"})
	require.ErrorIs(t, err, secretusecase.ErrSecretNotFound)
}
