// Contains the implementation of a LSP client.

package lsp

import (
	"encoding/json"
	"github.com/cmu440/lspnet"
)

type client struct {
    conn         *lspnet.UDPConn
    connID       int
    seq          int
    unackedCount int
    params       *Params
    //TODO: writeChan and readChan should not be used to store arbitrary length data; Need to switch to linked list.
    writeChan    chan []byte
    readChan     chan []byte
    setUnackChan chan int
    getUnackChan chan bool
    retUnackChan chan int
    closeMain    chan bool
    closeWrite   chan bool
    closeRead    chan bool
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
        var rcvMsg Message
        n, err := conn.Read(b_)
        err = json.Unmarshal(b_[0:n], &rcvMsg)
        if rcvMsg.Type == MsgAck {
            client := &client{
                conn:         conn,
                connID:       rcvMsg.ConnID,
                seq:          1,
                unackedCount: 0,
                params:       params,
                writeChan:    make(chan []byte),
                readChan:     make(chan []byte),
                setUnackChan: make(chan int),
                getUnackChan: make(chan bool),
                retUnackChan: make(chan int),
                closeMain:    make(chan bool),
                closeWrite:   make(chan bool),
                closeRead:    make(chan bool),
            }
            go client.MainRoutine()
            go client.ReadRoutine()
            go client.WriteRoutine()
            return client, err
        }
    }
}

func (c *client) MainRoutine() {
    for {
        select {
        case <-c.closeMain:
            return
        case delta := <-c.setUnackChan:
            c.unackedCount += delta
        case <- c.getUnackChan:
            c.retUnackChan <- c.unackedCount
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
                c.setUnackChan <- -1
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
                c.getUnackChan <- true
                unackedCount := <-c.retUnackChan
                if unackedCount < c.params.MaxUnackedMessages {
                    _, _ = c.conn.Write(b)
                    c.setUnackChan <- 1
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
    c.closeMain <- true
    c.closeRead <- true
    c.closeWrite <- true
    return nil
}
