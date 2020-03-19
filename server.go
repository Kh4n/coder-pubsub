// Simple pubsub system. Does not support topics
package main

import (
	"fmt"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

type subscriber struct {
	conn   *websocket.Conn
	closed bool
	msgs   chan string
}

// Server is a Pub/Sub server using websockets
type Server struct {
	upgrader *websocket.Upgrader

	subscribersMu sync.Mutex
	subscribers   []*subscriber
}

func newServer() *Server {
	upgrader := websocket.Upgrader{}
	upgrader.CheckOrigin = func(r *http.Request) bool { return true }
	return &Server{
		upgrader:    &upgrader,
		subscribers: make([]*subscriber, 0),
	}
}

func (s *Server) addSubscriber(sub *subscriber) error {
	s.subscribersMu.Lock()
	defer s.subscribersMu.Unlock()
	s.subscribers = append(s.subscribers, sub)
	log.Println("Added subscriber:", sub.conn.RemoteAddr())
	return nil
}

func (s *Server) handleSubscribeSocket(w http.ResponseWriter, r *http.Request) {
	c, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Error occurred while upgrading:", err)
		return
	}
	msgs := make(chan string)
	sub := &subscriber{conn: c, msgs: msgs}
	err = s.addSubscriber(sub)
	if err != nil {
		log.Println("Unable to add subscriber:", err)
		return
	}

	defer c.Close()
	go func() {
		for {
			// important to read messages so we know when the connection is closed
			// also need to remove the subscriber, as the close handler is not always called
			_, _, err := c.ReadMessage()
			if err != nil {
				log.Println("Error reading message:", err)
				s.subscribersMu.Lock()
				sub.closed = true
				close(sub.msgs)
				s.subscribersMu.Unlock()
				log.Println("Removed subscriber:", c.RemoteAddr())
				return
			}
		}
	}()
	for {
		msg, ok := <-msgs
		if !ok {
			return
		}
		log.Println("Broadcast message received:", msg)
		err := c.WriteMessage(websocket.TextMessage, []byte(msg))
		if err != nil {
			log.Println("Error writing message:", err)
			return
		}
	}
}

func (s *Server) handleBroadcast(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		log.Println("Unable to parse form:", err)
		return
	}
	msg := r.PostForm.Get("msg")
	if msg == "" {
		log.Println("Empty message received")
		return
	}
	s.subscribersMu.Lock()
	defer s.subscribersMu.Unlock()
	// clean up as we go, so we don't need to do any extra allocations
	h := 0
	for _, sub := range s.subscribers {
		if !sub.closed {
			s.subscribers[h] = sub
			sub.msgs <- msg
			h++
		}
	}
	s.subscribers = s.subscribers[:h]
}

// Run : Sets up handlers and runs the server on the given port
func (s *Server) Run(port uint) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws/subscribe", s.handleSubscribeSocket)
	mux.HandleFunc("/broadcast", s.handleBroadcast)
	mux.Handle("/", http.FileServer(http.Dir(".")))
	portStr := fmt.Sprintf(":%d", port)
	log.Fatal(http.ListenAndServe(portStr, mux))
}

func main() {
	s := newServer()
	s.Run(8080)
}
