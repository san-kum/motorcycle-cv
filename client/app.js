class MotorcycleFeedback {
  constructor() {
    this.videoElement = document.getElementById("videoElement");
    this.overlayCanvas = document.getElementById("overlayCanvas");
    this.ctx = this.overlayCanvas.getContext("2d");

    this.startBtn = document.getElementById("startBtn");
    this.stopBtn = document.getElementById("stopBtn");
    this.recordBtn = document.getElementById("recordBtn");

    this.connectionStatus = document.getElementById("connectionStatus");
    this.processingStatus = document.getElementById("processingStatus");
    this.overallScore = document.getElementById("overallScore");
    this.postureScore = document.getElementById("posture");
    this.laneScore = document.getElementById("laneScore");
    this.speedScore = document.getElementById("speedScore");
    this.feedbackList = document.getElementById("feedbackList");

    this.socket = null;
    this.mediaStream = null;
    this.isAnalyzing = false;
    this.isRecording = false;
    this.frameInterval = null;

    this.config = {
      frameRate: 10,
      websocketUrl: `${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/ws`,
      videoConstraints: {
        video: {
          width: { ideal: 1280 },
          height: { ideal: 720 },
          frameRate: { ideal: 30 },
          facingMode: { ideal: 'environment' },
        },
        audio: false,
      },
    };
    this.initializeEventListeners();
  }

  initializeEventListeners() {
    this.startBtn.addEventListener("click", () => this.startAnalysis());
    this.stopBtn.addEventListener("click", () => this.stopAnalysis());
    this.recordBtn.addEventListener("click", () => this.toggleRecording());

    window.addEventListener("resize", () => this.resizeCanvas());
    this.videoElement.addEventListener("loadedmetadata", () =>
      this.resizeCanvas(),
    );
  }
  async startAnalysis() {
    try {
      this.updateStatus("Intializing camera...", "processing");
      this.mediaStream = await navigator.mediaDevices.getUserMedia(
        this.config.videoConstraints,
      );
      this.videoElement.srcObject = this.mediaStream;

      await new Promise((resolve) => {
        this.videoElement.onloadedmetadata = resolve;
      });
      await this.connectWebSocket();
      this.startFrameProcessing();

      this.isAnalyzing = true;
      this.startBtn.disabled = true;
      this.stopBtn.disabled = false;
      this.updateStatus("Analyzing...", "connected");
    } catch (error) {
      console.error("Failed to start analysis: ", error);
      this.addFeedback("Failed to access camera", "error");
      this.updateStatus("Connection failed", "disconnected");
    }
  }
  stopAnalysis() {
    this.isAnalyzing = false;

    if (this.frameInterval) {
      clearInterval(this.frameInterval);
      this.frameInterval = null;
    }

    if (this.socket) {
      this.socket.close();
      this.socket = null;
    }

    if (this.mediaStream) {
      this.mediaStream.getTracks().forEach((track) => track.stop());
      this.mediaStream = null;
    }

    this.videoElement.srcObject = null;
    this.ctx.clearRect(
      0,
      0,
      this.overlayCanvas.width,
      this.overlayCanvas.height,
    );
    this.startBtn.disabled = false;
    this.stopBtn.disabled = true;
    this.updateStatus("Stopped", "disconnected");
    this.resetScores();
  }

  async connectWebSocket() {
    return new Promise((resolve, reject) => {
      this.socket = new WebSocket(this.config.websocketUrl);

      this.socket.onopen = () => {
        console.log("WebSocket connected");
        resolve();
      };

      this.socket.onmessage = (event) => {
        this.handleServerMessage(JSON.parse(event.data));
      };

      this.socket.onclose = () => {
        console.log("WebSocket disconnected");
        this.updateStatus("Disconnected", "disconnected");
      };

      this.socket.onerror = (error) => {
        console.error("WebSocket error:", error);
        reject(error);
      };

      setTimeout(() => {
        if (this.socket.readyState !== WebSocket.OPEN) {
          reject(new Error("WebSocket connection timeout"));
        }
      }, 5000);
    });
  }

  startFrameProcessing() {
    this.frameInterval = setInterval(() => {
      if (
        this.isAnalyzing &&
        this.socket &&
        this.socket.readyState === WebSocket.OPEN
      ) {
        this.captureAndSendFrame();
      }
    }, 1000 / this.config.frameRate);
  }

  captureAndSendFrame() {
    this.resizeCanvas();

    this.ctx.drawImage(
      this.videoElement,
      0,
      0,
      this.overlayCanvas.width,
      this.overlayCanvas.height,
    );

    const imageData = this.overlayCanvas.toDataURL("image/jpeg", 0.8);

    const message = {
      type: "frame",
      data: imageData,
      timestamp: Date.now(),
    };

    this.socket.send(JSON.stringify(message));
  }

  handleServerMessage(message) {
    switch (message.type) {
      case "analysis":
        this.updateAnalysisResults(message.data);
        break;
      case "feedback":
        this.addFeedback(message.data.message, message.data.type);
        break;
      case "error":
        this.addFeedback(message.data.message, "error");
        console.error("Server error:", message.data);
        break;
      default:
        console.log("Unknown message type:", message.type);
    }
  }

  updateAnalysisResults(data) {
    this.overallScore.textContent = `${data.overall_score}/100`;
    this.postureScore.textContent = `${data.posture_score}/100`;
    this.laneScore.textContent = `${data.lane_score}/100`;
    this.speedScore.textContent = `${data.speed_score}/100`;

    if (data.annotations) {
      this.drawAnnotations(data.annotations);
    }
  }

  drawAnnotations(annotations) {
    this.ctx.clearRect(
      0,
      0,
      this.overlayCanvas.width,
      this.overlayCanvas.height,
    );

    this.ctx.drawImage(
      this.videoElement,
      0,
      0,
      this.overlayCanvas.width,
      this.overlayCanvas.height,
    );

    this.ctx.strokeStyle = "#00ff00";
    this.ctx.lineWidth = 2;
    this.ctx.fillStyle = "#00ff00";
    this.ctx.font = "16px Arial";

    annotations.forEach((annotation) => {
      if (annotation.type === "bounding_box") {
        const { x, y, width, height, label, confidence } = annotation;

        this.ctx.strokeRect(x, y, width, height);

        const text = `${label} (${(confidence * 100).toFixed(1)}%)`;
        this.ctx.fillText(text, x, y - 10);
      }
    });
  }

  addFeedback(message, type = "info") {
    const noFeedback = this.feedbackList.querySelector(".no-feedback");
    if (noFeedback) {
      noFeedback.remove();
    }

    const feedbackElement = document.createElement("div");
    feedbackElement.className = `feedback-message ${type}`;
    feedbackElement.textContent = message;

    const timestamp = new Date().toLocaleTimeString();
    feedbackElement.innerHTML = `<strong>${timestamp}:</strong> ${message}`;

    this.feedbackList.appendChild(feedbackElement);
    this.feedbackList.scrollTop = this.feedbackList.scrollHeight;

    const messages = this.feedbackList.querySelectorAll(".feedback-message");
    if (messages.length > 20) {
      messages[0].remove();
    }
  }

  updateStatus(message, type) {
    this.processingStatus.textContent = message;
    this.connectionStatus.className = `status ${type}`;
    this.connectionStatus.textContent =
      type === "connected"
        ? "Connected"
        : type === "disconnected"
          ? "Disconnected"
          : "Processing";
  }

  resetScores() {
    this.overallScore.textContent = "--";
    this.postureScore.textContent = "--";
    this.laneScore.textContent = "--";
    this.speedScore.textContent = "--";

    this.feedbackList.innerHTML =
      '<p class="no-feedback">Start analysis to receive feedback</p>';
  }

  resizeCanvas() {
    const rect = this.videoElement.getBoundingClientRect();
    this.overlayCanvas.width = this.videoElement.videoWidth || rect.width;
    this.overlayCanvas.height = this.videoElement.videoHeight || rect.height;
  }

  toggleRecording() {
    if (!this.isRecording) {
      this.isRecording = true;
      this.recordBtn.textContent = "Stop Recording";
      this.recordBtn.style.background = "#e74c3c";
      this.addFeedback("Recording started (feature coming soon)", "info");
    } else {
      this.isRecording = false;
      this.recordBtn.textContent = "Start Recording";
      this.recordBtn.style.background = "#f39c12";
      this.addFeedback("Recording stopped", "info");
    }
  }
}
document.addEventListener("DOMContentLoaded", () => {
  new MotorcycleFeedback();
});
