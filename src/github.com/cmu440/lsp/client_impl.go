// Contains the implementation of a LSP client.

package lsp

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/cmu440/lspnet"
)

var id = 1
var seq = 1

type client struct {
	conn *lspnet.UDPConn
	connID int
}

// NewClient creates, initiates, and returns a new client. This function
// should return after a connection with the server has been established
// (i.e., the client has received an Ack message from the server in response
// to its connection request), and should return a non-nil error if a
// connection could not be made (i.e., if after K epochs, the client still
// hasn't received an Ack message from the server in response to its K
// connection requests).
//
// hostport is a colon-separated string identifying the server's host address
// and port number (i.e., "localhost:9999").
func NewClient(hostport string, params *Params) (Client, error) {
	var newClient Client
	raddr, _ := lspnet.ResolveUDPAddr("udp", hostport)
	conn, _ := lspnet.DialUDP("udp", nil, raddr)
	msg := NewConnect()
	var b []byte = make([]byte, 1000)
	var b_ []byte = make([]byte, 1000)
	b, _ = json.Marshal(msg)
	_, _ = conn.Write(b)
	for {
		// Eventually some epoch logic here?
		// Currently assuming the ack is going to arrive eventually
		// Will need to get the connect requesting write from above in the loop accordingly
		var rcvMsg Message
		n, err := conn.Read(b_)
		err = json.Unmarshal(b_[0:n], &rcvMsg)
		if rcvMsg.Type == MsgAck {
			fmt.Printf("Received Ack!\n")
			newClient = &client{
				conn: conn,
				connID: id,
			}
			id++
			return newClient, err
		} else {
			fmt.Printf("Connect received: %s\n", string(b_))
			err = json.Unmarshal(b_, &rcvMsg)
			if err != nil {
				fmt.Println("Unmarshal error", err)
			} else {
				println(rcvMsg.Type)
				println(rcvMsg.ConnID)
			}
		}
	}
}

func (c *client) ConnID() int {
	return c.connID
}

func (c *client) Read() ([]byte, error) {
	for {
		var rcvMsg Message
		var b []byte = make([]byte, 1000)
		n, err := c.conn.Read(b)
		err = json.Unmarshal(b[0:n], &rcvMsg)
		fmt.Printf("Read: %s\n", string(b))
		if rcvMsg.Type == MsgData {
			ack := NewAck(c.connID, rcvMsg.SeqNum)
			b, _ = json.Marshal(ack)
			_, _ = c.conn.Write(b)
			return rcvMsg.Payload, err
		}
	}
}

func (c *client) Write(payload []byte) error {
	checksum := ByteArray2Checksum(payload)
	msg := NewData(c.connID, seq, len(payload), payload, uint16(checksum))
	b, err := json.Marshal(msg)
	_, err = c.conn.Write(b)
	seq++
	return err
}

func (c *client) Close() error {
	return errors.New("not yet implemented")
}
