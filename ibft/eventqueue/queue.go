package eventqueue

import "sync"

// Event represent some function
type Event func()

// EventQueue is the interface for managing a queue of functions
type EventQueue interface {
	Add(Event) bool
	Pop() Event
	ClearAndStop()
}

// Queue thread safe implementation of EventQueue
type Queue struct {
	stop  bool
	queue []Event
	lock  sync.Mutex
}

// New returns a new instance of Queue
func New() EventQueue {
	q := Queue{
		queue: make([]Event, 0),
		lock:  sync.Mutex{},
	}
	return &q
}

// Add will add an an item to the queue, thread safe.
func (q *Queue) Add(e Event) bool {
	q.lock.Lock()
	defer q.lock.Unlock()

	if q.stop {
		return false
	}

	q.queue = append(q.queue, e)
	return true
}

// Pop will return and delete an an item from the queue, thread safe.
func (q *Queue) Pop() Event {
	q.lock.Lock()
	defer q.lock.Unlock()

	if q.stop {
		return nil
	}

	if len(q.queue) > 0 {
		ret := q.queue[0]
		q.queue = q.queue[1:len(q.queue)]
		return ret
	}
	return nil
}

// ClearAndStop will clear the queue disable adding more items to it, thread safe.
func (q *Queue) ClearAndStop() {
	q.lock.Lock()
	defer q.lock.Unlock()

	q.stop = true
	q.queue = make([]Event, 0)
}
