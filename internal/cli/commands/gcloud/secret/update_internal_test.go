// White-box tests of update.go.
//declscope:namespace update

package secret

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	secretusecase "github.com/mpyw/suve/internal/usecase/secret"
)

func TestUpdateRunner(t *testing.T) {
	t.Parallel()

	var gotDescription string

	store := &providermock.Store{
		GetFunc: func(_ context.Context, _ string, _ provider.VersionRef) (*domain.Entry, error) {
			return &domain.Entry{Name: "my-secret", Value: "old"}, nil
		},
		PutFunc: func(
			_ context.Context, _, value string, _ domain.ValueType, description string, _ ...provider.WriteOption,
		) (domain.Version, error) {
			assert.Equal(t, "new", value)

			gotDescription = description

			return domain.Version{ID: "2"}, nil
		},
	}

	var buf, errBuf bytes.Buffer

	r := &updateRunner{
		useCase: &secretusecase.UpdateUseCase{Store: store},
		stdout:  &buf,
		stderr:  &errBuf,
	}
	require.NoError(t, r.run(t.Context(), updateOptions{name: "my-secret", value: "new", description: "rotated key"}))
	assert.Contains(t, buf.String(), "Updated secret my-secret")
	assert.Contains(t, buf.String(), "version: 2")
	assert.Equal(t, "rotated key", gotDescription, "the --description value reaches the writer")
}

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
