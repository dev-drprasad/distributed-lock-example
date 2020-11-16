package lamport

import (
	"container/list"
	"distributed-lock-example/logger"
	udpclient "distributed-lock-example/udpclient"
	"encoding/json"
	"fmt"
	"net"
	"sync"
)

type message struct {
	SenderID   int    `json:"senderId"`
	ReceiverID int    `json:"receiverId"`
	Message    string `json:"message"`
	Time       uint   `json:"time"`
}

// var ids = []int{0, 1, 2, 3}

type Clock struct {
	time uint
}

func (c *Clock) Time() uint {
	return c.time
}

func (c *Clock) Tick() {
	c.time++
}

func (c *Clock) TakeMax(recvTime uint) {
	if recvTime > c.time {
		c.time = recvTime + 1
	} else {
		c.time++
	}
}

type Node struct {
	id           int
	clock        *Clock
	queue        *list.List
	waitCh       chan struct{}
	replies      int
	defered      *list.List
	inCS         bool
	neighbourIDs []int
	log          *logger.Logger
	lock         *sync.Mutex
}

func NewNode(id int, neighbourIDs []int) *Node {
	replyCh := make(chan struct{})
	return &Node{id: id,
		queue: list.New(), waitCh: replyCh, defered: list.New(), clock: &Clock{time: 0},
		neighbourIDs: neighbourIDs,
		log:          &logger.Logger{Prefix: fmt.Sprintf("[%d]", id)},
		lock:         &sync.Mutex{},
	}
}

func (l *Node) ID() int {
	return l.id
}

func (l *Node) ProcessMessage(b []byte) {
	var m message
	if err := json.Unmarshal(b, &m); err != nil {
		l.log.Println("error unmarshalling message", err)
	}
	l.clock.TakeMax(m.Time)
	switch m.Message {
	case "request":
		l.log.Println("request came from ", m.SenderID)
		l.queue.PushBack(m)
		reply := message{SenderID: l.id, Message: "reply", Time: l.clock.Time(), ReceiverID: m.SenderID}
		if !l.InCS() {
			l.log.Println("I am not in CS. replying to ", reply.ReceiverID)
			b, _ := json.Marshal(reply)
			if err := udpclient.SendMessage(fmt.Sprintf(":%d", 7000+m.SenderID), b); err != nil {
				l.log.Println("❗️ ", err)
			}
		} else {
			l.log.Println("I am in CS. deferring reply to ", reply.ReceiverID)
			l.defered.PushBack(reply)
		}
	case "reply":
		l.log.Println("got permission to enter from ", m.SenderID)
		l.replies++
		l.waitCh <- struct{}{}
	case "release":

		l.lock.Lock()
		for e := l.queue.Front(); e != nil; e = e.Next() {
			rm := e.Value.(message)
			if rm.SenderID == m.SenderID {
				l.queue.Remove(e)
				break
			}
		}
		l.lock.Unlock()
		l.waitCh <- struct{}{}
	}
}

func (l *Node) InCS() bool {
	return l.inCS
}

func (l *Node) EnterCS() {
	l.log.Println("entering to CS")
	l.clock.Tick()
	l.inCS = true
}

func (l *Node) ReplyToDefered() {
	l.lock.Lock()
	for e := l.defered.Front(); e != nil; e = e.Next() {
		m := e.Value.(message)
		l.log.Println("replying to defered requests. receiver : ", m.ReceiverID)
		b, _ := json.Marshal(m)
		l.log.Println("->> ", string(b))
		go func(id int) {
			if err := udpclient.SendMessage(fmt.Sprintf(":%d", 7000+id), b); err != nil {
				l.log.Println("❗️ ", err)
			}
		}(m.ReceiverID)
	}
	l.lock.Unlock()

	l.defered.Init()
}

func (l *Node) ExitCS() {
	l.log.Println("exiting  CS")
	l.clock.Tick()
	l.inCS = false
	l.ReplyToDefered()

	for _, id := range l.neighbourIDs {
		m := message{SenderID: l.id, ReceiverID: id, Message: "release", Time: l.clock.Time()}
		b, _ := json.Marshal(m)
		l.log.Println("->> ", string(b))
		go func(id int) {
			if err := udpclient.SendMessage(fmt.Sprintf(":%d", 7000+id), b); err != nil {
				l.log.Println("❗️ ", err)
			}
		}(id)

	}

	l.lock.Lock()
	for e := l.queue.Front(); e != nil; e = e.Next() {
		m := e.Value.(message)
		if m.SenderID == l.id {
			l.queue.Remove(e)
			break
		}
	}
	l.lock.Unlock()
}

func (l *Node) AskToEnterCS() {
	l.clock.Tick()
	m := message{SenderID: l.id, Message: "request", Time: l.clock.Time()}
	l.queue.PushBack(m)
	for _, id := range l.neighbourIDs {
		m := message{SenderID: l.id, Message: "request", Time: l.clock.Time(), ReceiverID: id}
		b, _ := json.Marshal(m)
		l.log.Println("->> ", string(b))
		go func(id int) {
			if err := udpclient.SendMessage(fmt.Sprintf(":%d", 7000+id), b); err != nil {
				l.log.Println("❗️ ", err)
			}
		}(id)

	}
}

func (l *Node) WaitForCS() {
	for {
		<-l.waitCh

		gotPermission := l.replies == len(l.neighbourIDs)
		if gotPermission {
			m := l.queue.Front().Value.(message)
			if m.SenderID == l.id {
				break
			} else {
				l.log.Println("I got permission. but priority goes to ", m.SenderID)
			}
		}
	}
	l.replies = 0
}

func (l *Node) Start() {
	listenAddr := fmt.Sprintf(":%d", 7000+l.id)

	s, err := net.ResolveUDPAddr("udp4", listenAddr)
	if err != nil {
		l.log.Fatalln(err)
	}

	conn, err := net.ListenUDP("udp4", s)
	if err != nil {
		l.log.Fatalln(err)
	}
	defer conn.Close()

	for {
		buffer := make([]byte, 1024)
		n, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			l.log.Println("failed to read from udp: ", err)
		}
		b := buffer[0 : n-1]
		l.log.Println("<<- ", string(b))
		go l.ProcessMessage(b)
	}
}
