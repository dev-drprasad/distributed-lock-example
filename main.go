package main

import (
	"distributed-lock-example/lamport"
	lamport_K_entry "distributed-lock-example/lamport-K-entry"
	"distributed-lock-example/raymond"
	raymond_K_entry "distributed-lock-example/raymond-K-entry"
	udpclient "distributed-lock-example/udpclient"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
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
	Start()
}

var _ Algorithm = &raymond.Node{}
var _ Algorithm = &lamport.Node{}
var _ Algorithm = &lamport_K_entry.Node{}
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
	gui       *udpclient.Client
	direction Direction
	algo      Algorithm
}

func (c *car) start() {
	c.algo.Start()
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
}

func (c *car) enteringBridge(pos int, direction Direction) bool {
	return pos == bridgeStartEndIndices[direction][0]
}

func (c *car) leavingBridge(pos int, direction Direction) bool {
	return pos == bridgeStartEndIndices[direction][1]
}

func (c *car) askToEnterBridge(d Direction) {
	log.Println("asking to enter bridge")
	c.algo.AskToEnterCS(string(d))
}

func (c *car) waitForReply() {
	log.Println("waiting for permission...")
	c.algo.WaitForCS()
}

func (c *car) startMovement(startPos int) {
	for i := startPos; i < len(travellingPath); i++ {
		pos := position{X: travellingPath[i][0], Y: travellingPath[i][1]}

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

type strs []string

type neighboursFlag map[int]string

func (i *neighboursFlag) String() string {
	return ""
}

func (i *neighboursFlag) Set(value string) error {
	splitted := strings.SplitN(value, ":", 2)
	if len(splitted) != 2 {
		return errors.New("must be in format of <id>:<addr>")
	}
	id, err := strconv.Atoi(splitted[0])
	if err != nil {
		return err
	}
	if *i == nil {
		*i = map[int]string{}
	}
	(*i)[id] = splitted[1]
	// *i = append(*i, value)
	return nil
}

func (i *strs) String() string {
	return ""
}

func (i *strs) Set(value string) error {
	*i = append(*i, value)
	return nil
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	var id int
	var algorithm string
	var listenAddr string

	var neighbours neighboursFlag
	var holder int
	var tokens int
	flag.IntVar(&id, "id", -1, "id of car")
	flag.IntVar(&holder, "holder", -1, "id of car")
	flag.IntVar(&tokens, "tokens", 0, "id of car")
	flag.StringVar(&algorithm, "algorithm", "lamport", "raymond or lamport")
	flag.StringVar(&listenAddr, "listen", "", "raymond or lamport")
	flag.Var(&neighbours, "neighbour", "neighbour ids")
	flag.Parse()
	rand.Seed(time.Now().UnixNano() + int64(id))

	gui, err := udpclient.NewClient(":7500")
	if err != nil {
		os.Exit(0)
	}

	carDirection := DirectionWest
	if id%2 == 0 {
		carDirection = DirectionEast
	}

	var algo Algorithm
	switch algorithm {
	case "raymond":
		algo = raymond.NewNode(id, listenAddr, neighbours, holder)
	case "lamport-K-entry":
		algo = lamport_K_entry.NewNode(id, listenAddr, neighbours)
	case "raymond-K-entry":
		algo = raymond_K_entry.NewNode(id, listenAddr, neighbours, holder, tokens)
	default:
		algo = lamport.NewNode(id, listenAddr, neighbours)
	}

	c := car{gui: gui, direction: carDirection, algo: algo}

	go c.start()

	doneCh := make(chan struct{})
	time.Sleep(time.Duration((rand.Intn(6) + 6)) * time.Second)

	for i := 0; i < iterations; i++ {
		if i == 0 {
			log.Println("my start position: ", carStartIndices[id])
			c.startMovement(carStartIndices[id])
		} else {
			c.startMovement(0)
		}
	}
	log.Println("✅ DONE")
	<-doneCh
}
