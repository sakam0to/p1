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
	conn          *lspnet.UDPConn
	readBuf       *list.List
	receiveBuf    *list.List
	rBufWriteLock chan int //the channel of readbuffer Writing flag
	rBufReadLock  chan int //the channel of readbuffer Reading flag
	writeBuf      *list.List
	connCoun      int                     // Connection counter as connID
	connSendCoun  map[int]int             // Connection : the counter of sending sequence
	connAddr      map[int]*lspnet.UDPAddr // Connection : Address
	connAckSeq    map[int]chan int        // Connection : Acknowledge of sequence number
	connSeq       map[int]int
	connSenSeq    map[int]chan int // Connection : Sending sequence number
}

// NewServer creates, initiates, and returns a new server. This function should
// NOT block. Instead, it should spawn one or more goroutines (to handle things
// like accepting incoming client connections, triggering epoch events at
// fixed intervals, synchronizing events using a for-select loop like you saw in
// project 0, etc.) and immediately return. It should return a non-nil error if
// there was an error resolving or listening on the specified port number.
func NewServer(port int, params *Params) (Server, error) {
	server := server{readBuf: list.New(), receiveBuf: list.New(), rBufWriteLock: make(chan int), rBufReadLock: make(chan int), writeBuf: list.New(), connSendCoun: make(map[int]int), connAddr: make(map[int]*lspnet.UDPAddr), connAckSeq: make(map[int]chan int), connSeq: make(map[int]int), connSenSeq: make(map[int]chan int)}
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

	go func() { server.rBufWriteLock <- 1 }()
	return &server, nil
}

func (s *server) Read() (int, []byte, error) {

	<-s.rBufReadLock
	element := s.readBuf.Front()
	<-s.rBufWriteLock
	message := element.Value.(*Message)
	s.readBuf.Remove(element)

	go func() { s.rBufWriteLock <- 1 }()
	return message.ConnID, message.Payload, nil
}

func (s *server) Write(connId int, payload []byte) error {
	seqNum := s.connSendCoun[connId]
	checksum := uint16(ByteArray2Checksum(payload))
	message := NewData(connId, seqNum, len(payload), payload, checksum)

	go s.WriteRoutine(message)
	s.connSendCoun[connId] = seqNum + 1
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

		var message *Message
		err = json.Unmarshal(buffer[0:n], &message)
		if err != nil {
			fmt.Println("error:", err)
		}
		if message.Type == MsgConnect {
			s.connCoun++
			s.connSendCoun[s.connCoun] = 1
			s.connAddr[s.connCoun] = addr
			s.connAckSeq[s.connCoun] = make(chan int)
			s.connSenSeq[s.connCoun] = make(chan int)
			s.connSeq[s.connCoun] = 0
			go s.Ack(s.connCoun, message.SeqNum)
		} else {
			go s.ReadRoutine(message)
		}
	}
}

func (s *server) ReadRoutine(message *Message) {
	if message.Type == MsgAck {
		if message.SeqNum > 0 { //Not HeartBeat
			s.connSenSeq[message.ConnID] <- message.SeqNum
		}
	} else {

		<-s.rBufWriteLock
		seqNum := <-s.connAckSeq[message.ConnID]
		seqNum = s.connSeq[message.ConnID]

		if seqNum+1 == message.SeqNum {
			s.readBuf.PushBack(message)

			seqNum++
			if s.receiveBuf.Len() > 0 {
				for {
					flag := false
					var toRemove *list.Element
					for e := s.receiveBuf.Front(); e != nil; e = e.Next() {
						eMessage := e.Value.(*Message)
						if eMessage.ConnID == message.ConnID && eMessage.SeqNum == seqNum+1 {
							toRemove = e
							flag = true
							s.readBuf.PushBack(eMessage)

							seqNum++
							break
						}
					}
					if flag == false {
						break
					} else {
						s.receiveBuf.Remove(toRemove)
					}
				}
			}

			s.connSeq[message.ConnID] = seqNum
			go func() { s.rBufWriteLock <- 1 }()
			go s.Ack(message.ConnID, seqNum)
			go func() {
				s.rBufReadLock <- 1
			}()

			for i := message.SeqNum + 1; i <= seqNum; i++ {
				<-s.connAckSeq[message.ConnID]
				go s.Ack(message.ConnID, seqNum)
				go func() {
					s.rBufReadLock <- 1
				}()
			}
		} else {
			s.receiveBuf.PushBack(message)

			go func() { s.connAckSeq[message.ConnID] <- seqNum }()
			go func() { s.rBufWriteLock <- 1 }()

		}
	}
}

func (s *server) SendRoutine(data []byte, addr *lspnet.UDPAddr) {
	_, err := s.conn.WriteToUDP(data, addr)
	if err != nil {
		return
	}
}

func (s *server) WriteRoutine(message *Message) {
	data, err := json.Marshal(message)
	if err != nil {
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
				break
			}
		}
	}
}

func (s *server) Ack(connID, seqNum int) {
	ack := NewAck(connID, seqNum)
	s.WriteRoutine(ack)
	s.connAckSeq[connID] <- seqNum
}
