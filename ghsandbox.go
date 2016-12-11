package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const (
	VERSION = "0.0.1"

	DefaultSanboxesDir = "/tmp/ghsandbox/"
)

var debug = flag.Bool("debug", false, "debug mode")

// packBytes returns byte slice with prepended 4 bytes with size data
func packBytes(b []byte) []byte {
	msgLen := uint32(len(b))
	log.Printf("%d\n", msgLen)
	buf := bytes.Buffer{}
	binary.Write(&buf, binary.LittleEndian, msgLen)
	buf.Write(b)
	return buf.Bytes()
}

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

// Message represent structure that is used for communication with GHSandbox Chrome extension
type Message struct {
	// Type tells to what you should unmarshal the Payload
	Type string `json:"type"`
	// Payload contains raw json
	Payload []byte `json:"payload"`

	rawPayload interface{}
}

// NewMessage creates new Message object ready to Pack
func NewMessage(messageType string, rawPayload interface{}) *Message {
	return &Message{Type: messageType, rawPayload: rawPayload}
}

func ReadMessage(r io.Reader) (*Message, error) {
	log.Println("READ MESSAGE\n")
	msgLen, err := readMsgLen(r)
	log.Printf("msgLen=%d\n", msgLen)
	if err != nil {
		return nil, err
	}
	body := make([]byte, msgLen)
	_, err = io.ReadFull(r, body)
	log.Printf("MSG BODY: %q", body)
	if err != nil {
		return nil, err
	}
	var msg Message
	err = json.Unmarshal(body, &msg)
	return &msg, err
}

func NewMessageStreamReader(r io.Reader) <-chan *Message {
	out := make(chan *Message)
	go func() {
		for {
			msg, err := ReadMessage(r)
			if err == nil {
				out <- msg
			} else {
				log.Printf("ReadMessage error: %s", err)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}()
	return out
}

// Pack returns marshalled message with prepended 4 bytes with size data
func (m *Message) Pack() (packedMessage []byte, err error) {
	if m.Payload == nil {
		m.Payload, err = json.Marshal(m.rawPayload)
		log.Printf("Raw marshalled : %s\n", m.Payload)
		if err != nil {
			return nil, err
		}
	}
	marshalled, err := json.Marshal(m)
	log.Printf("%s\n", marshalled)
	return packBytes(marshalled), err
}

// WriteTo writes packed message
func (m Message) WriteTo(w io.Writer) error {
	packed, err := m.Pack()
	if err != nil {
		return err
	}

	_, err = w.Write(packed)
	return err
}

type SandboxAction struct {
	Url string `json:"url"`
}

func cloneGitRepo(url, dest string) error {
	cmd := exec.Command("git", "clone", url, dest)
	return cmd.Run()
}

func openTerminal(workingDir string) error {
	switch runtime.GOOS {
	case "linux":
		return exec.Command("gnome-terminal", "--disable-factory", "--working-directory", workingDir).Run()
	case "darwin":
		panic("TODO")
	}
	return nil
}

func createRepoSandbox(url, path string, remove bool) error {
	err := cloneGitRepo(url, path)
	if err != nil {
		log.Print("Failed to clone repo")
		return err
	}
	if remove {
		defer func() {
			err = os.RemoveAll(path)
			if err != nil {
				log.Println("Failed to remove sandbox directory: %s", path)
			}
		}()
	}
	return openTerminal(path)
}

func normalizeRepoURL(u *url.URL) string {
	u.RawQuery = ""
	segs := strings.Split(u.Path, "/")
	if len(segs) == 3 {
		return u.String()
	}
	if len(segs) < 3 {
		log.Fatal("Invalid repo url")
	}
	u.Path = strings.Join(segs[:3], "/")
	return u.String()
}

func createSandboxDirectory(u *url.URL) string {
	sboxPath := filepath.Join(DefaultSanboxesDir, fmt.Sprintf("%d%s", time.Now().UnixNano(), strings.Replace(u.Path, "/", "_", -1)))
	err := os.MkdirAll(DefaultSanboxesDir, 0777)
	if err != nil {
		log.Fatalf("Failed to create sandboxes directory: %s", err)
	}
	return sboxPath
}

func handleSandboxAction(sa SandboxAction) {
	ur, err := url.Parse(sa.Url)
	if err != nil {
		log.Fatal("Failed to parse url")
	}

	err = createRepoSandbox(normalizeRepoURL(ur), createSandboxDirectory(ur), true)
	if err != nil {
		log.Print("Failed to start terminal: " + err.Error())
	}
}

func unmarshalMsg(body []byte, xmsg interface{}) interface{} {
	err := json.Unmarshal(body, xmsg)
	if err != nil {
		log.Fatal("Failed to parse message")
	}
	return xmsg
}

func main() {
	defer func() {
		if pnc := recover(); pnc != nil {
			log.Printf("PANIC: %v", pnc)
		}
	}()
	flag.Parse()
	if *debug || true {
		f, err := os.Create("ghquick.log")
		if err != nil {
			panic(err)
		}
		log.SetOutput(f)
	} else {
		log.SetOutput(ioutil.Discard)
	}

	log.Println("Starting github-sandbox")
	messageStream := NewMessageStreamReader(os.Stdin)
	msg := <-messageStream
	log.Printf("Received message: %#v", *msg)

	switch msg.Type {
	case "sandbox":
		respMsg := NewMessage("status", map[string]interface{}{"Foo": "bar"})
		respMsg.WriteTo(os.Stdout)
		handleSandboxAction(*unmarshalMsg(msg.Payload, &SandboxAction{}).(*SandboxAction))
	default:
		log.Println("NO ACTION MATCHED")
	}
}
