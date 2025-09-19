class MotorcycleFeedbackApp {
  constructor() {
    this.videoElement = document.getElementById("videoElement");
    this.overlayCanvas = document.getElementById("overlayCanvas");
    this.ctx = this.overlayCanvas.getContext("2d");

    this.startBtn = document.getElementById("startBtn");
    this.stopBtn = document.getElementById("stopBtn");
    this.recordBtn = document.getElementById("recordBtn");
    this.captureBtn = document.getElementById("captureBtn");

    this.connectionStatus = document.getElementById("connectionStatus");
    this.analysisIndicator = document.getElementById("analysisIndicator");
    this.recordingIndicator = document.getElementById("recordingIndicator");

    this.overallScore = document.getElementById("overallScore");
    this.postureScore = document.getElementById("postureScore");
    this.laneScore = document.getElementById("laneScore");
    this.speedScore = document.getElementById("speedScore");

    this.overallProgress = document.getElementById("overallProgress");
    this.postureProgress = document.getElementById("postureProgress");
    this.laneProgress = document.getElementById("laneProgress");
    this.speedProgress = document.getElementById("speedProgress");

    this.overallGrade = document.getElementById("overallGrade");
    this.overallTrend = document.getElementById("overallTrend");

    this.feedbackList = document.getElementById("feedbackList");
    this.clearFeedbackBtn = document.getElementById("clearFeedback");

    this.sessionDuration = document.getElementById("sessionDuration");
    this.framesAnalyzed = document.getElementById("framesAnalyzed");
    this.avgScore = document.getElementById("avgScore");
    this.improvements = document.getElementById("improvements");

    this.loadingScreen = document.getElementById("loadingScreen");
    this.onboardingModal = document.getElementById("onboardingModal");
    this.settingsModal = document.getElementById("settingsModal");

    this.socket = null;
    this.mediaStream = null;
    this.isAnalyzing = false;
    this.isRecording = false;
    this.isFullscreen = false;
    this.sessionStartTime = null;
    this.frameCount = 0;
    this.scoreHistory = [];
    this.previousScores = {};

    this.config = {
      frameRate: 10,
      websocketUrl: `${location.protocol === "https:" ? "wss" : "ws"}://${
        location.host
      }/ws`,
      videoConstraints: {
        video: {
          width: { ideal: 1280 },
          height: { ideal: 720 },
          frameRate: { ideal: 30 },
          facingMode: { ideal: "environment" },
        },
        audio: false,
      },
      analysis: {
        confidenceThreshold: 0.5,
        audioFeedback: true,
        visualFeedback: true,
      },
      ui: {
        theme: "light",
        animations: true,
      },
    };

    this.init();
  }

  async init() {
    try {
      this.showLoadingScreen();

      this.loadConfig();

      this.initializeUI();

      this.setupEventListeners();

      if (!localStorage.getItem("motorcycle-cv-onboarded")) {
        this.showOnboarding();
      } else {
        this.hideLoadingScreen();
      }

      this.applyTheme(this.config.ui.theme);
    } catch (error) {
      console.error("Failed to initialize app:", error);
      this.showToast("Failed to initialize application", "error");
      this.hideLoadingScreen();
    }
  }

  initializeUI() {
    this.setupCanvas();

    this.resetScores();

    this.updateThemeToggle();

    this.updateSessionStats();
  }

  setupEventListeners() {
    this.startBtn.addEventListener("click", () => this.startAnalysis());
    this.stopBtn.addEventListener("click", () => this.stopAnalysis());
    this.recordBtn.addEventListener("click", () => this.toggleRecording());
    this.captureBtn.addEventListener("click", () => this.captureFrame());

    document
      .getElementById("settingsBtn")
      .addEventListener("click", () => this.showSettings());
    document
      .getElementById("themeToggle")
      .addEventListener("click", () => this.toggleTheme());
    document
      .getElementById("fullscreenBtn")
      .addEventListener("click", () => this.toggleFullscreen());
    document
      .getElementById("clearFeedback")
      .addEventListener("click", () => this.clearFeedback());

    document
      .getElementById("closeOnboarding")
      .addEventListener("click", () => this.hideOnboarding());
    document
      .getElementById("closeSettings")
      .addEventListener("click", () => this.hideSettings());
    document
      .getElementById("nextStep")
      .addEventListener("click", () => this.nextOnboardingStep());
    document
      .getElementById("prevStep")
      .addEventListener("click", () => this.prevOnboardingStep());
    document
      .getElementById("startApp")
      .addEventListener("click", () => this.startApp());

    document
      .getElementById("saveSettings")
      .addEventListener("click", () => this.saveSettings());
    document
      .getElementById("resetSettings")
      .addEventListener("click", () => this.resetSettings());

    document.getElementById("frameRate").addEventListener("input", (e) => {
      this.config.frameRate = parseInt(e.target.value);
      document.querySelector(
        ".setting-value"
      ).textContent = `${e.target.value} FPS`;
    });

    document
      .getElementById("confidenceThreshold")
      .addEventListener("input", (e) => {
        this.config.analysis.confidenceThreshold = parseFloat(e.target.value);
        document.querySelectorAll(".setting-value")[1].textContent =
          e.target.value;
      });

    document.getElementById("audioFeedback").addEventListener("change", (e) => {
      this.config.analysis.audioFeedback = e.target.checked;
    });

    document
      .getElementById("visualFeedback")
      .addEventListener("change", (e) => {
        this.config.analysis.visualFeedback = e.target.checked;
      });

    window.addEventListener("resize", () => this.setupCanvas());
    window.addEventListener("beforeunload", () => this.cleanup());

    this.videoElement.addEventListener("loadedmetadata", () =>
      this.setupCanvas()
    );

    document.addEventListener("keydown", (e) => this.handleKeyboard(e));
  }

  setupCanvas() {
    const rect = this.videoElement.getBoundingClientRect();
    const videoWidth = this.videoElement.videoWidth || rect.width;
    const videoHeight = this.videoElement.videoHeight || rect.height;

    this.overlayCanvas.width = videoWidth;
    this.overlayCanvas.height = videoHeight;

    this.overlayCanvas.style.width = "100%";
    this.overlayCanvas.style.height = "100%";
  }

  async startAnalysis() {
    try {
      this.updateStatus("Initializing camera...", "processing");

      this.mediaStream = await navigator.mediaDevices.getUserMedia(
        this.config.videoConstraints
      );
      this.videoElement.srcObject = this.mediaStream;

      await new Promise((resolve) => {
        this.videoElement.onloadedmetadata = resolve;
      });

      await this.connectWebSocket();

      this.startFrameProcessing();

      this.isAnalyzing = true;
      this.sessionStartTime = Date.now();
      this.frameCount = 0;
      this.scoreHistory = [];

      this.startBtn.disabled = true;
      this.stopBtn.disabled = false;
      this.recordBtn.disabled = false;
      this.captureBtn.disabled = false;

      this.updateStatus("Analyzing...", "connected");
      this.showToast("Analysis started successfully", "success");
    } catch (error) {
      console.error("Failed to start analysis:", error);
      this.showToast(
        "Failed to start analysis. Please check camera permissions.",
        "error"
      );
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
      this.overlayCanvas.height
    );

    this.startBtn.disabled = false;
    this.stopBtn.disabled = true;
    this.recordBtn.disabled = true;
    this.captureBtn.disabled = true;

    this.updateStatus("Stopped", "disconnected");
    this.hideAnalysisIndicator();
    this.hideRecordingIndicator();
    this.resetScores();

    this.showToast("Analysis stopped", "info");
  }

  async connectWebSocket() {
    return new Promise((resolve, reject) => {
      this.socket = new WebSocket(this.config.websocketUrl);

      this.socket.onopen = () => {
        console.log("WebSocket connected");
        this.updateStatus("Connected", "connected");
        resolve();
      };

      this.socket.onmessage = (event) => {
        this.handleServerMessage(JSON.parse(event.data));
      };

      this.socket.onclose = () => {
        console.log("WebSocket disconnected");
        this.updateStatus("Disconnected", "disconnected");
        if (this.isAnalyzing) {
          this.showToast(
            "Connection lost. Attempting to reconnect...",
            "warning"
          );
          setTimeout(() => this.connectWebSocket(), 3000);
        }
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
    this.setupCanvas();

    this.ctx.drawImage(
      this.videoElement,
      0,
      0,
      this.overlayCanvas.width,
      this.overlayCanvas.height
    );

    const imageData = this.overlayCanvas.toDataURL("image/jpeg", 0.8);

    const message = {
      type: "frame",
      data: imageData,
      timestamp: Date.now(),
    };

    this.socket.send(JSON.stringify(message));
    this.frameCount++;
    this.updateSessionStats();

    this.showAnalysisIndicator();
  }

  handleServerMessage(message) {
    switch (message.type) {
      case "analysis":
        this.updateAnalysisResults(message.data);
        this.hideAnalysisIndicator();
        break;
      case "feedback":
        this.addFeedback(
          message.data.message,
          message.data.type,
          message.data.score
        );
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
    this.updateScore("overall", data.overall_score);
    this.updateScore("posture", data.posture_score);
    this.updateScore("lane", data.lane_score);
    this.updateScore("speed", data.speed_score);

    this.updateProgress("overall", data.overall_score);
    this.updateProgress("posture", data.posture_score);
    this.updateProgress("lane", data.lane_score);
    this.updateProgress("speed", data.speed_score);

    this.updateGrade(data.overall_score);
    this.updateTrend(data.overall_score);

    this.scoreHistory.push({
      timestamp: Date.now(),
      overall: data.overall_score,
      posture: data.posture_score,
      lane: data.lane_score,
      speed: data.speed_score,
    });

    if (this.scoreHistory.length > 100) {
      this.scoreHistory.shift();
    }

    if (this.config.analysis.visualFeedback && data.annotations) {
      this.drawAnnotations(data.annotations);
    }

    if (this.config.analysis.audioFeedback) {
      this.playAudioFeedback(data.overall_score);
    }
  }

  updateScore(type, score) {
    const element = document.getElementById(`${type}Score`);
    if (element) {
      element.textContent = score || "--";

      element.classList.add("score-update");
      setTimeout(() => element.classList.remove("score-update"), 300);
    }
  }

  updateProgress(type, score) {
    const element = document.getElementById(`${type}Progress`);
    if (element) {
      const percentage = Math.max(0, Math.min(100, score || 0));
      element.style.width = `${percentage}%`;
    }
  }

  updateGrade(score) {
    let grade = "--";
    if (score >= 90) grade = "A+";
    else if (score >= 80) grade = "A";
    else if (score >= 70) grade = "B";
    else if (score >= 60) grade = "C";
    else if (score >= 50) grade = "D";
    else if (score > 0) grade = "F";

    this.overallGrade.textContent = grade;
  }

  updateTrend(score) {
    if (this.previousScores.overall !== undefined) {
      const trend = score - this.previousScores.overall;
      const trendIcon = this.overallTrend.querySelector("i");

      if (trend > 5) {
        trendIcon.className = "fas fa-arrow-up";
        trendIcon.style.color = "var(--success-color)";
      } else if (trend < -5) {
        trendIcon.className = "fas fa-arrow-down";
        trendIcon.style.color = "var(--danger-color)";
      } else {
        trendIcon.className = "fas fa-minus";
        trendIcon.style.color = "var(--text-secondary)";
      }
    }

    this.previousScores.overall = score;
  }

  drawAnnotations(annotations) {
    this.ctx.clearRect(
      0,
      0,
      this.overlayCanvas.width,
      this.overlayCanvas.height
    );

    this.ctx.drawImage(
      this.videoElement,
      0,
      0,
      this.overlayCanvas.width,
      this.overlayCanvas.height
    );

    this.ctx.strokeStyle = "#00ff00";
    this.ctx.lineWidth = 2;
    this.ctx.fillStyle = "#00ff00";
    this.ctx.font = "16px Inter, sans-serif";

    annotations.forEach((annotation) => {
      if (annotation.type === "bounding_box") {
        const { x, y, width, height, label, confidence } = annotation;

        this.ctx.strokeRect(x, y, width, height);

        const text = `${label} (${(confidence * 100).toFixed(1)}%)`;
        this.ctx.fillText(text, x, y - 10);
      } else if (annotation.type === "keypoint") {
        const { x, y, label, confidence } = annotation;

        this.ctx.beginPath();
        this.ctx.arc(x, y, 5, 0, 2 * Math.PI);
        this.ctx.fill();

        this.ctx.fillText(label, x + 10, y - 10);
      }
    });
  }

  addFeedback(message, type = "info", score = null) {
    const noFeedback = this.feedbackList.querySelector(".no-feedback");
    if (noFeedback) {
      noFeedback.remove();
    }

    const feedbackElement = document.createElement("div");
    feedbackElement.className = `feedback-message ${type}`;

    const timestamp = new Date().toLocaleTimeString();
    const icon = this.getFeedbackIcon(type);

    feedbackElement.innerHTML = `
      <div class="feedback-icon">${icon}</div>
      <div class="feedback-content-text">
        <strong>${timestamp}:</strong> ${message}
        ${
          score
            ? `<div class="feedback-timestamp">Score: ${score}/100</div>`
            : ""
        }
      </div>
    `;

    this.feedbackList.appendChild(feedbackElement);
    this.feedbackList.scrollTop = this.feedbackList.scrollHeight;

    const messages = this.feedbackList.querySelectorAll(".feedback-message");
    if (messages.length > 20) {
      messages[0].remove();
    }

    if (type === "info") {
      setTimeout(() => {
        if (feedbackElement.parentNode) {
          feedbackElement.remove();
        }
      }, 10000);
    }
  }

  getFeedbackIcon(type) {
    const icons = {
      success: '<i class="fas fa-check-circle"></i>',
      warning: '<i class="fas fa-exclamation-triangle"></i>',
      error: '<i class="fas fa-times-circle"></i>',
      info: '<i class="fas fa-info-circle"></i>',
    };
    return icons[type] || icons.info;
  }

  clearFeedback() {
    this.feedbackList.innerHTML = `
      <div class="no-feedback">
        <i class="fas fa-info-circle"></i>
        <p>Start analysis to receive personalized feedback</p>
      </div>
    `;
  }

  updateStatus(message, type) {
    this.connectionStatus.className = `status-indicator ${type}`;
    this.connectionStatus.querySelector("span").textContent = message;
  }

  showAnalysisIndicator() {
    this.analysisIndicator.classList.add("active");
    setTimeout(() => {
      this.analysisIndicator.classList.remove("active");
    }, 1000);
  }

  hideAnalysisIndicator() {
    this.analysisIndicator.classList.remove("active");
  }

  showRecordingIndicator() {
    this.recordingIndicator.classList.add("active");
  }

  hideRecordingIndicator() {
    this.recordingIndicator.classList.remove("active");
  }

  toggleRecording() {
    if (!this.isRecording) {
      this.isRecording = true;
      this.recordBtn.innerHTML =
        '<i class="fas fa-stop"></i><span>Stop Recording</span>';
      this.recordBtn.classList.remove("btn-warning");
      this.recordBtn.classList.add("btn-danger");
      this.showRecordingIndicator();
      this.showToast("Recording started", "info");
    } else {
      this.isRecording = false;
      this.recordBtn.innerHTML =
        '<i class="fas fa-video"></i><span>Record Session</span>';
      this.recordBtn.classList.remove("btn-danger");
      this.recordBtn.classList.add("btn-warning");
      this.hideRecordingIndicator();
      this.showToast("Recording stopped", "info");
    }
  }

  captureFrame() {
    const link = document.createElement("a");
    link.download = `motorcycle-cv-${new Date()
      .toISOString()
      .slice(0, 19)}.jpg`;
    link.href = this.overlayCanvas.toDataURL("image/jpeg", 0.9);
    link.click();

    this.showToast("Frame captured", "success");
  }

  updateSessionStats() {
    if (this.sessionStartTime) {
      const duration = Date.now() - this.sessionStartTime;
      const minutes = Math.floor(duration / 60000);
      const seconds = Math.floor((duration % 60000) / 1000);
      this.sessionDuration.textContent = `${minutes
        .toString()
        .padStart(2, "0")}:${seconds.toString().padStart(2, "0")}`;
    }

    this.framesAnalyzed.textContent = this.frameCount;

    if (this.scoreHistory.length > 0) {
      const avgScore =
        this.scoreHistory.reduce((sum, score) => sum + score.overall, 0) /
        this.scoreHistory.length;
      this.avgScore.textContent = Math.round(avgScore);
    }
    let improvements = 0;
    for (let i = 1; i < this.scoreHistory.length; i++) {
      if (
        this.scoreHistory[i].overall - this.scoreHistory[i - 1].overall >=
        5
      ) {
        improvements++;
      }
    }
    this.improvements.textContent = improvements;
  }

  resetScores() {
    const scores = ["overall", "posture", "lane", "speed"];
    scores.forEach((score) => {
      document.getElementById(`${score}Score`).textContent = "--";
      document.getElementById(`${score}Progress`).style.width = "0%";
    });

    this.overallGrade.textContent = "--";
    this.overallTrend.querySelector("i").className = "fas fa-minus";
    this.overallTrend.querySelector("i").style.color = "var(--text-secondary)";
  }

  showOnboarding() {
    this.onboardingModal.classList.add("active");
  }

  hideOnboarding() {
    this.onboardingModal.classList.remove("active");
    localStorage.setItem("motorcycle-cv-onboarded", "true");
    this.hideLoadingScreen();
  }

  showSettings() {
    this.settingsModal.classList.add("active");
  }

  hideSettings() {
    this.settingsModal.classList.remove("active");
  }

  showLoadingScreen() {
    this.loadingScreen.classList.remove("hidden");
  }

  hideLoadingScreen() {
    this.loadingScreen.classList.add("hidden");
  }

  nextOnboardingStep() {
    const currentStep = document.querySelector(".onboarding-step.active");
    const nextStep = currentStep.nextElementSibling;

    if (nextStep) {
      currentStep.classList.remove("active");
      nextStep.classList.add("active");

      const prevBtn = document.getElementById("prevStep");
      const nextBtn = document.getElementById("nextStep");
      const startBtn = document.getElementById("startApp");

      prevBtn.disabled = false;

      if (nextStep.dataset.step === "3") {
        nextBtn.style.display = "none";
        startBtn.style.display = "inline-flex";
      }
    }
  }

  prevOnboardingStep() {
    const currentStep = document.querySelector(".onboarding-step.active");
    const prevStep = currentStep.previousElementSibling;

    if (prevStep) {
      currentStep.classList.remove("active");
      prevStep.classList.add("active");

      const prevBtn = document.getElementById("prevStep");
      const nextBtn = document.getElementById("nextStep");
      const startBtn = document.getElementById("startApp");

      if (prevStep.dataset.step === "1") {
        prevBtn.disabled = true;
      }

      nextBtn.style.display = "inline-flex";
      startBtn.style.display = "none";
    }
  }

  startApp() {
    this.hideOnboarding();
  }

  toggleTheme() {
    const newTheme = this.config.ui.theme === "light" ? "dark" : "light";
    this.config.ui.theme = newTheme;
    this.applyTheme(newTheme);
    this.saveConfig();
  }

  applyTheme(theme) {
    document.documentElement.setAttribute("data-theme", theme);
    this.updateThemeToggle();
  }

  updateThemeToggle() {
    const icon = document.querySelector("#themeToggle i");
    icon.className =
      this.config.ui.theme === "light" ? "fas fa-moon" : "fas fa-sun";
  }

  saveSettings() {
    this.config.frameRate = parseInt(
      document.getElementById("frameRate").value
    );
    this.config.analysis.confidenceThreshold = parseFloat(
      document.getElementById("confidenceThreshold").value
    );
    this.config.analysis.audioFeedback =
      document.getElementById("audioFeedback").checked;
    this.config.analysis.visualFeedback =
      document.getElementById("visualFeedback").checked;

    this.saveConfig();
    this.hideSettings();
    this.showToast("Settings saved successfully", "success");
  }

  resetSettings() {
    this.config = {
      frameRate: 10,
      analysis: {
        confidenceThreshold: 0.5,
        audioFeedback: true,
        visualFeedback: true,
      },
      ui: {
        theme: "light",
        animations: true,
      },
    };

    this.loadSettingsUI();
    this.showToast("Settings reset to defaults", "info");
  }

  loadSettingsUI() {
    document.getElementById("frameRate").value = this.config.frameRate;
    document.getElementById("confidenceThreshold").value =
      this.config.analysis.confidenceThreshold;
    document.getElementById("audioFeedback").checked =
      this.config.analysis.audioFeedback;
    document.getElementById("visualFeedback").checked =
      this.config.analysis.visualFeedback;

    document.querySelectorAll(
      ".setting-value"
    )[0].textContent = `${this.config.frameRate} FPS`;
    document.querySelectorAll(".setting-value")[1].textContent =
      this.config.analysis.confidenceThreshold;
  }

  saveConfig() {
    localStorage.setItem("motorcycle-cv-config", JSON.stringify(this.config));
  }

  loadConfig() {
    const saved = localStorage.getItem("motorcycle-cv-config");
    if (saved) {
      try {
        const parsed = JSON.parse(saved);
        this.config = { ...this.config, ...parsed };
      } catch (error) {
        console.error("Failed to load config:", error);
      }
    }
  }

  toggleFullscreen() {
    if (!this.isFullscreen) {
      if (document.documentElement.requestFullscreen) {
        document.documentElement.requestFullscreen();
      }
    } else {
      if (document.exitFullscreen) {
        document.exitFullscreen();
      }
    }
  }

  handleKeyboard(e) {
    if (e.ctrlKey || e.metaKey) return;

    switch (e.key) {
      case " ":
        e.preventDefault();
        if (this.isAnalyzing) {
          this.stopAnalysis();
        } else {
          this.startAnalysis();
        }
        break;
      case "r":
        if (this.isAnalyzing) {
          this.toggleRecording();
        }
        break;
      case "c":
        if (this.isAnalyzing) {
          this.captureFrame();
        }
        break;
      case "Escape":
        if (this.onboardingModal.classList.contains("active")) {
          this.hideOnboarding();
        } else if (this.settingsModal.classList.contains("active")) {
          this.hideSettings();
        }
        break;
    }
  }

  playAudioFeedback(score) {
    if (!this.config.analysis.audioFeedback) return;

    const audioContext = new (window.AudioContext ||
      window.webkitAudioContext)();
    const oscillator = audioContext.createOscillator();
    const gainNode = audioContext.createGain();

    oscillator.connect(gainNode);
    gainNode.connect(audioContext.destination);

    if (score >= 80) {
      oscillator.frequency.setValueAtTime(800, audioContext.currentTime);
    } else if (score >= 60) {
      oscillator.frequency.setValueAtTime(600, audioContext.currentTime);
    } else {
      oscillator.frequency.setValueAtTime(400, audioContext.currentTime);
    }

    gainNode.gain.setValueAtTime(0.1, audioContext.currentTime);
    gainNode.gain.exponentialRampToValueAtTime(
      0.01,
      audioContext.currentTime + 0.2
    );

    oscillator.start(audioContext.currentTime);
    oscillator.stop(audioContext.currentTime + 0.2);
  }

  showToast(message, type = "info") {
    const container = document.getElementById("toastContainer");
    const toast = document.createElement("div");
    toast.className = `toast ${type}`;
    toast.textContent = message;

    container.appendChild(toast);

    setTimeout(() => toast.classList.add("show"), 100);

    setTimeout(() => {
      toast.classList.remove("show");
      setTimeout(() => {
        if (toast.parentNode) {
          toast.remove();
        }
      }, 300);
    }, 3000);
  }

  cleanup() {
    this.stopAnalysis();
    this.saveConfig();
  }
}

document.addEventListener("DOMContentLoaded", () => {
  new MotorcycleFeedbackApp();
});

document.addEventListener("fullscreenchange", () => {
  const app = window.motorcycleApp;
  if (app) {
    app.isFullscreen = !!document.fullscreenElement;
    const icon = document.querySelector("#fullscreenBtn i");
    icon.className = app.isFullscreen ? "fas fa-compress" : "fas fa-expand";
  }
});

document.addEventListener("visibilitychange", () => {
  if (document.hidden) {
    console.log("Page hidden");
  } else {
    console.log("Page visible");
  }
});
