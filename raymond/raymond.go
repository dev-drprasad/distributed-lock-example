package raymond

import (
	"distributed-lock-example/logger"
	udpclient "distributed-lock-example/udpclient"
	"encoding/json"
	"fmt"
	"sync"
)

var MessageRequest string = "request"
var MessagePrivilege string = "privilege"

type message struct {
	SenderID   int    `json:"senderId"`
	ReceiverID int    `json:"receiverId"`
	Message    string `json:"message"`
}

type Node struct {
	nodeID       int
	neighbourIDs []int
	using        bool
	requestQueue *Queue
	holder       int
	asked        bool
	enterCSCh    chan struct{}
	log          logger.Logger
	mutex        *sync.Mutex
}

func NewNode(ID int, neighbourIDs []int, holder int) *Node {
	return &Node{nodeID: ID,
		neighbourIDs: neighbourIDs, requestQueue: NewQueue(), holder: holder, enterCSCh: make(chan struct{}, 1),
		log:   logger.Logger{Prefix: fmt.Sprintf("[%d]", ID)},
		mutex: &sync.Mutex{},
	}
}

func (r *Node) ID() int {
	return r.nodeID
}

func (r *Node) makeRequest() {
	var holderID int
	r.mutex.Lock()
	holderID = r.holder
	r.mutex.Unlock()
	if holderID != r.nodeID && !r.asked && r.requestQueue.Len() > 0 {
		m := message{SenderID: r.nodeID, Message: MessageRequest, ReceiverID: holderID}
		b, _ := json.Marshal(m)
		r.log.Println("->> ", string(b))
		if err := udpclient.SendMessage(fmt.Sprintf(":%d", 7000+m.ReceiverID), b); err != nil {
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
	if holderID == r.nodeID && !r.using && r.requestQueue.Len() > 0 {

		nextHolder := r.requestQueue.Dequeue().(int)

		// e := r.requestQueue.Front()
		// nextHolder := e.Value.(int)
		// r.requestQueue.Remove(e)

		if nextHolder == r.nodeID {
			r.log.Println("using token for myself")
			r.enterCSCh <- struct{}{}
			r.using = true
		} else {
			r.log.Println("giving privilege to ", nextHolder)
			m := message{SenderID: r.nodeID, Message: MessagePrivilege, ReceiverID: nextHolder}
			b, _ := json.Marshal(m)
			r.log.Println("->> ", string(b))
			if err := udpclient.SendMessage(fmt.Sprintf(":%d", 7000+m.ReceiverID), b); err != nil {
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

func (r *Node) AskToEnterCS() {
	// r.requestQueue.PushBack(r.nodeID)
	r.requestQueue.Enqueue(r.nodeID)
	if r.holder == r.nodeID {
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
		if r.holder == r.nodeID {
			r.assignPrivilege()
		} else {
			r.makeRequest()
		}
	case MessagePrivilege:
		r.holder = r.nodeID
		r.assignPrivilege()
	}
}
