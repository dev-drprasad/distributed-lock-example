package raymond

import (
	"container/list"
	"sync"
)

type Queue struct {
	lock *sync.Mutex
	q    *list.List
}

func NewQueue() *Queue {
	return &Queue{q: list.New(), lock: &sync.Mutex{}}
}

func (q *Queue) Enqueue(v interface{}) {
	q.lock.Lock()
	q.q.PushBack(v)
	q.lock.Unlock()
}

func (q *Queue) Len() int {
	q.lock.Lock()
	length := q.q.Len()
	q.lock.Unlock()
	return length
}

func (q *Queue) Dequeue() interface{} {
	q.lock.Lock()
	e := q.q.Front()
	v := e.Value
	q.q.Remove(e)
	q.lock.Unlock()
	return v
}
