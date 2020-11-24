package client

import (
	"log"
	"net"
)

type Client struct {
	conn *net.UDPConn
}

func NewClient(address string) (*Client, error) {
	s, err := net.ResolveUDPAddr("udp4", address)
	if err != nil {
		return nil, err
	}
	conn, err := net.DialUDP("udp4", nil, s)
	if err != nil {
		return nil, err
	}

	return &Client{conn: conn}, nil
}

func (c *Client) Init(address string) error {
	if c.conn == nil {
		s, err := net.ResolveUDPAddr("udp4", address)
		if err != nil {
			return err
		}
		conn, err := net.DialUDP("udp4", nil, s)
		if err != nil {
			return err
		}
		c.conn = conn
	}
	return nil
}

func (c *Client) Send(data []byte) error {
	_, err := c.conn.Write(append(data, []byte("\n")...))
	if err != nil {
		return err
	}
	return nil
}

func SendMessage(addr string, data []byte) error {
	s, err := net.ResolveUDPAddr("udp4", addr)
	if err != nil {
		return err
	}
	conn, err := net.DialUDP("udp4", nil, s)
	if err != nil {
		return err
	}
	defer conn.Close()

	log.Println("addr: ", addr)
	_, err = conn.Write(append(data, []byte("\n")...))
	if err != nil {
		return err
	}
	return nil
}
