package converter

import (
	"bytes"
	"fmt"
	"io"
	"strings"
)

// Streaming readers must either be interruptible through Close or finite,
// nonblocking memory readers. Wrapping a blocking reader in io.NopCloser does
// not make it interruptible and must not be used here.
func validateStreamReader(reader io.Reader) error {
	switch reader.(type) {
	case io.ReadCloser, *strings.Reader, *bytes.Reader, *bytes.Buffer:
		return nil
	default:
		return fmt.Errorf("stream reader must be a closable upstream body or an in-memory reader")
	}
}
