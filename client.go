package main

import (
	"bytes"
	"log"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

type Client struct {
	hub  *Hub
	conn *websocket.Conn
	send chan []byte
	name string
}

func newClient(hub *Hub, conn *websocket.Conn, name string) *Client {
	return &Client{
		hub:  hub,
		conn: conn,
		send: make(chan []byte, 256),
		name: name,
	}
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	// Первое сообщение — ник.
	_, nameBytes, err := c.conn.ReadMessage()
	if err != nil {
		return
	}
	c.name = sanitizeName(string(nameBytes))

	// Сообщаем всем, что мы вошли.
	c.hub.join <- c

	// Дальше — обычный цикл чата.
	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("read error: %v", err)
			}
			break
		}
		message = bytes.TrimSpace(bytes.Replace(message,
			[]byte{'\n'}, []byte{' '}, -1))

		// Оборачиваем в "Ник: сообщение".
		payload := append([]byte(c.name+": "), message...)
		c.hub.broadcast <- payload
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Дописываем всё, что уже накопилось в канале, одним фреймом.
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write([]byte{'\n'})
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// sanitizeName чистит ник от лишних пробелов и переносов,
// обрезает до 32 символов и подставляет "anon", если пусто.
func sanitizeName(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	if len(s) > 32 {
		s = s[:32]
	}
	if s == "" {
		s = "anon"
	}
	return s
}
