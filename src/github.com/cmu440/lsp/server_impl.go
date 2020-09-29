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
	conn       *lspnet.UDPConn
	readBuf    *list.List
	writeBuf   *list.List
	connCoun   int                     //Connection counter as connID
	connAddr   map[int]*lspnet.UDPAddr // Connection : Address
	connRecSeq map[int]chan int        // Connection : Receiving sequence number
	connSenSeq map[int]chan int        // Connection : Sending sequence number
}

// NewServer creates, initiates, and returns a new server. This function should
// NOT block. Instead, it should spawn one or more goroutines (to handle things
// like accepting incoming client connections, triggering epoch events at
// fixed intervals, synchronizing events using a for-select loop like you saw in
// project 0, etc.) and immediately return. It should return a non-nil error if
// there was an error resolving or listening on the specified port number.
func NewServer(port int, params *Params) (Server, error) {
	server := server{readBuf: list.New(), writeBuf: list.New(), connAddr: make(map[int]*lspnet.UDPAddr), connRecSeq: make(map[int]chan int), connSenSeq: make(map[int]chan int)}
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
	// TODO: remove this line when you are ready to begin implementing this method.
	//select {} // Blocks indefinitely.

	if s.readBuf.Len() > 0 {
		message := s.readBuf.Front().Value.(*Message)
		fmt.Println("10 ")
		return message.Size, message.Payload, nil
	} else {
		return -1, nil, errors.New("not yet implemented")
	}
}

func (s *server) Write(connId int, payload []byte) error {
	fmt.Println("9 ")
	seqNum := <-s.connSenSeq[connId]
	checksum := uint16(0)
	message := NewData(connId, seqNum, len(payload), payload, checksum)

	go s.WriteRoutine(message)
	return nil
	//return errors.New("not yet implemented")
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
		fmt.Println("waiting...")
		n, addr, err := s.conn.ReadFromUDP(buffer)

		if err != nil {
			break
		}

		if addr == nil {
			break
		}

		fmt.Println("received data:", string(buffer[0:n]))
		var message *Message
		err = json.Unmarshal(buffer[0:n], &message)
		if err != nil {
			fmt.Println("error:", err)
		}
		if message.Type == MsgConnect {
			s.connCoun++
			s.connAddr[s.connCoun] = addr
			s.connRecSeq[s.connCoun] = make(chan int)
			s.connSenSeq[s.connCoun] = make(chan int)
			go s.Ack(s.connCoun, message.SeqNum)
		} else if message.Type == MsgAck {
			fmt.Println("MsgAck")
			if message.SeqNum > 0 { //Not HeartBeat
				s.connSenSeq[message.ConnID] <- message.SeqNum
			}
		} else {
			fmt.Println("MsgData")
			go s.ReadRoutine(message)
		}
	}
}

func (s *server) ReadRoutine(message *Message) {
	//message := s.revBuf.Front().Value.(Message)
	//fmt.Println("-> ", message.String())
	seqNum := <-s.connRecSeq[message.ConnID]

	fmt.Println("seqNum:", seqNum)
	if seqNum == message.SeqNum {
		s.readBuf.PushBack(message)
		go s.Ack(message.ConnID, message.SeqNum)
	}
}

func (s *server) SendRoutine(data []byte, addr *lspnet.UDPAddr) {
	fmt.Printf("Send data: %s\n", string(data))
	_, err := s.conn.WriteToUDP(data, addr)
	//fmt.Println("5 ")
	if err != nil {
		//fmt.Println("6 ")
		fmt.Println(err)
		return
	}
	//fmt.Println("7 ")
}

func (s *server) WriteRoutine(message *Message) {
	//fmt.Println("3")
	data, err := json.Marshal(message)
	if err != nil {
		fmt.Println(err)
		return
	}
	//fmt.Println("1 ")
	addr := s.connAddr[message.ConnID]
	//fmt.Println("2 ")
	s.SendRoutine(data, addr)
	if message.Type == MsgData {
		for {
			ack := <-s.connSenSeq[message.ConnID]
			if ack != message.SeqNum {
				s.SendRoutine(data, addr)
			} else {
				s.connSenSeq[message.ConnID] <- ack + 1
				break
			}
		}
	}
}

func (s *server) Ack(connID, seqNum int) {
	ack := NewAck(connID, seqNum)
	fmt.Println("ACK ->", ack.String())
	s.WriteRoutine(ack)
	//fmt.Println(connID)
	s.connRecSeq[connID] <- seqNum + 1
	fmt.Println("ACK Done")
}
