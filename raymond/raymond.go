package raymond

import (
	"distributed-lock-example/logger"
	udpclient "distributed-lock-example/udpclient"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"sync"
)

var MessageRequest string = "request"
var MessagePrivilege string = "privilege"

type message struct {
	SenderID     int    `json:"senderId"`
	ReceiverID   int    `json:"receiverId"`
	ReceiverAddr string `json:"receiverAddr"`
	Message      string `json:"message"`
}

type Node struct {
	id           int
	neighbours   map[int]string
	using        bool
	requestQueue *Queue
	holder       int
	asked        bool
	enterCSCh    chan struct{}
	log          logger.Logger
	mutex        *sync.Mutex
	listenAddr   string
}

func NewNode(ID int, listenAddr string, neighbourIDs map[int]string, holder int) *Node {
	return &Node{id: ID,
		neighbours: neighbourIDs, requestQueue: NewQueue(), holder: holder, enterCSCh: make(chan struct{}, 1),
		log:        logger.Logger{Prefix: fmt.Sprintf("[%d]", ID)},
		mutex:      &sync.Mutex{},
		listenAddr: listenAddr,
	}
}

func (r *Node) ID() int {
	return r.id
}

func (r *Node) makeRequest() {
	var holderID int
	r.mutex.Lock()
	holderID = r.holder
	r.mutex.Unlock()
	if holderID != r.id && !r.asked && r.requestQueue.Len() > 0 {
		m := message{SenderID: r.id, Message: MessageRequest, ReceiverID: holderID, ReceiverAddr: r.neighbours[holderID]}
		b, _ := json.Marshal(m)
		r.log.Println("->> ", string(b))
		if err := udpclient.SendMessage(m.ReceiverAddr, b); err != nil {
			r.log.Fatalln("❗️", err)
		}
		r.asked = true
	} else {
		r.log.Printf("makeRequest ignored. hoder: %d; asked: %v queue size: %d\n", holderID, r.asked, r.requestQueue.Len())
	}
}

func (r *Node) assignPrivilege() {
	var holderID int
	r.mutex.Lock()
	holderID = r.holder
	r.mutex.Unlock()
	if holderID == r.id && !r.using && r.requestQueue.Len() > 0 {

		nextHolder := r.requestQueue.Dequeue().(int)

		// e := r.requestQueue.Front()
		// nextHolder := e.Value.(int)
		// r.requestQueue.Remove(e)

		if nextHolder == r.id {
			r.log.Println("using token for myself")
			r.enterCSCh <- struct{}{}
			r.using = true
		} else {
			r.log.Println("giving privilege to ", nextHolder)
			m := message{SenderID: r.id, Message: MessagePrivilege, ReceiverAddr: r.neighbours[nextHolder]}
			b, _ := json.Marshal(m)
			r.log.Println("->> ", string(b))
			if err := udpclient.SendMessage(m.ReceiverAddr, b); err != nil {
				r.log.Fatalln("❗️", err)
			}
			r.mutex.Lock()
			r.asked = false
			r.holder = nextHolder
			r.mutex.Unlock()
			r.log.Println("new holder is ", r.holder)
			// 3.4 section of raymond publication paper states:
			// "If the privilege is passed to another node, MAKE_REQUEST may request that the privilege be returned."
			r.makeRequest()
		}
	}

}

func (r *Node) AskToEnterCS(_ string /* just to statisfy interface */) {
	// r.requestQueue.PushBack(r.nodeID)
	r.requestQueue.Enqueue(r.id)
	if r.holder == r.id {
		r.assignPrivilege()
	} else {
		r.log.Println("asking holder for token...")
		r.makeRequest()
	}
}

func (r *Node) WaitForCS() {
	<-r.enterCSCh
}

func (r *Node) InCS() bool {
	return r.using
}

func (r *Node) EnterCS() {
	r.using = true
}
func (r *Node) ExitCS() {
	r.using = false
	r.assignPrivilege()
}
func (r *Node) ProcessMessage(b []byte) {
	var m message
	if err := json.Unmarshal(b, &m); err != nil {
		r.log.Println("⚠️", err)
		return
	}
	switch m.Message {
	case MessageRequest:
		// r.requestQueue.PushBack(m.SenderID)
		r.requestQueue.Enqueue(m.SenderID)
		if r.holder == r.id {
			r.assignPrivilege()
		} else {
			r.makeRequest()
		}
	case MessagePrivilege:
		r.holder = r.id
		r.assignPrivilege()
	}
}

func (r *Node) Start() {

	s, err := net.ResolveUDPAddr("udp4", r.listenAddr)
	if err != nil {
		log.Fatalln(err)
	}

	conn, err := net.ListenUDP("udp4", s)
	if err != nil {
		log.Fatalln(err)
	}
	defer conn.Close()

	for {
		buffer := make([]byte, 1024)
		n, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			log.Println("failed to read from udp: ", err)
			break
		}
		b := buffer[0 : n-1]
		log.Println(fmt.Sprintf("[%d]", r.ID()), "<<- ", string(b))
		go r.ProcessMessage(b)
	}
}
