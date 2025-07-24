package backoff

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestFixed_Delay(t *testing.T) {
	// Given a fixed backoff strategy with 100ms delay
	fixed := NewFixed(100 * time.Millisecond)

	// When Delay() is called for different attempts
	delay1 := fixed.Delay(1)
	delay2 := fixed.Delay(2)
	delay3 := fixed.Delay(3)

	// Then all delays should be the same
	assert.Equal(t, 100*time.Millisecond, delay1)
	assert.Equal(t, 100*time.Millisecond, delay2)
	assert.Equal(t, 100*time.Millisecond, delay3)
}
