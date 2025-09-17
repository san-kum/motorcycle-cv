package handlers

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/san-kum/motorcycle-cv/server/models"
	"github.com/san-kum/motorcycle-cv/server/processor"
	"go.uber.org/zap"
)

type WebSocketHandler struct {
	processor *processor.FrameProcessor
	logger    *zap.Logger
	upgrader  websocket.Upgrader
}

type ClientMessage struct {
	Type      string `json:"type"`
	Data      string `json:"data"`
	Timestamp int64  `json:"timestamp"`
}

type ServerMessage struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

func NewWebSocketHandler(processor *processor.FrameProcessor, logger *zap.Logger) *WebSocketHandler {
	return &WebSocketHandler{
		processor: processor,
		logger:    logger,
		upgrader: websocket.Upgrader{
			ReadBufferSize:  4096,
			WriteBufferSize: 4096,
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}
}

func (h *WebSocketHandler) HandleWebSocket(c *gin.Context) {
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		h.logger.Error("failed to upgrade websocket connection", zap.Error(err))
		return
	}
	defer conn.Close()

	clientIP := c.ClientIP()
	h.logger.Info("WebSocket client connected", zap.String("client_ip", clientIP))

	conn.SetReadLimit(10 * 1024 * 1024)
	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(appData string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	ticker := time.NewTicker(54 * time.Second)
	defer ticker.Stop()

	done := make(chan struct{})
	writeMu := &sync.Mutex{}

	go h.pingRoutine(conn, ticker, done, writeMu)

	for {
		select {
		case <-done:
			return
		default:
			var message ClientMessage
			err := conn.ReadJSON(&message)
			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					h.logger.Error("Websocket error: ", zap.Error(err))
				}
				close(done)
				return
			}
			h.handleMessage(conn, &message, writeMu)
		}
	}

}

func (h *WebSocketHandler) handleMessage(conn *websocket.Conn, message *ClientMessage, writeMu *sync.Mutex) {
	switch message.Type {
	case "frame":
		h.processVideoFrame(conn, message, writeMu)
	case "ping":
		h.sendMessage(conn, writeMu, "pong", map[string]any{"timestamp": time.Now().Unix()})
	case "config":
		h.handleConfigUpdate(conn, message, writeMu)
	default:
		h.logger.Warn("Unknown message type received", zap.String("type", message.Type))
		h.sendError(conn, writeMu, "Unknown message type: "+message.Type)
	}
}

func (h *WebSocketHandler) processVideoFrame(conn *websocket.Conn, message *ClientMessage, writeMu *sync.Mutex) {
	imageData, err := h.extractImageData(message.Data)

	if err != nil {
		h.logger.Error("Failed to extract image data", zap.Error(err))
		h.sendError(conn, writeMu, "invalid image data format")
		return
	}

	frameRequest := &models.FrameRequest{
		ImageData: imageData,
		Timestamp: message.Timestamp,
		ClientID:  h.getClientID(conn),
	}
	go func() {
		result, err := h.processor.ProcessFrame(frameRequest)
		if err != nil {
			h.logger.Error("Frame processing failed", zap.Error(err))
			h.sendError(conn, writeMu, "Frame processing failed")
			return
		}

		h.sendMessage(conn, writeMu, "analysis", result)

		if len(result.Feedback) > 0 {
			for _, feedback := range result.Feedback {
				h.sendMessage(conn, writeMu, "feedback", map[string]any{
					"message": feedback.Message,
					"type":    feedback.Type,
					"score":   feedback.Score,
				})
			}
		}
	}()
}

func (h *WebSocketHandler) extractImageData(dataURL string) ([]byte, error) {
	if !strings.Contains(dataURL, ",") {
		return nil, websocket.ErrBadHandshake
	}

	parts := strings.Split(dataURL, ",")
	if len(parts) != 2 {
		return nil, websocket.ErrBadHandshake
	}

	imageData, err := base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, err
	}
	return imageData, err
}

func (h *WebSocketHandler) handleConfigUpdate(conn *websocket.Conn, message *ClientMessage, writeMu *sync.Mutex) {
	var config map[string]any
	if err := json.Unmarshal([]byte(message.Data), &config); err != nil {
		h.logger.Error("Invalid config format", zap.Error(err))
		h.sendError(conn, writeMu, "Invalid configuration format")
		return
	}

	h.processor.UpdateConfig(config)
	h.sendMessage(conn, writeMu, "config_updated", map[string]interface{}{
		"status": "success",
		"config": config,
	})
}

func (h *WebSocketHandler) sendMessage(conn *websocket.Conn, writeMu *sync.Mutex, messageType string, data interface{}) {
	message := ServerMessage{
		Type: messageType,
		Data: data,
	}

	writeMu.Lock()
	defer writeMu.Unlock()

	if err := conn.WriteJSON(message); err != nil {
		h.logger.Error("Failed to send WebSocket message", zap.Error(err))
	}
}

func (h *WebSocketHandler) sendError(conn *websocket.Conn, writeMu *sync.Mutex, errorMsg string) {
	h.sendMessage(conn, writeMu, "error", map[string]interface{}{
		"message":   errorMsg,
		"timestamp": time.Now().Unix(),
	})
}

func (h *WebSocketHandler) pingRoutine(conn *websocket.Conn, ticker *time.Ticker, done chan struct{}, writeMu *sync.Mutex) {
	for {
		select {
		case <-ticker.C:
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			writeMu.Lock()
			err := conn.WriteMessage(websocket.PingMessage, nil)
			writeMu.Unlock()
			if err != nil {
				h.logger.Error("Failed to send ping", zap.Error(err))
				close(done)
				return
			}
		case <-done:
			return
		}
	}
}

func (h *WebSocketHandler) getClientID(conn *websocket.Conn) string {
	return conn.RemoteAddr().String()
}
