package revisioning

import (
	"crypto/sha1"
	"errors"
	"fmt"
	"time"

	"github.com/mpyw/suve/pkg/core/versioning"
)

//go:generate stringer -type=RevisionContentType -output=revisioning_string.gen.go
type RevisionContentType int

var _ fmt.Stringer = RevisionContentType(0)

const (
	RevisionContentTypeUnknown RevisionContentType = iota
	// RevisionContentTypeString is used for general purposes.
	RevisionContentTypeString
	// RevisionContentTypeBytes is used for binary records in Secrets Manager.
	RevisionContentTypeBytes
)

var ErrInvalidRevisionContent = errors.New("invalid revision content")

type Revision struct {
	Version versioning.Version
	Content *RevisionContent
	Date    time.Time
}

type RevisionContent struct {
	Type RevisionContentType
	// StringValue is valid when Type == RevisionContentTypeString.
	StringValue string
	// BytesValue is valid when Type == RevisionContentTypeBytes.
	BytesValue []byte
	// EncryptionEnabled is true for:
	//   - Secrets Manager records
	//   - Parameter Store "SecureString" records
	EncryptionEnabled bool
}

func (content *RevisionContent) String() string {
	switch content.Type {
	case RevisionContentTypeBytes:
		return fmt.Sprintf("<Binary Data: SHA-1: %x>", sha1.Sum(content.BytesValue))
	case RevisionContentTypeString:
		return content.StringValue
	default:
		return "<Unknown Data>"
	}
}
