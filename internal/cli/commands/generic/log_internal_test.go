// White-box tests of log.go.
//declscope:namespace log

package generic

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
)

// recordingPresenter captures the maxValueLength the LogRunner passes to the render
// methods, so a flag-parsing test can assert the Int32 --max-value-length flag
// actually reaches LogOptions (it was silently read via cmd.Int, always 0) (#345).
type logRecordingPresenter struct {
	gotMaxValueLength int
}

func (p *logRecordingPresenter) Fetch(context.Context) error { return nil }

func (p *logRecordingPresenter) Len() int { return 1 }

func (p *logRecordingPresenter) RenderJSON(io.Writer) error { return nil }

func (p *logRecordingPresenter) RenderOneline(_ io.Writer, _, maxValueLength int) {
	p.gotMaxValueLength = maxValueLength
}

func (p *logRecordingPresenter) RenderHeader(io.Writer, int) {}

func (p *logRecordingPresenter) RenderValue(_ io.Writer, _, maxValueLength int) {
	p.gotMaxValueLength = maxValueLength
}

func (p *logRecordingPresenter) RenderPatch(io.Writer, io.Writer, int, bool, bool) {}

func TestCommand_MaxValueLengthFlagReachesOptions(t *testing.T) {
	t.Parallel()

	rec := &logRecordingPresenter{}

	logCmd := LogCommand(LogConfig{
		Usage:      "u",
		ArgsUsage:  "<name>",
		UsageError: "usage",
		Flags: []cli.Flag{
			&cli.Int32Flag{Name: "max-value-length"},
			&cli.BoolFlag{Name: "no-pager"},
		},
		NewPresenter: func(_ context.Context, _ LogRequest) (LogPresenter, error) {
			return rec, nil
		},
	})

	var buf, errBuf bytes.Buffer

	app := &cli.Command{
		Name:      "suve",
		Commands:  []*cli.Command{logCmd},
		Writer:    &buf,
		ErrWriter: &errBuf,
	}

	err := app.Run(t.Context(), []string{"suve", "log", "--no-pager", "--max-value-length", "20", "myname"})
	require.NoError(t, err)
	assert.Equal(t, 20, rec.gotMaxValueLength, "Int32 --max-value-length must reach Options.MaxValueLength")
}
