package ratelimit

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestAllowBurstThenRefill(t *testing.T) {
	l := New(1, 3) // 3 burst, 1 token/second
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		assert.True(t, l.Allow("ip"), "request %d should pass within burst", i+1)
	}
	assert.False(t, l.Allow("ip"), "4th request exceeds burst")

	now = now.Add(time.Second)
	assert.True(t, l.Allow("ip"), "one token refilled after 1s")
	assert.False(t, l.Allow("ip"))

	now = now.Add(time.Hour)
	for i := 0; i < 3; i++ {
		assert.True(t, l.Allow("ip"), "bucket refills to burst, not beyond")
	}
	assert.False(t, l.Allow("ip"))
}

func TestKeysAreIndependent(t *testing.T) {
	l := New(1, 1)
	assert.True(t, l.Allow("a"))
	assert.False(t, l.Allow("a"))
	assert.True(t, l.Allow("b"))
}

func TestPruneForgetsIdleBuckets(t *testing.T) {
	l := New(100, 1)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	l.now = func() time.Time { return now }

	for i := 0; i < 1100; i++ {
		l.Allow(fmt.Sprintf("ip-%d", i))
	}
	assert.GreaterOrEqual(t, len(l.buckets), 1100)

	now = now.Add(l.ttl + time.Second)
	l.Allow("fresh")
	assert.Less(t, len(l.buckets), 10, "idle buckets pruned once the map is large")
}

func TestAllowIsSafeForConcurrentUse(t *testing.T) {
	// Refill is negligible within the test, so each of the 3 keys hands out
	// exactly its burst regardless of how the goroutines interleave.
	l := New(0.001, 50)
	var allowed atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if l.Allow(fmt.Sprintf("ip-%d", i%3)) {
					allowed.Add(1)
				}
			}
		}(i)
	}
	wg.Wait()
	assert.EqualValues(t, 3*50, allowed.Load())
}

func TestNewClampsBadArguments(t *testing.T) {
	l := New(-1, 0)
	assert.True(t, l.Allow("x"))
	assert.False(t, l.Allow("x"))
}
