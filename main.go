package main

import (
	"context"
	"flag"
	"fmt"
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
		log.Println("Failed to clone repo")
		return err
	}
	if remove {
		defer func() {
			err = os.RemoveAll(path)
			if err != nil {
				log.Printf("Failed to remove sandbox directory %s: %s", path, err)
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

func openRepoSandbox(sa RepoInfo) {
	ur, err := url.Parse(sa.URL)
	if err != nil {
		log.Fatal("Failed to parse url")
	}

	err = createRepoSandbox(normalizeRepoURL(ur), createSandboxDirectory(ur), true)
	if err != nil {
		log.Print("Failed to start terminal: " + err.Error())
	}
}

func main() {
	defer func() {
		if pnc := recover(); pnc != nil {
			log.Printf("PANIC: %v", pnc)
		}
	}()
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
	msgStreamCtx, closeMsgStream := context.WithCancel(context.Background())
	msgStream := NewMessageStream(msgStreamCtx, os.Stdin)
	for msg := range msgStream {
		log.Printf("Received message: %#v", *msg)

		switch msg.Type {
		case "sandbox":
			SendMessage("status", map[string]interface{}{"Foo": "bar"})

			repoInfo := RepoInfo{}
			if err := msg.UnmarshalTo(&repoInfo); err != nil {
				log.Printf("Failed to unmarshal message %#v; ERROR: %s", msg, err)
				break
			}
			openRepoSandbox(repoInfo)

		default:
			log.Println("NO ACTION MATCHED")
			closeMsgStream()
		}
	}
	closeMsgStream()
}
