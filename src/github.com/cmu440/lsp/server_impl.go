// Contains the implementation of a LSP server.

package lsp

import (
	"container/list"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/cmu440/lspnet"
)

type server struct {
	conn         *lspnet.UDPConn
	readBuf      *list.List
	readFlag     chan int //the channel of read buffer flag
	writeBuf     *list.List
	connCoun     int                     // Connection counter as connID
	connSendCoun map[int]int             // Connection : the counter of sending sequence
	connAddr     map[int]*lspnet.UDPAddr // Connection : Address
	connAckSeq   map[int]chan int        // Connection : Acknowledge of sequence number
	connSenSeq   map[int]chan int        // Connection : Sending sequence number
}

// NewServer creates, initiates, and returns a new server. This function should
// NOT block. Instead, it should spawn one or more goroutines (to handle things
// like accepting incoming client connections, triggering epoch events at
// fixed intervals, synchronizing events using a for-select loop like you saw in
// project 0, etc.) and immediately return. It should return a non-nil error if
// there was an error resolving or listening on the specified port number.
func NewServer(port int, params *Params) (Server, error) {
	server := server{readBuf: list.New(), readFlag: make(chan int), writeBuf: list.New(), connSendCoun: make(map[int]int), connAddr: make(map[int]*lspnet.UDPAddr), connAckSeq: make(map[int]chan int), connSenSeq: make(map[int]chan int)}
	laddr, err := lspnet.ResolveUDPAddr("udp", "127.0.0.1:"+fmt.Sprint(port))

	if err != nil {
		return nil, err
	}

	ln, err := lspnet.ListenUDP("udp", laddr)

	if err != nil {
		return nil, err
	}

	server.conn = ln
	go server.ReceiveRoutine()

	return &server, nil
}

func (s *server) Read() (int, []byte, error) {
	//fmt.Println("Read ")
	select {
	case <-s.readFlag:
		message := s.readBuf.Remove(s.readBuf.Front()).(*Message)

		//fmt.Println("Read ", message.String())
		return message.ConnID, message.Payload, nil
	}
}

func (s *server) Write(connId int, payload []byte) error {
	//fmt.Println("Write conn:", connId)
	seqNum := s.connSendCoun[connId]
	checksum := uint16(ByteArray2Checksum(payload))
	message := NewData(connId, seqNum, len(payload), payload, checksum)

	go s.WriteRoutine(message)
	s.connSendCoun[connId] = seqNum + 1
	//fmt.Println("Write Done")
	return nil
}

func (s *server) CloseConn(connId int) error {
	return errors.New("not yet implemented")
}

func (s *server) Close() error {
	return errors.New("not yet implemented")
}

func (s *server) ReceiveRoutine() {
	buffer := make([]byte, 1024)
	for {
		n, addr, err := s.conn.ReadFromUDP(buffer)

		if err != nil {
			break
		}

		if addr == nil {
			break
		}

		//fmt.Printf("received data: %s from %s\n", string(buffer[0:n]), addr.String())
		var message *Message
		err = json.Unmarshal(buffer[0:n], &message)
		if err != nil {
			fmt.Println("error:", err)
		}
		if message.Type == MsgConnect {
			//fmt.Printf("received data: %s from %s\n", string(buffer[0:n]), addr.String())
			s.connCoun++
			s.connSendCoun[s.connCoun] = 1
			s.connAddr[s.connCoun] = addr
			s.connAckSeq[s.connCoun] = make(chan int)
			s.connSenSeq[s.connCoun] = make(chan int)
			go s.Ack(s.connCoun, message.SeqNum)
		} else {
			//fmt.Println("MsgData")
			go s.ReadRoutine(message)
		}
	}
}

func (s *server) ReadRoutine(message *Message) {
	if message.Type == MsgAck {
		////fmt.Println("MsgAck")
		if message.SeqNum > 0 { //Not HeartBeat
			//fmt.Printf("Received data: %s\n", message.String())
			s.connSenSeq[message.ConnID] <- message.SeqNum
		}
	} else {

		//fmt.Printf("Received data: %s\n", message.String())
		seqNum := <-s.connAckSeq[message.ConnID]
		//fmt.Println("seqNum", seqNum)
		if seqNum+1 == message.SeqNum {
			s.readBuf.PushBack(message)
			go s.Ack(message.ConnID, message.SeqNum)
			s.readFlag <- 1
		}
	}
}

func (s *server) SendRoutine(data []byte, addr *lspnet.UDPAddr) {
	//fmt.Printf("Send data: %s to %s\n", string(data), addr.String())
	_, err := s.conn.WriteToUDP(data, addr)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
}

func (s *server) WriteRoutine(message *Message) {
	data, err := json.Marshal(message)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	addr := s.connAddr[message.ConnID]
	s.SendRoutine(data, addr)
	if message.Type == MsgData {
		for {
			ack := <-s.connSenSeq[message.ConnID]
			if ack != message.SeqNum {
				s.SendRoutine(data, addr)
			} else {
				//s.connSenSeq[message.ConnID] <- ack + 1
				break
			}
		}
	}
}

func (s *server) Ack(connID, seqNum int) {
	ack := NewAck(connID, seqNum)
	//fmt.Println("ACK ->", ack.String())
	s.WriteRoutine(ack)
	s.connAckSeq[connID] <- seqNum
}
