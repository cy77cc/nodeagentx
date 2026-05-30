package federation

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRegisterMetrics_DoesNotPanic(t *testing.T) {
	assert.NotPanics(t, func() {
		RegisterMetrics()
	})
}
