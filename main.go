package main

import (
	"container/list"
	raymond_K_entry "distributed-lock-example/raymond-K-entry"
	udpclient "distributed-lock-example/udpclient"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net"
	"os"
	"strconv"
	"time"
)

type position struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type Algorithm interface {
	ID() int
	ProcessMessage(b []byte)
	InCS() bool
	EnterCS()
	ExitCS()
	AskToEnterCS(CSID string)
	WaitForCS()
}

// var _ Algorithm = &raymod.Node{}
var _ Algorithm = &raymond_K_entry.Node{}

type guiMessage struct {
	SenderID int      `json:"senderId"`
	Position position `json:"position"`
	Angle    float64  `json:"angle"`
}

var d = 10

var iterations = 5

type Direction string

var DirectionEast Direction = "east"
var DirectionWest Direction = "west"

type car struct {
	// id        int
	// onBridge  bool
	gui *udpclient.Client
	// clock     uint
	queue *list.List
	// replyCh   chan struct{}
	// replies   int
	// defered   *list.List
	direction Direction
	algo      Algorithm
}

func (c *car) onBridge() bool {
	return c.algo.InCS()
}

func (c *car) enterBridge() {
	log.Println("entering bridge...")
	c.algo.EnterCS()

}

func (c *car) leaveBridge() {
	log.Println("leaving bridge...")
	c.algo.ExitCS()
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

func (c *car) askToEnterBridge(d Direction) {
	c.algo.AskToEnterCS(string(d))
}

func (c *car) waitForReply() {
	log.Println("waiting for permission...")
	c.algo.WaitForCS()
}

// func (c *car) askToEnterBridge() {
// 	c.clock++
// 	m := message{SenderID: c.id, Message: "request", Time: c.clock}
// 	c.queue.PushBack(m)
// 	for _, id := range ids {
// 		if id != c.id {
// 			log.Printf("asking %d to enter bridge...\n", id)
// 			m.ReceiverID = id
// 			b, _ := json.Marshal(m)
// 			udpclient.SendMessage(fmt.Sprintf(":700%d", id), b)
// 		}
// 	}
// }

func (c *car) move(startPos int) {
	for i := startPos; i < len(travellingPath); i++ {
		pos := position{X: travellingPath[i][0], Y: travellingPath[i][1]}

		// log.Println("i, c.direction", i, c.direction)
		if c.enteringBridge(i, c.direction) {
			go c.askToEnterBridge(c.direction)
			c.waitForReply()
			c.enterBridge()
		} else if c.leavingBridge(i, c.direction) {
			c.leaveBridge()
		}

		if i%5 == 0 {
			m := guiMessage{SenderID: c.algo.ID(), Position: pos}
			b, _ := json.Marshal(&m)
			if err := c.gui.Send(b); err != nil {
				log.Println(err)
			}
			time.Sleep(200 * time.Millisecond)
		}
	}
}

type ints []int

func (i *ints) String() string {
	return "my string representation"
}

func (i *ints) Set(value string) error {
	if v, err := strconv.Atoi(value); err != nil {
		*i = append(*i, v)
	}
	return nil
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	var id int
	var algorithm string

	var neighourIDs ints
	var holder int
	var tokens int
	flag.IntVar(&id, "id", -1, "id of car")
	flag.IntVar(&holder, "holder", -1, "id of car")
	flag.IntVar(&tokens, "tokens", 0, "id of car")
	flag.StringVar(&algorithm, "algorithm", "lamport", "raymond or lamport")
	flag.Var(&neighourIDs, "neighbour", "neighbour ids")
	flag.Parse()
	rand.Seed(time.Now().UnixNano() + int64(id))
	gui, err := udpclient.NewClient(":7500")
	if err != nil {
		os.Exit(0)
	}

	listenAddr := fmt.Sprintf(":%d", 7000+id)

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

	carDirection := DirectionWest
	if id%2 == 0 {
		carDirection = DirectionEast
	}

	var algo Algorithm
	if algorithm == "raymond" {
		log.Println("initial holder: ", holder)
		// algo = raymond.NewNode(id, neighourIDs, holder)
	} else if algorithm == "raymond-K-entry" {
		algo = raymond_K_entry.NewNode(id, neighourIDs, holder, tokens)
	} else {
		// algo = lamport.NewNode(id)
	}
	c := car{gui: gui, queue: list.New(), direction: carDirection, algo: algo}
	log.Println("bridgeStartEndIndices", bridgeStartEndIndices)

	go func(conn *net.UDPConn) {
		for {
			n, _, err := conn.ReadFromUDP(buffer)
			if err != nil {
				log.Println("failed to read from udp", err)
			}
			b := buffer[0 : n-1]
			log.Println("message: ", string(b))
			c.algo.ProcessMessage(b)

			// var m message
			// if err := json.Unmarshal(b, &m); err != nil {
			// 	log.Println("error unmarshalling message", err)
			// }

			// switch m.Message {
			// case "request":
			// 	log.Println("request came from ", m.SenderID)
			// 	c.queue.PushBack(m)
			// 	c.clock++
			// 	reply := message{SenderID: c.id, Message: "reply", Time: c.clock, ReceiverID: m.SenderID}
			// 	if !c.onBridge {
			// 		log.Println("I am not on bridge. replying to ", reply.ReceiverID)
			// 		b, _ := json.Marshal(reply)
			// 		udpclient.SendMessage(fmt.Sprintf(":700%d", m.SenderID), b)
			// 	} else {
			// 		log.Println("I am on bridge. deferring reply to ", reply.ReceiverID)
			// 		c.defered.PushBack(reply)
			// 	}
			// case "reply":
			// 	log.Println("got permission to enter from ", m.SenderID)
			// 	c.replies++
			// 	c.replyCh <- struct{}{}
			// case "release":
			// 	for e := c.queue.Front(); e != nil; e = e.Next() {
			// 		rm := e.Value.(message)
			// 		if rm.SenderID == m.SenderID {
			// 			c.queue.Remove(e)
			// 			break
			// 		}
			// 	}
			// 	c.replyCh <- struct{}{}
			// }
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
