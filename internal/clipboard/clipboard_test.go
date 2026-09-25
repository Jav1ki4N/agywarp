package clipboard

import (
	"testing"
)

func TestReadTextDoesNotPanicOrHang(t *testing.T) {
	// Should complete quickly and return string (empty or current clipboard content)
	text := ReadText()
	t.Logf("ReadText returned: %q", text)
}
