package pubsub_test

import (
	"sync"
	"testing"

	"github.com/vexner67/freenet/pkg/pubsub"
)

func TestHub_Delivers(t *testing.T) {
	t.Parallel()

	hub := pubsub.New[string, int](4)

	ann1, cancel1 := hub.Subscribe("ann")
	defer cancel1()
	ann2, cancel2 := hub.Subscribe("ann")
	defer cancel2()
	bob, cancelBob := hub.Subscribe("bob")
	defer cancelBob()

	hub.Publish("ann", 1)
	hub.Publish("ann", 2)
	hub.Publish("cat", 3) // nobody listens

	for _, ch := range []<-chan int{ann1, ann2} {
		if a, b := <-ch, <-ch; a != 1 || b != 2 {
			t.Fatalf("got %d, %d; want 1, 2 in order", a, b)
		}
	}

	select {
	case e := <-bob:
		t.Fatalf("bob got ann's event %d", e)
	default:
	}
}

func TestHub_DropsWhoFallsBehind(t *testing.T) {
	t.Parallel()

	hub := pubsub.New[string, int](2)
	slow, cancel := hub.Subscribe("ann")
	defer cancel()

	for i := range 3 { // one more than the buffer
		hub.Publish("ann", i)
	}

	got := 0
	for range slow {
		got++
	}

	if got != 2 {
		t.Fatalf("got %d events before the channel closed, want the 2 buffered", got)
	}

	hub.Publish("ann", 4) // must not panic on the dropped subscriber
}

func TestHub_CancelAndClose(t *testing.T) {
	t.Parallel()

	hub := pubsub.New[string, int](1)

	ch, cancel := hub.Subscribe("ann")
	cancel()
	cancel() // twice is fine

	if _, open := <-ch; open {
		t.Fatal("cancel must close the channel")
	}

	other, otherCancel := hub.Subscribe("bob")
	defer otherCancel()

	hub.Close()

	if _, open := <-other; open {
		t.Fatal("Close must close every channel")
	}
}

func TestHub_Concurrent(t *testing.T) {
	t.Parallel()

	hub := pubsub.New[int, int](8)

	var wg sync.WaitGroup
	for key := range 8 {
		wg.Go(func() {
			ch, cancel := hub.Subscribe(key % 2)
			defer cancel()
			for i := range 100 {
				hub.Publish(key%2, i)
				select {
				case <-ch:
				default:
				}
			}
		})
	}
	wg.Wait()
}
