package main

import "log"

type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
	join       chan *Client
}

func newHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan []byte),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		join:       make(chan *Client),
	}
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			log.Printf("client connected, total: %d", len(h.clients))

		case client := <-h.join:
			// Самому себе не сообщаем о своём входе.
			h.systemMessageExcept(client.name+" вошёл в чат", client)

		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				h.systemMessage(client.name + " покинул чат")
				log.Printf("client disconnected, total: %d", len(h.clients))
			}

		case message := <-h.broadcast:
			for client := range h.clients {
				select {
				case client.send <- message:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
		}
	}
}

// systemMessage рассылает всем служебное сообщение (вход/выход).
func (h *Hub) systemMessage(text string) {
	h.systemMessageExcept(text, nil)
}

// systemMessageExcept рассылает всем, кроме указанного клиента.
// Если except == nil — рассылает всем.
func (h *Hub) systemMessageExcept(text string, except *Client) {
	msg := []byte("*** " + text + " ***")
	for client := range h.clients {
		if client == except {
			continue
		}
		select {
		case client.send <- msg:
		default:
			close(client.send)
			delete(h.clients, client)
		}
	}
}
