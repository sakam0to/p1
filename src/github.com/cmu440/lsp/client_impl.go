// Contains the implementation of a LSP client.

package lsp

import (
	"encoding/json"
	"fmt"
	"github.com/cmu440/lspnet"
)

type client struct {
	conn *lspnet.UDPConn
	connID int
	seq int
	unackedCount int
	params *Params
	writeChan chan []byte
	readChan chan []byte
	closeWrite chan bool
	closeRead chan bool
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
	raddr, _ := lspnet.ResolveUDPAddr("udp", hostport)
	conn, _ := lspnet.DialUDP("udp", nil, raddr)
	msg := NewConnect()
	var b []byte = make([]byte, 1024)
	var b_ []byte = make([]byte, 1024)
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
			client := &client{
				conn: conn,
				connID: rcvMsg.ConnID,
				seq: 1,
				unackedCount: 0,
				params: params,
				writeChan: make(chan []byte),
				readChan: make(chan []byte),
				closeWrite: make(chan bool),
				closeRead: make(chan bool),
			}
			go client.ReadRoutine()
			go client.WriteRoutine()
			return client, err
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

func (c *client) ReadRoutine() {
	for {
		select {
		case <-c.closeRead:
			return
		default:
			var rcvMsg Message
			var b []byte = make([]byte, 1024)
			// Todo: Capture error handling in the new routines to send out through channel struct field
			n, _ := c.conn.Read(b)
			_ = json.Unmarshal(b[0:n], &rcvMsg)
			if rcvMsg.Type == MsgData {
				ack := NewAck(c.connID, rcvMsg.SeqNum)
				b, _ = json.Marshal(ack)
				_, _ = c.conn.Write(b)
				c.readChan <- rcvMsg.Payload
			} else if rcvMsg.Type == MsgAck {
				// May need to improve this for race and other stability
				c.unackedCount--
			}
		}
	}
}

func (c *client) WriteRoutine() {
	for {
		select {
		case <-c.closeWrite:
			return
		default:
			payload := <-c.writeChan
			checksum := ByteArray2Checksum(payload)
			msg := NewData(c.connID, c.seq, len(payload), payload, uint16(checksum))
			b, _ := json.Marshal(msg)
			for {
				if c.unackedCount < c.params.MaxUnackedMessages {
					_, _ = c.conn.Write(b)
					// Might need to send increments over a channel to pass race
					c.unackedCount++
					break
				}
			}
			c.seq++
		}
	}
}

func (c *client) ConnID() int {
	return c.connID
}

func (c *client) Read() ([]byte, error) {
	read := <-c.readChan
	return read, nil
}

func (c *client) Write(payload []byte) error {
	c.writeChan <- payload
	return nil
}

func (c *client) Close() error {
	c.closeRead <- true
	c.closeWrite <- true
	return nil
}
