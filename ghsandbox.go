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

type ActionName struct {
	Action string `json:"action"`
}

func readMsgLen(r io.Reader) int {
	var msgLen uint32
	tmpb := make([]byte, 4)
	n, err := r.Read(tmpb)
	buf := bytes.NewBuffer(tmpb)
	log.Printf("Read %d bytes of len", n)
	if n == 0 {
		return 0
	}
	if err != nil {
		log.Fatal("Failed to read message")
	}
	binary.Read(buf, binary.LittleEndian, &msgLen)
	if msgLen < 0 {
		log.Fatalf("Invalid msg len: %d", msgLen)
	}
	return int(msgLen)
}

func receiveMsg() (action string, body []byte) {
	log.Println("Reading msg")
	msgLen := readMsgLen(os.Stdin)
	log.Printf("Message len: %d", msgLen)
	body = make([]byte, msgLen)
	n, err := os.Stdin.Read(body)
	if err != nil {
		log.Fatal("Failed to read stdin")
	}
	if n != msgLen {
		log.Fatal("Invalid message (size mismatch: %d vs %d)", msgLen, n)
	}

	act := ActionName{}
	err = json.Unmarshal(body, &act)
	if err != nil {
		log.Fatal("Failed to parse action")
	}

	action = act.Action

	return
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
	flag.Parse()
	if *debug {
		f, err := os.Create("ghquick.log")
		if err != nil {
			panic(err)
		}
		log.SetOutput(f)
	} else {
		log.SetOutput(ioutil.Discard)
	}

	log.Println("Starting github-sandbox")
	action, msg := receiveMsg()
	log.Printf("Received message: %s", msg)

	switch action {
	case "sandbox":
		handleSandboxAction(*unmarshalMsg(msg, &SandboxAction{}).(*SandboxAction))
	default:
		log.Println("NO ACTION MATCHED")
	}
}
