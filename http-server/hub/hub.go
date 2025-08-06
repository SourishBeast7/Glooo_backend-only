package hub

import (
	"log"
	"sync"

	"github.com/SourishBeast7/Glooo/db"
	"github.com/SourishBeast7/Glooo/db/models"
	"github.com/gorilla/websocket"
)

type Client struct {
	ID   uint
	Conn *websocket.Conn
	Hub  *Hub
}

type Hub struct {
	Clients    map[uint]*Client
	Register   chan *Client
	Unregister chan *Client
	Broadcast  chan []byte
	Mutex      sync.RWMutex
	db         *db.Storage
}

func (h *Hub) NewClient(id uint, conn *websocket.Conn) *Client {
	return &Client{
		ID:   id,
		Conn: conn,
		Hub:  h,
	}
}

func NewHub() *Hub {
	return &Hub{
		Clients:    make(map[uint]*Client),
		Register:   make(chan *Client),
		Unregister: make(chan *Client),
		Broadcast:  make(chan []byte),
	}
}

func (h *Hub) Run(db *db.Storage) {
	h.db = db
	for {
		select {
		case client := <-h.Register:
			h.Mutex.Lock()
			h.Clients[client.ID] = client
			h.Mutex.Unlock()
		case client := <-h.Unregister:
			h.Mutex.Lock()
			if _, ok := h.Clients[client.ID]; ok {
				h.Mutex.Lock()
				delete(h.Clients, client.ID)
				h.Mutex.Unlock()
			}
			h.Mutex.Unlock()
		}
	}
}

func (h *Hub) Readloop(c *Client) {
	defer func() {
		h.Mutex.Lock()
		delete(h.Clients, c.ID)
		h.Mutex.Unlock()
		defer c.Conn.Close()
	}()
	for {
		message := new(models.Message)
		message.SenderID = c.ID
		err := c.Conn.ReadJSON(message)
		if err != nil {
			log.Println(err)
			continue
		}
		h.db.AddMessages(message)
		go h.WriteMessage(message)
	}
}

func (h *Hub) WriteMessage(message *models.Message) {
	h.Mutex.Lock()
	if c, ok := h.Clients[message.ReceiverID]; ok {
		if err := c.Conn.WriteJSON(message); err != nil {
			log.Printf("%v", err)
			return
		}
	}
	h.Mutex.Unlock()
}
