package pubsub

import (
	"sync"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var plog zerolog.Logger

func init() {
	plog = log.With().Str("component", "pubsub").Logger()
}

type SubscriptionID int64

type Pubsub[T any] struct {
	nextID      SubscriptionID
	subscribers map[SubscriptionID]chan<- T
	mu          sync.RWMutex
}

func New[T any]() *Pubsub[T] {
	return &Pubsub[T]{
		subscribers: make(map[SubscriptionID]chan<- T),
	}
}

func (this *Pubsub[T]) Subscribe() (SubscriptionID, <-chan T) {
	this.mu.Lock()
	defer this.mu.Unlock()

	ch := make(chan T)
	id := this.nextID
	// fmt.Println("OPENING A PUBSUB ", id)

	this.subscribers[id] = ch
	this.nextID += 1

	return id, ch
}

func (this *Pubsub[T]) Unsubscribe(id SubscriptionID) {
	this.mu.Lock()
	defer this.mu.Unlock()

	// fmt.Println("CLOSING A PUBSUB ", id)

	ch, ok := this.subscribers[id]
	if !ok {
		return
	}

	delete(this.subscribers, id)
	close(ch)
}

func (this *Pubsub[T]) Publish(msg T) {
	this.mu.RLock()
	defer this.mu.RUnlock()

	// fmt.Println("PUBLISHING A MESSAGE ", msg)
	// fmt.Println("PUBSUB SIZE ", len(this.subscribers))

	var wg sync.WaitGroup
	for id, ch := range this.subscribers {
		wg.Add(1)

		go func(id SubscriptionID, ch chan<- T) {
			defer wg.Done()
			// fmt.Println("SENDING MESSAGE TO ", id)
			select {
			case ch <- msg:
			default:
				plog.Warn().
					Int64("subscription_id", int64(id)).
					Interface("message", msg).
					Msg("Message dropped, channel full")
			}
		}(id, ch)
	}

	wg.Wait()
}
