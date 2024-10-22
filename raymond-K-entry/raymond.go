package raymod

import (
	"container/list"
	udpclient "distributed-lock-example/udpclient"
	"encoding/json"
	"fmt"
	"log"
	"net"
)

var MessageRequest string = "request"
var MessagePrivilege string = "privilege"

type message struct {
	SenderID     int    `json:"senderId"`
	ReceiverAddr string `json:"receiverAddr"`
	Message      string `json:"message"`
	CSID         string `json:"csId"`
}

type request struct {
	ID   int
	CSID string
}

type Node struct {
	nodeID       int
	neighbourIDs map[int]string
	using        bool
	requestQueue *list.List
	enterCSCh    chan struct{}
	tdb          *list.List // size will equal to number of tokens in system
	groupID      string     // ex: car moving from east->west belongs to group `0`, west->east belongs to group `1` and so on
	listenAddr   string
}

func NewNode(ID int, listenAddr string, neighbourIDs map[int]string, holder int, tokens int) *Node {
	tdb := list.New()
	for i := 0; i < tokens; i++ {
		r := request{ID: holder, CSID: ""}
		tdb.PushBack(r)
	}

	return &Node{
		nodeID: ID, neighbourIDs: neighbourIDs, requestQueue: list.New(), enterCSCh: make(chan struct{}, 1),
		tdb:        tdb,
		listenAddr: listenAddr,
	}
}

func (r *Node) deleteFromTDB(id int) {
	log.Println("deleting ", id, "from tdb. len: ", r.tdb.Len())
	for e := r.tdb.Front(); e != nil; e = e.Next() {
		if e.Value.(request).ID == id {
			r.tdb.Remove(e)
			break
		}
	}
	log.Println("deleted from tdb. len: ", r.tdb.Len())
}

func (r *Node) getOtherHolder() int {
	for e := r.tdb.Front(); e != nil; e = e.Next() {
		log.Println("getOtherHolder e.Value ", e.Value)
		if e.Value.(request).ID != r.nodeID {
			return e.Value.(request).ID
		}
	}
	return -1
}

func (r *Node) ID() int {
	return r.nodeID
}

func (r *Node) makeRequest(CSID string) {

	if !r.hasToken(CSID) && r.tdb.Len() > 0 && r.requestQueue.Len() > 0 {
		holder := r.getOtherHolder()
		m := message{SenderID: r.nodeID, Message: MessageRequest, CSID: CSID, ReceiverAddr: r.neighbourIDs[holder]}
		log.Println("request for CSID ", CSID, " to ", holder)
		b, _ := json.Marshal(m)
		if err := udpclient.SendMessage(m.ReceiverAddr, b); err != nil {
			// r.asked = true
			r.deleteFromTDB(holder)
		}
	}
}

func (r *Node) hasToken(CSID string) bool {
	hasToken := false
	for e := r.tdb.Front(); e != nil; e = e.Next() {
		if e.Value.(request).ID == r.nodeID && (r.groupID == "" || r.groupID == CSID) {
			hasToken = true
			break
		}
	}
	if !hasToken {
		log.Println("I dont have token for CSID: ", CSID, ". My CSID: ", r.groupID)
	}
	return hasToken
}

func (r *Node) assignPrivilege(CSID string) {
	log.Println("assign privilege called with CSID: ", CSID)
	log.Println("r.requestQueue.Len() ", r.requestQueue.Len())
	if r.hasToken(CSID) && r.requestQueue.Len() > 0 {
		log.Println("I have token")

		for e := r.requestQueue.Front(); e != nil; e = e.Next() {
			var nextHolder *request
			req := e.Value.(request)
			for e := r.tdb.Front(); e != nil; e = e.Next() {
				log.Println("r.tdb", e.Value)
				// t := e.Value.(request)

				if (CSID != "" && req.CSID == CSID) || (CSID == "") {
					nextHolder = &req
					break
				}
			}

			if nextHolder == nil {
				continue
			}

			log.Println("next holder: ", nextHolder)

			r.requestQueue.Remove(e)
			r.deleteFromTDB(r.nodeID)
			if nextHolder.ID == r.nodeID {
				log.Println("using token for myself. CSID: ", nextHolder.CSID)
				r.using = true
				r.groupID = nextHolder.CSID
				r.tdb.PushBack(*nextHolder)
				r.enterCSCh <- struct{}{}
			} else {
				log.Println("giving privilege to ", nextHolder)
				m := message{SenderID: r.nodeID, Message: MessagePrivilege, CSID: nextHolder.CSID, ReceiverAddr: r.neighbourIDs[nextHolder.ID]}
				b, _ := json.Marshal(m)
				if err := udpclient.SendMessage(m.ReceiverAddr, b); err != nil {
				} else {
					r.groupID = nextHolder.CSID
					r.tdb.PushBack(*nextHolder)
					log.Println("new holder is ", nextHolder.ID)
				}
			}
		}
	}
}

func (r *Node) AskToEnterCS(CSID string) {
	request := request{ID: r.nodeID, CSID: CSID}
	r.requestQueue.PushBack(request)
	if r.hasToken(CSID) {
		r.assignPrivilege(CSID)
	} else {
		r.makeRequest(CSID)
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
	r.groupID = ""
	r.assignPrivilege("")
}

func (r *Node) ProcessMessage(b []byte) {
	var m message
	json.Unmarshal(b, &m)
	switch m.Message {
	case MessageRequest:
		request := request{ID: m.SenderID, CSID: m.CSID}
		r.requestQueue.PushBack(request)
		if r.hasToken(m.CSID) {
			r.assignPrivilege(m.CSID)
		} else if r.getOtherHolder() != -1 && r.getOtherHolder() != m.SenderID {
			r.makeRequest(m.CSID)
		} else {
			log.Println("❗️ message got ignored")
		}
	case MessagePrivilege:
		req := request{ID: r.nodeID, CSID: m.CSID}
		r.tdb.PushBack(req)
		r.assignPrivilege(m.CSID)
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
