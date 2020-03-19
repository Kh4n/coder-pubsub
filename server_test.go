package main

import (
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestServerSimple(t *testing.T) {
	s := newServer()
	go s.Run(9090)

	var (
		curBroadcast   = make(chan string)
		done           = make(chan struct{})
		closedNormally = false
		c              *websocket.Conn
		err            error
		resp           *http.Response
	)

	go func() {
		c, resp, err = websocket.DefaultDialer.Dial("ws://localhost:9090/ws/subscribe", nil)
		if err != nil {
			t.Error("Unable to dial server:", err)
		}
		if resp.StatusCode != http.StatusSwitchingProtocols {
			t.Errorf("Expected status to be %d but got status %d", http.StatusSwitchingProtocols, resp.StatusCode)
		}
		for {
			mt, msg, err := c.ReadMessage()
			if err != nil {
				if !closedNormally {
					t.Error("Error reading message:", err)
					c.Close()
				}
				close(curBroadcast)
				return
			}
			switch mt {
			case websocket.TextMessage:
				cb := <-curBroadcast
				if string(msg) != cb {
					t.Errorf("Unexpected message broadcasted: expected %s but got %s", cb, string(msg))
				}
			case websocket.BinaryMessage:
				t.Error("Unexpected binary message from server")
			}
		}
	}()

	go func() {
		defer close(done)
		time.Sleep(200 * time.Millisecond)
		for i := 1; i <= 10; i++ {
			msg := fmt.Sprintf("Message #%d", i)
			resp, err := http.PostForm("http://localhost:9090/broadcast", url.Values{
				"msg": {msg},
			})
			if err != nil {
				t.Error("Error occurred attempting to broadcast:", msg, err)
				return
			}
			if resp.StatusCode != http.StatusOK {
				t.Errorf("Expected status to be %d but got status %d", http.StatusOK, resp.StatusCode)
				return
			}
			resp.Body.Close()
			curBroadcast <- msg
		}

		if len(s.subscribers) != 1 {
			t.Errorf("Unexpected number of subscribers")
			return
		}

		closedNormally = true
		// we should handle abrupt closures
		c.Close()
		<-curBroadcast

		time.Sleep(200 * time.Millisecond)
		if s.subscribers[0].closed == false {
			t.Error("Subscriber leaked:", s.subscribers[0])
			return
		}
	}()
	select {
	case <-done:
		return
	case <-time.After(5 * time.Second):
		t.Fatal("Test took too long")
	}
}
