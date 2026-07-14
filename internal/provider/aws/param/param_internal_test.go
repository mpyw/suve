package param

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/stretchr/testify/assert"

	"github.com/mpyw/suve/internal/domain"
)

// mapTypeToDomain / mapDomainToType are pure package-private enum maps. The
// emulator only ever round-trips String/SecureString/StringList, so the default
// (unknown-enum) fallbacks never fire in e2e; the tables below cover every arm.

func TestMapTypeToDomain(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   types.ParameterType
		want domain.ValueType
	}{
		{name: "secure string", in: types.ParameterTypeSecureString, want: domain.ValueTypeSecret},
		{name: "string list", in: types.ParameterTypeStringList, want: domain.ValueTypeList},
		{name: "string", in: types.ParameterTypeString, want: domain.ValueTypePlaintext},
		{name: "unknown falls back to plaintext", in: types.ParameterType("bogus"), want: domain.ValueTypePlaintext},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, mapTypeToDomain(tt.in))
		})
	}
}

func TestMapDomainToType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   domain.ValueType
		want types.ParameterType
	}{
		{name: "secret", in: domain.ValueTypeSecret, want: types.ParameterTypeSecureString},
		{name: "list", in: domain.ValueTypeList, want: types.ParameterTypeStringList},
		{name: "plaintext", in: domain.ValueTypePlaintext, want: types.ParameterTypeString},
		{name: "unknown falls back to string", in: domain.ValueType("bogus"), want: types.ParameterTypeString},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, mapDomainToType(tt.in))
		})
	}
}
