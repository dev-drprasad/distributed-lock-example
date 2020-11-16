package main

import (
	"io/ioutil"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"time"
)

func reversePath(input [][2]int) [][2]int {
	if len(input) == 0 {
		return input
	}
	return append(reversePath(input[1:]), input[0])
}

func parseCoords(s string) [][2]int {
	splitted := strings.Split(s, " ")
	o := [][2]int{}
	for _, w := range splitted {
		xy := strings.Split(strings.TrimSpace(w), ",")
		x, _ := strconv.Atoi(xy[0])
		y, err := strconv.Atoi(xy[1])
		if err != nil {
			log.Println(err, xy)
		}
		o = append(o, [2]int{x, y})
	}
	return o
}

func coordsFromFile(filename string) [][2]int {
	b, err := ioutil.ReadFile(filename)
	if err != nil {
		os.Exit(1)
	}
	return parseCoords(string(b))
}

var travellingPath [][2]int
var bridgeStartEndIndices map[Direction][2]int
var carStartIndices [4]int

func init() {
	rand.Seed(time.Now().UnixNano())

	bridge := coordsFromFile("./paths/bridge.txt")
	leftsidecircle := coordsFromFile("./paths/leftsidecircle.txt")
	rightsidecircle := coordsFromFile("./paths/rightsidecircle.txt")

	reversedBridge := reversePath(bridge)
	path1 := append(leftsidecircle, bridge...)
	path2 := append(rightsidecircle, reversedBridge...)

	travellingPath = append(path1, path2...)

	bridgeStartEndIndices = map[Direction][2]int{
		DirectionEast: {len(leftsidecircle), len(leftsidecircle) + len(bridge)},
		DirectionWest: {len(leftsidecircle) + len(bridge) + len(rightsidecircle), len(travellingPath) - 1},
	}

	carStartIndices = [4]int{
		rand.Intn(len(leftsidecircle)),
		rand.Intn(len(rightsidecircle)) + len(bridge) + len(leftsidecircle),
		rand.Intn(len(leftsidecircle)),
		rand.Intn(len(rightsidecircle)) + len(bridge) + len(leftsidecircle),
	}
}
