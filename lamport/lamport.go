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
	SenderID     int    `json:"senderId"`
	SenderAddr   string `json:"senderAddr"`
	ReceiverAddr string `json:"receiverAddr"`
	Message      string `json:"message"`
	Time         uint   `json:"time"`
}

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
	id         int
	clock      *Clock
	queue      *list.List
	waitCh     chan struct{}
	replies    int
	defered    *list.List
	inCS       bool
	neighbours map[int]string
	log        *logger.Logger
	lock       *sync.Mutex
	listenAddr string
}

func NewNode(id int, listenAddr string, neighbourIDs map[int]string) *Node {
	replyCh := make(chan struct{})
	return &Node{id: id,
		queue: list.New(), waitCh: replyCh, defered: list.New(), clock: &Clock{time: 0},
		neighbours: neighbourIDs,
		log:        &logger.Logger{Prefix: fmt.Sprintf("[%d]", id)},
		lock:       &sync.Mutex{},
		listenAddr: listenAddr,
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
	senderAddr := l.neighbours[m.SenderID]
	switch m.Message {
	case "request":
		l.log.Println("request came from ", m.SenderID)
		l.queue.PushBack(m)
		reply := message{SenderID: l.id, Message: "reply", Time: l.clock.Time(), ReceiverAddr: senderAddr}
		if !l.InCS() {
			l.log.Println("I am not in CS. replying to ", reply.ReceiverAddr)
			b, _ := json.Marshal(reply)
			if err := udpclient.SendMessage(reply.ReceiverAddr, b); err != nil {
				l.log.Println("❗️ ", err)
			}
			l.log.Println("->>", string(b))
		} else {
			l.log.Println("I am in CS. deferring reply to ", reply.ReceiverAddr)
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
		l.log.Println("replying to defered requests. receiver : ", m.ReceiverAddr)
		b, _ := json.Marshal(m)
		go func(recieverAddr string) {
			if err := udpclient.SendMessage(recieverAddr, b); err != nil {
				l.log.Println("❗️ ", err)
			}
			l.log.Println("->>", string(b))
		}(m.ReceiverAddr)
	}
	l.lock.Unlock()

	l.defered.Init()
}

func (l *Node) ExitCS() {
	l.log.Println("exiting  CS")
	l.clock.Tick()
	l.inCS = false
	l.ReplyToDefered()

	for _, addr := range l.neighbours {
		m := message{SenderID: l.id, ReceiverAddr: addr, Message: "release", Time: l.clock.Time()}
		b, _ := json.Marshal(m)
		go func(receiverAddr string) {
			if err := udpclient.SendMessage(receiverAddr, b); err != nil {
				l.log.Println("❗️ ", err)
			}
			l.log.Println("->> ", string(b))
		}(addr)

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

func (l *Node) AskToEnterCS(_ string /* just to satisfy interface */) {
	l.clock.Tick()
	m := message{SenderID: l.id, Message: "request", Time: l.clock.Time()}
	l.queue.PushBack(m)
	for _, addr := range l.neighbours {
		m := message{SenderID: l.id, Message: "request", Time: l.clock.Time(), ReceiverAddr: addr}
		b, _ := json.Marshal(m)
		l.log.Println("->> ", string(b))
		go func(receiverAddr string) {
			if err := udpclient.SendMessage(receiverAddr, b); err != nil {
				l.log.Println("❗️ ", err)
			}
			l.log.Println("->> ", string(b))
		}(addr)

	}
}

func (l *Node) WaitForCS() {
	for {
		<-l.waitCh

		gotPermission := l.replies == len(l.neighbours)
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
	s, err := net.ResolveUDPAddr("udp4", l.listenAddr)
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
		l.log.Println("<<-", string(b))
		go l.ProcessMessage(b)
	}
}
