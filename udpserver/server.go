package server

import (
	"log"
	"net"
)

type server struct {
	MessageCh *chan []byte
	Close     func()
}

func NewUDPServer(listenAddr string) server {
	s, err := net.ResolveUDPAddr("udp4", listenAddr)
	if err != nil {
		log.Fatalln(err)
	}

	conn, err := net.ListenUDP("udp4", s)
	if err != nil {
		log.Fatalln(err)
	}

	buffer := make([]byte, 1024)

	ch := make(chan []byte)
	closeFn := func() { conn.Close() }
	server := server{
		MessageCh: &ch,
		Close:     closeFn,
	}
	go func(conn *net.UDPConn) {
		for {
			n, _, err := conn.ReadFromUDP(buffer)
			if err != nil {
				log.Fatalln("failed to read from udp", err)
			}
			b := buffer[0 : n-1]
			log.Println("message: ", string(b))

			ch <- b
		}
	}(conn)
	return server
}
