// White-box tests of create.go.
//declscope:namespace create

package param

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mpyw/suve/internal/domain"
	"github.com/mpyw/suve/internal/provider"
	"github.com/mpyw/suve/internal/provider/providermock"
	paramusecase "github.com/mpyw/suve/internal/usecase/param"
)

func TestCreateRunner(t *testing.T) {
	t.Parallel()

	store := &providermock.Store{
		CreateFunc: func(
			_ context.Context, name, value string, vt domain.ValueType, _ string, _ ...provider.WriteOption,
		) (domain.Version, error) {
			assert.Equal(t, "app/timeout", name)
			assert.Equal(t, "30", value)
			assert.Equal(t, domain.ValueTypePlaintext, vt)

			return domain.Version{}, nil
		},
	}

	var buf, errBuf bytes.Buffer

	r := &createRunner{
		useCase: &paramusecase.CreateUseCase{Writer: store},
		stdout:  &buf,
		stderr:  &errBuf,
	}
	require.NoError(t, r.run(t.Context(), createOptions{name: "app/timeout", value: "30"}))
	assert.Contains(t, buf.String(), "Created setting app/timeout")
}
