package main

import (
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// WSMessage WebSocket 消息类型
type WSMessage struct {
	Type      string      `json:"type"` // message, history, stats, cats
	SessionID string      `json:"sessionId,omitempty"`
	Data      interface{} `json:"data"`
	Timestamp time.Time   `json:"timestamp"`
}

// WSClient WebSocket 客户端
type WSClient struct {
	conn      *websocket.Conn
	send      chan WSMessage
	sessionID string
	mu        sync.Mutex
}

// WSHub WebSocket 连接管理中心
type WSHub struct {
	clients    map[string]map[*WSClient]bool // sessionID -> clients
	broadcast  chan WSMessage
	register   chan *WSClient
	unregister chan *WSClient
	mu         sync.RWMutex
}

// NewWSHub 创建 WebSocket Hub
func NewWSHub() *WSHub {
	return &WSHub{
		clients:    make(map[string]map[*WSClient]bool),
		broadcast:  make(chan WSMessage, 256),
		register:   make(chan *WSClient),
		unregister: make(chan *WSClient),
	}
}

// Run 启动 Hub
func (h *WSHub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			if h.clients[client.sessionID] == nil {
				h.clients[client.sessionID] = make(map[*WSClient]bool)
			}
			h.clients[client.sessionID][client] = true
			h.mu.Unlock()
			LogInfo("[WS] 客户端已连接 - SessionID: %s", client.sessionID)

		case client := <-h.unregister:
			h.mu.Lock()
			if clients, ok := h.clients[client.sessionID]; ok {
				if _, exists := clients[client]; exists {
					delete(clients, client)
					close(client.send)
					if len(clients) == 0 {
						delete(h.clients, client.sessionID)
					}
				}
			}
			h.mu.Unlock()
			LogInfo("[WS] 客户端已断开 - SessionID: %s", client.sessionID)

		case message := <-h.broadcast:
			h.mu.RLock()
			clients := h.clients[message.SessionID]
			h.mu.RUnlock()

			for client := range clients {
				select {
				case client.send <- message:
				default:
					// 发送失败，关闭客户端
					h.mu.Lock()
					delete(h.clients[message.SessionID], client)
					close(client.send)
					h.mu.Unlock()
				}
			}
		}
	}
}

// BroadcastToSession 向指定会话的所有客户端广播消息
func (h *WSHub) BroadcastToSession(sessionID string, msgType string, data interface{}) {
	message := WSMessage{
		Type:      msgType,
		SessionID: sessionID,
		Data:      data,
		Timestamp: time.Now(),
	}
	h.broadcast <- message
}

// writePump 处理向客户端写入消息
func (c *WSClient) writePump() {
	ticker := time.NewTicker(54 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}

			data, err := json.Marshal(message)
			if err != nil {
				LogError("[WS] 序列化消息失败: %v", err)
				continue
			}

			w.Write(data)

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// readPump 处理从客户端读取消息
func (c *WSClient) readPump(hub *WSHub) {
	defer func() {
		hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				LogError("[WS] 读取消息错误: %v", err)
			}
			break
		}
	}
}
