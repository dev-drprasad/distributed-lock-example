package main

import (
	"encoding/json"
	"flag"
	_ "image/png"
	"log"
	"net"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
)

type position struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type message struct {
	SenderID uint     `json:"senderId"`
	Position position `json:"position"`
	Angle    float64  `json:"angle"`
}

const (
	screenWidth  = 640
	screenHeight = 480
	maxAngle     = 256
)

type Sprite struct {
	imageWidth  int
	imageHeight int
	x           int
	y           int
	vx          int
	vy          int
	angle       float64
}

type Sprites struct {
	sprites []*Sprite
	num     int
}

func (g *Game) Update() error {
	if !g.inited {
		g.init()
	}
	g.sprites.Update()

	return nil
}

func (s *Sprite) Update() {
	// s.x += 1
}

func (s *Sprites) Update() {
	for i := 0; i < s.num; i++ {
		s.sprites[i].Update()
	}
}

var (
	bg   *ebiten.Image
	car0 *ebiten.Image
	car1 *ebiten.Image
	car2 *ebiten.Image
	car3 *ebiten.Image
)

var cars []*ebiten.Image

type Game struct {
	sprites Sprites
	op      ebiten.DrawImageOptions
	inited  bool
}

var spriteScale = 0.75

func init() {
	// Decode image from a byte slice instead of a file so that
	// this example works in any working directory.
	// If you want to use a file, there are some options:
	// 1) Use os.Open and pass the file to the image decoder.
	//    This is a very regular way, but doesn't work on browsers.
	// 2) Use ebitenutil.OpenFile and pass the file to the image decoder.
	//    This works even on browsers.
	// 3) Use ebitenutil.NewImageFromFile to create an ebiten.Image directly from a file.
	//    This also works on browsers.
	car0PNG, _, err := ebitenutil.NewImageFromFile("./gui/resources/car0.png")
	if err != nil {
		log.Fatal(err)
	}
	car1PNG, _, err := ebitenutil.NewImageFromFile("./gui/resources/car1.png")
	if err != nil {
		log.Fatal(err)
	}
	car2PNG, _, err := ebitenutil.NewImageFromFile("./gui/resources/car2.png")
	if err != nil {
		log.Fatal(err)
	}
	car3PNG, _, err := ebitenutil.NewImageFromFile("./gui/resources/car3.png")
	if err != nil {
		log.Fatal(err)
	}
	bgPNG, _, err := ebitenutil.NewImageFromFile("./gui/resources/bg.png")
	if err != nil {
		log.Fatal(err)
	}

	bg = ebiten.NewImage(screenWidth, screenHeight)

	w0, h0 := car0PNG.Size()
	car0 = ebiten.NewImage(w0, h0)
	w1, h1 := car1PNG.Size()
	car1 = ebiten.NewImage(w1, h1)
	w2, h2 := car2PNG.Size()
	car2 = ebiten.NewImage(w2, h2)
	w3, h3 := car3PNG.Size()
	car3 = ebiten.NewImage(w3, h3)

	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(spriteScale, spriteScale)

	car0.DrawImage(car0PNG, op)
	car1.DrawImage(car1PNG, op)
	car2.DrawImage(car2PNG, op)
	car3.DrawImage(car3PNG, op)
	bgops := ebiten.DrawImageOptions{}
	op.GeoM.Scale(1, 1)
	bg.DrawImage(bgPNG, &bgops)
	cars = append(cars, car0, car1, car2, car3)
}

func (g *Game) init() {
	defer func() {
		g.inited = true
	}()

	g.sprites.sprites = make([]*Sprite, 4)
	g.sprites.num = 4
	w, h := car1.Size()
	for i := range g.sprites.sprites {
		g.sprites.sprites[i] = &Sprite{
			imageWidth:  w,
			imageHeight: h,
			x:           10,
			y:           10,
			angle:       0,
		}
	}
}

func (g *Game) Draw(screen *ebiten.Image) {
	// Draw each sprite.
	// DrawImage can be called many many times, but in the implementation,
	// the actual draw call to GPU is very few since these calls satisfy
	// some conditions e.g. all the rendering sources and targets are same.
	// For more detail, see:
	// https://pkg.go.dev/github.com/hajimehoshi/ebiten/v2#Image.DrawImage

	bgops := ebiten.DrawImageOptions{}
	screen.DrawImage(bg, &bgops)

	for i, s := range g.sprites.sprites {
		w, h := cars[i].Size()

		g.op.GeoM.Reset()

		g.op.GeoM.Translate(-float64(w)*spriteScale/2, -float64(h)*spriteScale/2)

		g.op.GeoM.Translate(float64(s.x), float64(s.y))

		screen.DrawImage(cars[i], &g.op)
	}
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	var listenAddr string
	flag.StringVar(&listenAddr, "listen", "0.0.0.0:7500", "listen address <ip>:<port> of GUI")
	flag.Parse()

	g := Game{}

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

			g.sprites.sprites[m.SenderID].x = m.Position.X
			g.sprites.sprites[m.SenderID].y = m.Position.Y
			g.sprites.sprites[m.SenderID].angle = m.Angle
		}
	}(conn)

	ebiten.SetMaxTPS(25)
	windowScale := 1
	ebiten.SetWindowSize(screenWidth*windowScale, screenHeight*windowScale)
	ebiten.SetWindowTitle("Sprites (Ebiten Demo)")
	if err := ebiten.RunGame(&g); err != nil {
		log.Fatal(err)
	}
}
