// White-box tests of create.go.
//declscope:namespace create

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

func TestCreateRunner(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		CreateFunc: func(
			_ context.Context, name, value string, vt domain.ValueType, _ string, _ ...provider.WriteOption,
		) (domain.Version, error) {
			assert.Equal(t, "my-secret", name)
			assert.Equal(t, "value", value)
			assert.Equal(t, domain.ValueTypeSecret, vt)

			return domain.Version{ID: "v1"}, nil
		},
	}

	var buf, errBuf bytes.Buffer

	r := &createRunner{
		useCase: &secretusecase.CreateUseCase{Writer: store},
		stdout:  &buf,
		stderr:  &errBuf,
	}
	require.NoError(t, r.run(t.Context(), createOptions{name: "my-secret", value: "value"}))
	assert.Contains(t, buf.String(), "Created secret my-secret")
	assert.Contains(t, buf.String(), "version: v1")
}
