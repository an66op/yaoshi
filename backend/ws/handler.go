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
	userID, ok := wsTickets.consume(ticket)
	if ticket == "" || !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "实时连接凭据已失效"})
		return
	}

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}

	client := &client{userID: userID, send: make(chan []byte, 16)}
	defaultHub.register(client)

	go writePump(conn, client)
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

func writePump(conn *websocket.Conn, client *client) {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		_ = conn.Close()
	}()
	for {
		select {
		case payload, ok := <-client.send:
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				_ = conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := conn.WriteMessage(websocket.TextMessage, payload); err != nil {
				return
			}
		case <-ticker.C:
			_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
