package pubsub_test

import (
	"gregoryjjb/gomas/pubsub"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPubsub(t *testing.T) {

	ps := pubsub.New[string]()

	_, ch1 := ps.Subscribe()
	id2, ch2 := ps.Subscribe()
	var val1, val2 string

	go func() {
		for v := range ch1 {
			val1 = v
		}
	}()
	go func() {
		for v := range ch2 {
			val2 = v
		}
	}()

	time.Sleep(10 * time.Millisecond)

	ps.Publish("a")
	time.Sleep(10 * time.Millisecond)

	assert.Equal(t, "a", val1)
	assert.Equal(t, "a", val2)

	ps.Unsubscribe(id2)

	ps.Publish("b")
	time.Sleep(10 * time.Millisecond)

	assert.Equal(t, "b", val1)
	assert.Equal(t, "a", val2)
}
