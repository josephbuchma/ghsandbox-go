package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"
)

// StdoutMutex is the only mutex you should use to ensure safe STDOUT access
var StdoutMutex = &sync.Mutex{}

// packBytes returns given byte slice with prepended 4 bytes with its size data.
// Returned bytes are ready to be sent to chrome.
func packBytes(b []byte) []byte {
	msgLen := uint32(len(b))
	buf := bytes.Buffer{}
	binary.Write(&buf, binary.LittleEndian, msgLen)
	buf.Write(b)
	return buf.Bytes()
}

// readMsgLen reads length of incoming message
func readMsgLen(r io.Reader) (int, error) {
	var msgLen uint32
	rawMsgLenBytes := make([]byte, 4)

	_, err := io.ReadFull(r, rawMsgLenBytes)
	if err != nil {
		return 0, err
	}
	buf := bytes.NewBuffer(rawMsgLenBytes)
	binary.Read(buf, binary.LittleEndian, &msgLen)
	if msgLen < 0 {
		return 0, fmt.Errorf("Invalid message length: %d", msgLen)
	}
	return int(msgLen), nil
}

// Message represent structure that is used for communication with Chrome extension
type Message struct {
	// Type tells to what you should unmarshal the Payload. Must be unique.
	Type string `json:"type"`
	// Payload contains raw base64-encoded json.
	Payload []byte `json:"payload"`

	// payload is an object that will be marshalled/unmarshalled to/from Payload.
	payload interface{}
}

// NewMessage creates new Message object ready to Pack or
func NewMessage(messageType string, payload interface{}) *Message {
	return &Message{Type: messageType, payload: payload}
}

func ReadMessage(r io.Reader) (*Message, error) {
	msgLen, err := readMsgLen(r)
	if err != nil {
		return nil, err
	}
	body := make([]byte, msgLen)
	_, err = io.ReadFull(r, body)
	if err != nil {
		return nil, err
	}
	var msg Message
	err = json.Unmarshal(body, &msg)
	return &msg, err
}

// SendMessage creates new message and writes it to STDOUT. It's safe to use it concurrently.
func SendMessage(messageType string, payload interface{}) error {
	StdoutMutex.Lock()
	_, err := NewMessage(messageType, payload).WriteTo(os.Stdout)
	StdoutMutex.Unlock()
	return err
}

func NewMessageStream(ctx context.Context, r io.Reader) <-chan *Message {
	out := make(chan *Message)
	go func() {
		for {
			select {
			case <-ctx.Done():
				close(out)
				return
			case <-time.After(time.Millisecond):
				msg, err := ReadMessage(r)
				if err == nil {
					out <- msg
				} else {
					log.Printf("ReadMessage error: %s", err)
				}
			}
		}
	}()
	return out
}

// Pack returns marshalled message with prepended 4 bytes with size data
func (m *Message) Pack() (packedMessage []byte, err error) {
	if m.Payload == nil {
		m.Payload, err = json.Marshal(m.payload)
		if err != nil {
			return nil, err
		}
	}
	marshalled, err := json.Marshal(m)
	return packBytes(marshalled), err
}

// WriteTo writes packed message to given writer.
func (m Message) WriteTo(w io.Writer) (int64, error) {
	packed, err := m.Pack()
	if err != nil {
		return 0, err
	}

	n, err := w.Write(packed)
	return int64(n), err
}

// UnmarshalTo
func (m Message) UnmarshalTo(objPointer interface{}) error {
	return json.Unmarshal(m.Payload, objPointer)
}
