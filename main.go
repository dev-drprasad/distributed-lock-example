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

var iterations = 4

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

		// this is kind debouncing.
		// we are not sending each and every positions to GUI. It will blow up GUI
		// low value means slow motion, high value means faster motion
		sampleRate := 20
		if i%sampleRate == 0 {
			if c.gui != nil {
				m := guiMessage{SenderID: c.algo.ID(), Position: pos}
				b, _ := json.Marshal(&m)
				if err := c.gui.Send(b); err != nil {
					// log.Println(err)
				}
			}
			// min: 100ms. different cars move with different speeds
			time.Sleep(time.Duration(rand.Intn(100*(c.algo.ID()+1))+100) * time.Millisecond)
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
	var guiAddr string

	var neighbours neighboursFlag
	var holder int
	var tokens int
	flag.IntVar(&id, "id", -1, "id of car")
	flag.IntVar(&holder, "holder", -1, "initial holder of token") // applicable to raymond and raymond-K-entry
	flag.IntVar(&tokens, "tokens", 0, "num of tokens")            // applicable only to ramond-K-entry
	flag.StringVar(&algorithm, "algorithm", "lamport", "raymond or lamport")
	flag.StringVar(&listenAddr, "listen", "", "own listening address")
	flag.StringVar(&guiAddr, "gui", "", "address of GUI")
	flag.Var(&neighbours, "neighbour", "neighbour ids")
	flag.Parse()
	rand.Seed(time.Now().UnixNano() + int64(id))

	var gui *udpclient.Client
	var err error
	if guiAddr != "" {
		gui, err = udpclient.NewClient(guiAddr)
		if err != nil {
			os.Exit(0)
		}
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

	c := car{gui: gui,
		direction: carDirection,
		algo:      algo,
	}

	go c.start()

	doneCh := make(chan struct{})
	time.Sleep(time.Duration((rand.Intn(6) + 6)) * time.Second) // wait for others to join

	for i := 0; i < iterations; i++ {
		if i == 0 {
			log.Println("my start position: ", carStartIndices[id])
			c.startMovement(carStartIndices[id])
		} else {
			c.startMovement(0)
		}
	}
	log.Println("✅ DONE")
	<-doneCh // donot exit program. because it still needs to respond other unfinished peers
}
