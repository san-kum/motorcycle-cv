package handlers

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
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
		h.logger.Error("failed to upgraade websocket connecction", zap.Error(err))
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

	go h.pingRoutine(conn, ticker, done)

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
			h.handleMessage(conn, &message)
		}
	}

}

func (h *WebSocketHandler) handleMessage(conn *websocket.Conn, message *ClientMessage) {
	switch message.Type {
	case "frame":
		h.processVideoFrame(conn, message)
	case "ping":
		h.sendMessage(conn, "pong", map[string]any{"timestamp": time.Now().Unix()})
	case "config":
		h.handleConfigUpdate(conn, message)
	default:
		h.logger.Warn("Unknown message type recieved", zap.String("type", message.Type))
		h.sendError(conn, "Unknown message type: "+message.Type)
	}
}

func (h *WebSocketHandler) processVideoFrame(conn *websocket.Conn, message *ClientMessage) {
	imageData, err := h.extractImageData(message.Data)

	if err != nil {
		h.logger.Error("Failed to extract image data", zap.Error(err))
		h.sendError(conn, "invalid image data format")
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
			h.sendError(conn, "Frame processing failed")
			return
		}

		h.sendMessage(conn, "analysis", result)

		if len(result.Feedback) > 0 {
			for _, feedback := range result.Feedback {
				h.sendMessage(conn, "feedback", map[string]any{
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

func (h *WebSocketHandler) handleConfigUpdate(conn *websocket.Conn, message *ClientMessage) {
	var config map[string]any
	if err := json.Unmarshal([]byte(message.Data), &config); err != nil {
		h.logger.Error("Invalid config format", zap.Error(err))
		h.sendError(conn, "Invalid configuration format")
		return
	}

	h.processor.UpdateConfig(config)
	h.sendMessage(conn, "config_updated", map[string]interface{}{
		"status": "success",
		"config": config,
	})
}

func (h *WebSocketHandler) sendMessage(conn *websocket.Conn, messageType string, data interface{}) {
	message := ServerMessage{
		Type: messageType,
		Data: data,
	}

	if err := conn.WriteJSON(message); err != nil {
		h.logger.Error("Failed to send WebSocket message", zap.Error(err))
	}
}

func (h *WebSocketHandler) sendError(conn *websocket.Conn, errorMsg string) {
	h.sendMessage(conn, "error", map[string]interface{}{
		"message":   errorMsg,
		"timestamp": time.Now().Unix(),
	})
}

func (h *WebSocketHandler) pingRoutine(conn *websocket.Conn, ticker *time.Ticker, done chan struct{}) {
	for {
		select {
		case <-ticker.C:
			conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
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
