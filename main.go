package main

import (
	"container/list"
	udpclient "distributed-lock-example/udpclient"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"time"
)

type position struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type lamport struct {
	nodeID  int
	clock   uint
	queue   *list.List
	replyCh chan struct{}
	replies int
	defered *list.List
	inCS    bool
}

func (l *lamport) processMessage(b []byte) {
	var m message
	if err := json.Unmarshal(b, &m); err != nil {
		log.Println("error unmarshalling message", err)
	}

	switch m.Message {
	case "request":
		log.Println("request came from ", m.SenderID)
		l.queue.PushBack(m)
		l.clock++
		reply := message{SenderID: l.nodeID, Message: "reply", Time: l.clock, ReceiverID: m.SenderID}
		if !l.inCS {
			log.Println("I am not on bridge. replying to ", reply.ReceiverID)
			b, _ := json.Marshal(reply)
			udpclient.SendMessage(fmt.Sprintf(":700%d", m.SenderID), b)
		} else {
			log.Println("I am on bridge. deferring reply to ", reply.ReceiverID)
			l.defered.PushBack(reply)
		}
	case "reply":
		log.Println("got permission to enter from ", m.SenderID)
		l.replies++
		l.replyCh <- struct{}{}
	case "release":
		for e := l.queue.Front(); e != nil; e = e.Next() {
			rm := e.Value.(message)
			if rm.SenderID == m.SenderID {
				l.queue.Remove(e)
				break
			}
		}
		l.replyCh <- struct{}{}
	}

}

type message struct {
	SenderID   int    `json:"senderId"`
	ReceiverID int    `json:"receiverId"`
	Message    string `json:"message"`
	Time       uint   `json:"time"`
}

type guiMessage struct {
	SenderID int      `json:"senderId"`
	Position position `json:"position"`
	Angle    float64  `json:"angle"`
}

var d = 10

// var bridgeStart = [2]int{157, 179}
// var bridgeEnd = [2]int{149, 227}
var ids = []int{0, 1, 2, 3}
var iterations = 5

type Direction string

var DirectionEast Direction = "east"
var DirectionWest Direction = "west"

type car struct {
	id        int
	onBridge  bool
	gui       *udpclient.Client
	clock     uint
	queue     *list.List
	replyCh   chan struct{}
	replies   int
	defered   *list.List
	direction Direction
	algo      lamport
}

func (c *car) enterBridge() {
	log.Println("entering bridge...")
	c.onBridge = true

}

func (c *car) leaveBridge() {
	log.Println("leaving bridge...")
	c.onBridge = false
	if c.direction == DirectionEast {
		c.direction = DirectionWest
	} else {
		c.direction = DirectionEast
	}
	// m := message{SenderID: c.id, Message: "release", Time: c.clock}
	// for _, id := range ids {
	// 	m.ReceiverID = id
	// 	if id != c.id {
	// 		b, _ := json.Marshal(m)
	// 		udpclient.SendMessage(fmt.Sprintf(":700%d", id), b)
	// 	}
	// }
}

func (c *car) enteringBridge(pos int, direction Direction) bool {
	return pos == bridgeStartEndIndices[direction][0]
	// if direction == DirectionEast {
	// 	return pos == bridgeStartEndIndices[0]
	// }
	// return pos == bridgeStartEndIndices[1]

}

func (c *car) leavingBridge(pos int, direction Direction) bool {
	return pos == bridgeStartEndIndices[direction][1]
	// if direction == DirectionEast {
	// 	return pos == bridgeStartEndIndices[1]
	// }

	// return pos == bridgeStartEndIndices[0]
}

func (c *car) askToEnterBridge() {
	c.clock++
	m := message{SenderID: c.id, Message: "request", Time: c.clock}
	c.queue.PushBack(m)
	for _, id := range ids {
		if id != c.id {
			log.Printf("asking %d to enter bridge...\n", id)
			m.ReceiverID = id
			b, _ := json.Marshal(m)
			udpclient.SendMessage(fmt.Sprintf(":700%d", id), b)
		}
	}
}

