package main

import (
	"distributed-lock-example/lamport"
	raymod "distributed-lock-example/raymond"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net"
	"sort"
	"strconv"
	"sync"
	"time"

	"gonum.org/v1/plot"
	"gonum.org/v1/plot/plotter"
	"gonum.org/v1/plot/plotutil"
	"gonum.org/v1/plot/vg"
)

func calcMedian(n ...float64) float64 {
	sort.Float64s(n) // sort the numbers
	fmt.Println(n)

	mNumber := len(n) / 2

	if len(n)%2 == 0 {
		return (n[mNumber-1] + n[mNumber]) / 2
	}

	return n[mNumber]
}

func makeTree2(n int) map[int][2]interface{} {
	parentchild := map[int][]int{}
	args := map[int][2]interface{}{}
	for i := 0; i < n; i++ {
		left := i*2 + 1
		if left == n {
			break
		}
		parentchild[i] = []int{left}
		right := i*2 + 2
		if right == n {
			break
		}
		parentchild[i] = append(parentchild[i], right)
		if i != 0 {
		}
	}
	for i := 0; i < n; i++ {
		neighbours := []int{}
		holder := 0
		if i != 0 {
			holder = (i - 1) / 2
			neighbours = append(neighbours, parentchild[i]...)
			neighbours = append(neighbours, holder) // holder is also a neighbour
		}
		args[i] = [2]interface{}{neighbours, holder}
	}
	return args
}
func getParentChildRelations(n int) map[int][]int {
	parentchild := map[int][]int{}
	for i := 0; i < n; i++ {
		left := i*2 + 1
		if left == n {
			break
		}
		parentchild[i] = []int{left}
		right := i*2 + 2
		if right == n {
			break
		}
		parentchild[i] = append(parentchild[i], right)
		if i != 0 {
		}
	}

	return parentchild
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

var _ Algorithm = &raymod.Node{}
var _ Algorithm = &lamport.Node{}

type TestNode struct {
	numOfMessages int
	Algorithm
	endedCh chan struct{}
}

func (n *TestNode) ProcessMessageDebug(b []byte) {
	n.numOfMessages++
	n.ProcessMessage(b)
}

func (t *TestNode) Start() {
	listenAddr := fmt.Sprintf(":%d", 7000+t.ID())

	s, err := net.ResolveUDPAddr("udp4", listenAddr)
	if err != nil {
		log.Fatalln(err)
	}

	conn, err := net.ListenUDP("udp4", s)
	if err != nil {
		log.Fatalln(err)
	}
	defer conn.Close()

	go func() {
		<-t.endedCh
		conn.Close()
	}()

	for {
		buffer := make([]byte, 1024)
		n, _, err := conn.ReadFromUDP(buffer)
		if err != nil {
			log.Println("failed to read from udp: ", err)
			break
		}
		b := buffer[0 : n-1]
		log.Println(fmt.Sprintf("[%d]", t.ID()), "<<- ", string(b))
		go t.ProcessMessageDebug(b)
	}
}

func NewLamportNode(id int, neighbourIDs []int) *TestNode {
	var neighbours map[int]string
	for _, id := range neighbourIDs {
		neighbours[id] = fmt.Sprintf(":%d", 7000+id)
	}
	ln := lamport.NewNode(id, fmt.Sprintf(":%d", 7000+id), neighbours)
	return &TestNode{0, ln, make(chan struct{}, 1)}
}

func NewRaymondNode(id int, neighbourIDs []int, holderID int) *TestNode {
	var neighbours map[int]string
	for _, id := range neighbourIDs {
		neighbours[id] = fmt.Sprintf(":%d", 7000+id)
	}
	rn := raymod.NewNode(id, fmt.Sprintf(":%d", 7000+id), neighbours, holderID)
	return &TestNode{0, rn, make(chan struct{}, 1)}
}

type resultT struct {
	avgNumOfMessages int
	avgCSWaitTime    float64 // in seconds
	avgThroughput    float64 // in seconds
}

func Init(algo string, id int, numOfNodes int, neighbourIDs []int, holderID int) (*resultT, error) {
	result := resultT{}
	if id < 0 {
		return nil, errors.New("id must be non negative integer")
	}

	var node *TestNode
	switch algo {
	case "raymond":
		node = NewRaymondNode(id, neighbourIDs, holderID)
	case "lamport":
		node = NewLamportNode(id, neighbourIDs)
	default:
		return nil, errors.New("unknown algorithm specified")
	}

	go node.Start()

	// wait for others to join
	time.Sleep(time.Duration((rand.Intn(3) + 2)) * time.Second)

	timeTakens := []float64{}
	csWaitTimes := []float64{}
	iterations := 15
	for i := 0; i < iterations; i++ {
		time.Sleep(1200 * time.Millisecond)
		node.AskToEnterCS("")
		waitStartTime := time.Now()
		node.WaitForCS()
		// result.avgCSWaitTime += float64(time.Now().Sub(waitStartTime)) / float64(iterations)
		csWaitTimes = append(csWaitTimes, float64(time.Now().Sub(waitStartTime))/float64(iterations))
		node.EnterCS()
		time.Sleep(700 * time.Millisecond)
		node.ExitCS()
		timeTakens = append(timeTakens, float64(time.Now().Sub(waitStartTime)))
	}

	log.Println("✅ DONE")
	<-time.After(time.Duration(numOfNodes) * time.Second) // let others to complete
	node.endedCh <- struct{}{}

	result.avgCSWaitTime = calcMedian(csWaitTimes...) / float64(time.Second) // convert to Seconds
	result.avgThroughput = calcMedian(timeTakens...) / float64(time.Second)  // convert to Seconds
	result.avgNumOfMessages = node.numOfMessages / iterations

	return &result, nil
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	var id int
	var numOfNodes int

	flag.IntVar(&id, "id", -1, "id of node")
	flag.IntVar(&numOfNodes, "num-of-nodes", 0, "number of nodes in mesh")
	flag.Parse()

	rand.Seed(time.Now().UnixNano() + int64(id))

	nodes := []int{3, 6, 9, 12}
	// nodes := []int{5, 10, 15, 20, 25, 30, 35}

	result := map[int]*resultT{}
	for _, numOfNodes := range nodes {
		var wg sync.WaitGroup
		res := resultT{}
		for i := 0; i < numOfNodes; i++ {
			wg.Add(1)
			go func(id int, res *resultT, wg *sync.WaitGroup) {
				defer wg.Done()

				neighbourIDs := []int{}
				for i := 0; i < numOfNodes; i++ {
					if i != id {
						neighbourIDs = append(neighbourIDs, i)
					}
				}

				// incase of lamport, holderID is ignored
				r, err := Init("lamport", id, numOfNodes, neighbourIDs, 0)
				if err != nil {
					log.Fatalln(err)

				}

				res.avgCSWaitTime += r.avgCSWaitTime
				res.avgNumOfMessages += r.avgNumOfMessages
				res.avgThroughput += r.avgThroughput
			}(i, &res, &wg)
		}
		result[numOfNodes] = &res
		wg.Wait()
	}

	result2 := map[int]*resultT{}
	for _, numOfNodes := range nodes {
		parentchild := getParentChildRelations(numOfNodes)
		var wg sync.WaitGroup
		res := resultT{}
		for i := 0; i < numOfNodes; i++ {
			neighourIDs := []int{}
			holderID := 0
			if i != 0 {
				holderID = (i - 1) / 2
				neighourIDs = append(neighourIDs, parentchild[i]...)
				neighourIDs = append(neighourIDs, holderID) // holder is also a neighbour
			}
			wg.Add(1)
			go func(id int, neighourIDs []int, holderID int, res *resultT, wg *sync.WaitGroup) {
				defer wg.Done()

				r, err := Init("raymond", id, numOfNodes, neighourIDs, holderID)
				if err != nil {
					log.Fatalln(err)

				}

				res.avgCSWaitTime += r.avgCSWaitTime
				res.avgNumOfMessages += r.avgNumOfMessages
				res.avgThroughput += r.avgThroughput
			}(i, neighourIDs, holderID, &res, &wg)
		}
		result2[numOfNodes] = &res
		wg.Wait()
	}

	log.Printf("Algo\t|Nodes\t|Messages (avg)\t|CS waiting time (median) (sec)\t|Time taken to complete CS (median) (sec)\n")
	log.Println("-----|-------|--------------------|---------------|------------")
	for _, numOfNodes := range nodes {
		log.Printf("%s\t\t|%d\t\t|%d\t\t|%.2f\t\t|%.2f", "Lamport", numOfNodes, result[numOfNodes].avgNumOfMessages, result[numOfNodes].avgCSWaitTime, result[numOfNodes].avgThroughput)
	}
	for _, numOfNodes := range nodes {
		log.Printf("%s\t\t|%d\t\t|%d\t\t|%.2f\t\t|%.2f", "Raymond", numOfNodes, result2[numOfNodes].avgNumOfMessages, result2[numOfNodes].avgCSWaitTime, result2[numOfNodes].avgThroughput)
	}

	lamportResult := make(plotter.XYs, len(result))
	raymondResult := make(plotter.XYs, len(result2))
	for i, numOfNodes := range nodes {
		lamportResult[i].X = float64(numOfNodes)
		lamportResult[i].Y = float64(result[numOfNodes].avgNumOfMessages)

		raymondResult[i].X = float64(numOfNodes)
		raymondResult[i].Y = float64(result2[numOfNodes].avgNumOfMessages)
	}

	p, err := plot.New()
	if err != nil {
		panic(err)
	}

	p.Title.Text = "(num of msgs / num of nodes) vs num of nodes"
	p.Y.Label.Text = "num of msgs ÷ num of nodes"
	p.X.Label.Text = "num of nodes"

	p.X.Tick.Marker = MyTicks{ticksAt: nodes}

	err = plotutil.AddLinePoints(p, "Lamport", lamportResult, "Raymond", raymondResult)
	if err != nil {
		panic(err)
	}

	// Save the plot to a PNG file.
	if err := p.Save(6*vg.Inch, 4*vg.Inch, "report/messages.png"); err != nil {
		panic(err)
	}

	lamportResponseTimeResult := make(plotter.XYs, len(result))
	raymondResponseTimeResult := make(plotter.XYs, len(result2))
	for i, numOfNodes := range nodes {
		lamportResponseTimeResult[i].X = float64(numOfNodes)
		lamportResponseTimeResult[i].Y = float64(result[numOfNodes].avgCSWaitTime)

		raymondResponseTimeResult[i].X = float64(numOfNodes)
		raymondResponseTimeResult[i].Y = float64(result2[numOfNodes].avgCSWaitTime)
	}

	pp, err := plot.New()
	if err != nil {
		panic(err)
	}

	pp.Title.Text = "CS wait time vs num of nodes"
	pp.Y.Label.Text = "CS wait time (ms)"
	pp.X.Label.Text = "num of nodes"

	pp.X.Tick.Marker = MyTicks{ticksAt: nodes}

	err = plotutil.AddLinePoints(pp, "Lamport", lamportResponseTimeResult, "Raymond", raymondResponseTimeResult)
	if err != nil {
		panic(err)
	}

	// Save the plot to a PNG file.
	if err := pp.Save(6*vg.Inch, 4*vg.Inch, "report/response_time.png"); err != nil {
		panic(err)
	}

	lamportThroughputResult := make(plotter.XYs, len(result))
	raymondthroughputResult := make(plotter.XYs, len(result2))
	for i, numOfNodes := range nodes {
		lamportThroughputResult[i].X = float64(numOfNodes)
		lamportThroughputResult[i].Y = result[numOfNodes].avgThroughput

		raymondthroughputResult[i].X = float64(numOfNodes)
		raymondthroughputResult[i].Y = result2[numOfNodes].avgThroughput
	}

	ppp, err := plot.New()
	if err != nil {
		panic(err)
	}

	ppp.Title.Text = "Throughput vs num of nodes"
	ppp.Y.Label.Text = "throughput (num of CS enters per sec)"
	ppp.X.Label.Text = "num of nodes"

	ppp.X.Tick.Marker = MyTicks{ticksAt: nodes}

	err = plotutil.AddLinePoints(ppp, "Lamport", lamportThroughputResult, "Raymond", raymondthroughputResult)
	if err != nil {
		panic(err)
	}

	// Save the plot to a PNG file.
	if err := ppp.Save(6*vg.Inch, 4*vg.Inch, "report/throughput.png"); err != nil {
		panic(err)
	}
}

type MyTicks struct {
	ticksAt []int
}

// Ticks returns Ticks in the specified range.
func (t MyTicks) Ticks(min, max float64) []plot.Tick {
	var ticks []plot.Tick
	for _, v := range t.ticksAt {
		ticks = append(ticks, plot.Tick{Value: float64(v), Label: strconv.FormatFloat(float64(v), 'f', 0, 64)})
	}

	return ticks
}

type DurationTicks struct {
	values []float64
}

func (t DurationTicks) Ticks(min, max float64) []plot.Tick {
	ticks := []plot.Tick{}
	for _, v := range t.values {
		ticks = append(ticks, plot.Tick{Value: v, Label: fmt.Sprintf("%.2f ms", v)})
	}
	return ticks
}
