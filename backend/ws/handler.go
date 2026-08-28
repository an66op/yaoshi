package ws

import (
	"backend/config"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return config.IsOriginAllowed(r.Header.Get("Origin")) },
}

// HandleConnect upgrades HTTP after a one-time ticket check. JWTs are never
// accepted in the query string or WebSocket headers; browsers must first call
// the authenticated ticket endpoint.
func HandleConnect(c *gin.Context) {
	ticket := strings.TrimSpace(c.Query("ticket"))
	identity, ok, consumeErr := wsTickets.consume(ticket)
	if consumeErr != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"message": "实时连接服务暂不可用，请稍后重试"})
		return
	}
	if ticket == "" || !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "实时连接凭据已失效"})
		return
	}
	if !defaultHub.hasSessionValidator() || !defaultHub.validate(identity) {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "登录或房间状态已变化，请重新连接"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	client := &client{identity: identity, send: make(chan []byte, 16), done: make(chan struct{})}
	defaultHub.register(client)

	go writePump(conn, client, defaultHub)
	readPump(conn, client)
}

func readPump(conn *websocket.Conn, client *client) {
	defer func() {
		defaultHub.unregister(client)
		_ = conn.Close()
	}()
	_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		_ = conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			break
		}
	}
}

func writePump(conn *websocket.Conn, client *client, hub *Hub) {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		_ = conn.Close()
	}()
	for {
		select {
		case <-client.done:
			_ = conn.WriteControl(websocket.CloseMessage,
				websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "登录状态已变化"), time.Now().Add(time.Second))
			return
		case payload := <-client.send:
			open, err := client.writeIfOpen(func() error {
				_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				return conn.WriteMessage(websocket.TextMessage, payload)
			})
			if !open || err != nil {
				return
			}
		case <-ticker.C:
			if hub == nil || !hub.validate(client.identity) {
				if hub != nil {
					hub.unregister(client)
				}
				_ = conn.WriteControl(websocket.CloseMessage,
					websocket.FormatCloseMessage(websocket.ClosePolicyViolation, "登录状态已变化"), time.Now().Add(time.Second))
				return
			}
			open, err := client.writeIfOpen(func() error {
				_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				return conn.WriteMessage(websocket.PingMessage, nil)
			})
			if !open || err != nil {
				return
			}
		}
	}
}
