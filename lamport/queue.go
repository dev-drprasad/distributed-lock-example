package lamport

import "container/list"

type Queue struct {
	q *list.List
}

func New() *Queue {
	return &Queue{q: list.New()}
}

func Enqueue() {

}
