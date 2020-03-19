package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func broadcast(t *testing.T, broadcastURL string, msg string) {
	resp, err := http.PostForm(broadcastURL, url.Values{
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
}

func TestServerSimple(t *testing.T) {
	var (
		s             = newServer()
		broadcastPath = "/broadcast"
		subscribePath = "/ws/subscribe"

		curBroadcast     = make(chan string)
		done             = make(chan struct{})
		connectionClosed = false

		c    *websocket.Conn
		err  error
		resp *http.Response
	)

	ts := httptest.NewServer(s.createMux())
	tsURL, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal("Test server generated bad url:", err)
	}
	base := fmt.Sprintf("%s:%s", tsURL.Hostname(), tsURL.Port())
	defer ts.Close()

	go func() {
		wsURL := fmt.Sprintf("ws://" + base + subscribePath)
		c, resp, err = websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			t.Error("Unable to dial server:", err)
			return
		}
		if resp.StatusCode != http.StatusSwitchingProtocols {
			t.Errorf("Expected status to be %d but got status %d", http.StatusSwitchingProtocols, resp.StatusCode)
			return
		}
		for {
			mt, msg, err := c.ReadMessage()
			if err != nil {
				if !connectionClosed {
					t.Error("Error reading message:", err)
					c.Close()
				}
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
		// wait for server to open
		for {
			resp, err := http.Get(ts.URL)
			if err != nil {
				continue
			}
			if resp.StatusCode != http.StatusOK {
				continue
			}
			break
		}
		for i := 1; i <= 10; i++ {
			msg := fmt.Sprintf("Message #%d", i)
			broadcast(t, ts.URL+broadcastPath, msg)
			curBroadcast <- msg
		}

		if len(s.subscribers) != 1 {
			t.Errorf("Unexpected number of subscribers, expected %d but have %d", 1, len(s.subscribers))
			return
		}

		connectionClosed = true
		// we should handle abrupt closures
		c.Close()

		broadcast(t, ts.URL+broadcastPath, "last")
		if len(s.subscribers) != 0 {
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