func (c *car) move(startPos int) {
	for i := startPos; i < len(travellingPath); i++ {
		pos := position{X: travellingPath[i][0], Y: travellingPath[i][1]}

		// log.Println("i, c.direction", i, c.direction)
		if c.enteringBridge(i, c.direction) {
			go c.askToEnterBridge()
			for {
				<-c.replyCh

				gotPermission := c.replies == len(ids)-1
				if gotPermission {
					m := c.queue.Front().Value.(message)
					if m.SenderID == c.id {
						break
					} else {
						log.Println("I got permission. but priority goes to ", m.SenderID)
					}
				}
			}
			c.replies = 0
			c.enterBridge()
		}
		if c.leavingBridge(i, c.direction) {
			c.leaveBridge()
			for e := c.defered.Front(); e != nil; e = e.Next() {
				log.Println(c.defered.Len())

				m := e.Value.(message)
				log.Println("replying to defered requests. receiver : ", m.ReceiverID)
				b, _ := json.Marshal(m)
				udpclient.SendMessage(fmt.Sprintf(":700%d", m.ReceiverID), b)
				// c.defered.Remove(e)
			}

			c.defered.Init()

			c.clock++
			for _, id := range ids {
				if id != c.id {
					m := message{SenderID: c.id, ReceiverID: id, Message: "release", Time: c.clock}
					b, _ := json.Marshal(m)
					udpclient.SendMessage(fmt.Sprintf(":700%d", id), b)
				}
			}

			for e := c.queue.Front(); e != nil; e = e.Next() {
				m := e.Value.(message)
				if m.SenderID == c.id {
					c.queue.Remove(e)
					break
				}
			}
		}

		if i%5 == 0 {
			m := guiMessage{SenderID: c.id, Position: pos}
			b, _ := json.Marshal(&m)
			if err := c.gui.Send(b); err != nil {
				log.Println(err)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	var id int
	flag.IntVar(&id, "id", -1, "id of car")
	flag.Parse()
	rand.Seed(time.Now().UnixNano() + int64(id))
	gui, err := udpclient.NewClient(":7500")
	if err != nil {
		os.Exit(0)
	}

	listenAddr := fmt.Sprintf(":700%d", id)

	s, err := net.ResolveUDPAddr("udp4", listenAddr)
	if err != nil {
		log.Fatalln(err)
	}

	conn, err := net.ListenUDP("udp4", s)
	if err != nil {
		log.Fatalln(err)
	}
	defer conn.Close()

	buffer := make([]byte, 1024)
	replyCh := make(chan struct{})

	carDirection := DirectionWest
	if id%2 == 0 {
		carDirection = DirectionEast
	}
	algo := lamport{nodeID: id, queue: list.New(), replyCh: replyCh, defered: list.New()}
	c := car{id: id, gui: gui, queue: list.New(), replyCh: replyCh, defered: list.New(), direction: carDirection, algo: algo}
	log.Println("bridgeStartEndIndices", bridgeStartEndIndices)

	go func(conn *net.UDPConn) {
		for {
			n, _, err := conn.ReadFromUDP(buffer)
			if err != nil {
				log.Println("failed to read from udp", err)
			}
			b := buffer[0 : n-1]
			log.Println("message: ", string(b))

			var m message
			if err := json.Unmarshal(b, &m); err != nil {
				log.Println("error unmarshalling message", err)
			}

			switch m.Message {
			case "request":
				log.Println("request came from ", m.SenderID)
				c.queue.PushBack(m)
				c.clock++
				reply := message{SenderID: c.id, Message: "reply", Time: c.clock, ReceiverID: m.SenderID}
				if !c.onBridge {
					log.Println("I am not on bridge. replying to ", reply.ReceiverID)
					b, _ := json.Marshal(reply)
					udpclient.SendMessage(fmt.Sprintf(":700%d", m.SenderID), b)
				} else {
					log.Println("I am on bridge. deferring reply to ", reply.ReceiverID)
					c.defered.PushBack(reply)
				}
			case "reply":
				log.Println("got permission to enter from ", m.SenderID)
				c.replies++
				c.replyCh <- struct{}{}
			case "release":
				for e := c.queue.Front(); e != nil; e = e.Next() {
					rm := e.Value.(message)
					if rm.SenderID == m.SenderID {
						c.queue.Remove(e)
						break
					}
				}
				c.replyCh <- struct{}{}
			}
		}
	}(conn)

	donCh := make(chan struct{})
	time.Sleep(time.Duration((rand.Intn(6) + 6)) * time.Second)

	for i := 0; i < iterations; i++ {
		if i == 0 {
			log.Println("my start position: ", carStartIndices[id])
			c.move(carStartIndices[id])
		} else {
			c.move(0)
		}
	}
	log.Println("✅ DONE")
	<-donCh

	// turnAngle := 0.0
	// for i, p := range track {
	// 	pos := position{X: p[0], Y: p[1]}
	// var start position
	// if i < d {
	// 	start = position{X: track[len(track)-1][0], Y: track[len(track)-1][1]}
	// } else {
	// 	start = position{X: track[i-d][0], Y: track[i-d][1]}
	// }
	// var end position
	// if len(track)-1 < i+d {
	// 	end = position{X: track[0][0], Y: track[0][1]}
	// } else {
	// 	end = position{X: track[i+d][0], Y: track[i+d][1]}
	// }
	// n := getDirection(start, pos, end) - turnAngle
	// if math.Abs(n) > 0.5 {
	// 	neg := math.Signbit(n)
	// 	if neg {
	// 		turnAngle = 0.1 - turnAngle
	// 	} else {
	// 		turnAngle = -0.1 - turnAngle
	// 	}
	// } else {
	// 	turnAngle = getDirection(start, pos, end) - turnAngle
	// }

	// if i%10 == 0 {
	// 	m := message{ID: 0, Position: pos, Angle: turnAngle}
	// 	b, _ := json.Marshal(&m)
	// 	if err := gui.Send(b); err != nil {
	// 		log.Println(err)
	// 	}
	// 	time.Sleep(100 * time.Millisecond)
	// }
	// }
}
