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

    // Use the video element's actual dimensions for the canvas
    this.overlayCanvas.width = videoWidth;
    this.overlayCanvas.height = videoHeight;

    // Set canvas display size to match the video element
    this.overlayCanvas.style.width = "100%";
    this.overlayCanvas.style.height = "100%";
    
    console.log("Canvas setup:", { videoWidth, videoHeight, rectWidth: rect.width, rectHeight: rect.height });
  }

  async startAnalysis() {
    try {
      this.updateStatus("Requesting camera access...", "processing");
      console.log("Starting camera analysis...");

      // Check if we're on HTTPS or localhost
      if (!this.isSecureContext()) {
        this.showToast(
          "Camera access requires HTTPS. Please use the secure tunnel URL.",
          "error"
        );
        this.updateStatus("HTTPS required", "disconnected");
        return;
      }

      // Check if mediaDevices is available
      if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia) {
        this.showToast(
          "Camera access not supported on this device/browser.",
          "error"
        );
        this.updateStatus("Camera not supported", "disconnected");
        return;
      }

      // Detect browser and platform
      const isChrome = /Chrome/.test(navigator.userAgent) && /Google Inc/.test(navigator.vendor);
      const isMobile = /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent);
      
      console.log("Browser detection:", { isChrome, isMobile, userAgent: navigator.userAgent });

      // Use Chrome-specific constraints if needed
      const constraints = this.getChromeOptimizedConstraints(isChrome, isMobile);

      console.log("Requesting camera with constraints:", constraints);
      this.showToast("Please allow camera access when prompted", "info");

      // Add a small delay for Chrome to ensure proper permission handling
      if (isChrome && isMobile) {
        await new Promise(resolve => setTimeout(resolve, 100));
      }

      this.mediaStream = await navigator.mediaDevices.getUserMedia(constraints);
      console.log("Camera access granted!");
      
      this.videoElement.srcObject = this.mediaStream;

      await new Promise((resolve) => {
        this.videoElement.onloadedmetadata = resolve;
      });

      console.log("Video metadata loaded, connecting to WebSocket...");
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
      console.error("Camera access failed:", error);
      this.handleCameraError(error);
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
        this.connectionRetries = 0;
        resolve();
      };

      this.socket.onmessage = (event) => {
        try {
          const message = JSON.parse(event.data);
          this.handleServerMessage(message);
        } catch (error) {
          console.error("Failed to parse WebSocket message:", error);
        }
      };

      this.socket.onclose = (event) => {
        console.log("WebSocket disconnected", event.code, event.reason);
        this.updateStatus("Disconnected", "disconnected");
        
        if (this.isAnalyzing && event.code !== 1000) {
          this.connectionRetries = (this.connectionRetries || 0) + 1;
          
          if (this.connectionRetries <= 5) {
            this.showToast(
              `Connection lost. Attempting to reconnect... (${this.connectionRetries}/5)`,
              "warning"
            );
            setTimeout(() => this.connectWebSocket(), 3000 * this.connectionRetries);
          } else {
            this.showToast(
              "Connection failed after multiple attempts. Please refresh the page.",
              "error"
            );
            this.stopAnalysis();
          }
        }
      };

      this.socket.onerror = (error) => {
        console.error("WebSocket error:", error);
        this.updateStatus("Connection error", "disconnected");
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
    // Check if we have a valid video stream
    if (!this.videoElement || this.videoElement.readyState !== 4) {
      return;
    }

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

    try {
      this.socket.send(JSON.stringify(message));
      this.frameCount++;
      this.updateSessionStats();
      this.showAnalysisIndicator();
    } catch (error) {
      console.error("Failed to send frame:", error);
      this.addFeedback("Failed to send frame for analysis", "error");
    }
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
        this.hideAnalysisIndicator();
        console.error("Server error:", message.data);
        break;
      case "pong":
        // Handle ping-pong for connection health
        break;
      default:
        console.log("Unknown message type:", message.type);
    }
  }

  updateAnalysisResults(data) {
    // Update scores
    this.updateScore("overall", data.overall_score);
    this.updateScore("posture", data.posture_score);
    this.updateScore("lane", data.lane_score);
    this.updateScore("speed", data.speed_score);

    // Update progress bars
    this.updateProgress("overall", data.overall_score);
    this.updateProgress("posture", data.posture_score);
    this.updateProgress("lane", data.lane_score);
    this.updateProgress("speed", data.speed_score);

    // Update grade and trend
    this.updateGrade(data.overall_score);
    this.updateTrend(data.overall_score);

    // Add to score history
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

    // Process feedback messages from the analysis result
    if (data.feedback && Array.isArray(data.feedback)) {
      data.feedback.forEach(feedback => {
        this.addFeedback(
          feedback.message || feedback.Message,
          feedback.type || feedback.Type,
          feedback.score || feedback.Score
        );
      });
    }

    // Handle visual feedback
    if (this.config.analysis.visualFeedback && data.annotations) {
      this.drawAnnotations(data.annotations);
    }

    // Handle audio feedback
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
    // Prevent duplicate messages by checking recent feedback
    const recentFeedback = Array.from(this.feedbackList.children).slice(0, 3);
    const isDuplicate = recentFeedback.some(feedback => {
      const text = feedback.textContent.toLowerCase();
      return text.includes(message.toLowerCase()) && 
             (Date.now() - (this.lastFeedbackTime || 0)) < 2000; // Within 2 seconds
    });

    if (isDuplicate) {
      return; // Skip duplicate feedback
    }

    this.lastFeedbackTime = Date.now();

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
    
    // Show mobile-specific instructions if on mobile
    const isMobile = /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent);
    const mobileInstructions = this.onboardingModal.querySelector('.mobile-instructions');
    if (mobileInstructions) {
      mobileInstructions.style.display = isMobile ? 'block' : 'none';
    }
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

  isSecureContext() {
    return window.isSecureContext || 
           location.protocol === 'https:' || 
           location.hostname === 'localhost' || 
           location.hostname === '127.0.0.1';
  }

  async checkCameraPermission() {
    try {
      if (navigator.permissions) {
        const permission = await navigator.permissions.query({ name: 'camera' });
        return permission.state; // 'granted', 'denied', or 'prompt'
      }
      return 'prompt'; // If permissions API not available, assume we can prompt
    } catch (error) {
      console.log('Permission check failed:', error);
      return 'prompt'; // Default to allowing prompt
    }
  }

  getChromeOptimizedConstraints(isChrome, isMobile) {
    if (isChrome && isMobile) {
      // Chrome mobile has stricter requirements
      return {
        video: {
          facingMode: { ideal: "environment" },
          width: { min: 320, ideal: 640, max: 1280 },
          height: { min: 240, ideal: 480, max: 960 },
          frameRate: { ideal: 15, max: 30 },
          aspectRatio: { ideal: 4/3 } // Better for mobile cameras
        },
        audio: false
      };
    } else if (isMobile) {
      // Other mobile browsers - try to get good aspect ratio
      return {
        video: {
          facingMode: { ideal: "environment" },
          width: { ideal: 1280, max: 1920 },
          height: { ideal: 960, max: 1440 }, // 4:3 aspect ratio
          frameRate: { ideal: 30, max: 60 },
          aspectRatio: { ideal: 4/3 }
        },
        audio: false
      };
    }
    
    // Desktop browsers
    return this.config.videoConstraints;
  }

  getMobileOptimizedConstraints() {
    const isMobile = /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent);
    
    if (isMobile) {
      return {
        video: {
          width: { ideal: 1280, max: 1920 },
          height: { ideal: 720, max: 1080 },
          frameRate: { ideal: 30, max: 60 },
          facingMode: { ideal: "environment" },
          aspectRatio: { ideal: 16/9 }
        },
        audio: false
      };
    }
    
    return this.config.videoConstraints;
  }

  handleCameraError(error) {
    console.error("Camera error details:", error);
    console.error("Error name:", error.name);
    console.error("Error message:", error.message);
    
    const isMobile = /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent);
    
    switch (error.name) {
      case 'NotAllowedError':
        console.log("Camera permission denied");
        this.showMobileCameraPermissionGuide();
        break;
      case 'NotFoundError':
        this.showToast("No camera found on this device.", "error");
        this.updateStatus("No camera", "disconnected");
        break;
      case 'NotReadableError':
        this.showToast("Camera is already in use by another application. Please close other apps using the camera.", "error");
        this.updateStatus("Camera in use", "disconnected");
        break;
      case 'OverconstrainedError':
        console.log("Camera constraints too strict, trying basic settings...");
        this.tryBasicCameraSettings();
        return;
      case 'SecurityError':
        this.showToast("Camera access blocked. Please ensure you're using HTTPS and allow camera permissions.", "error");
        this.updateStatus("Security error", "disconnected");
        break;
      case 'TypeError':
        this.showToast("Camera access not supported on this device.", "error");
        this.updateStatus("Not supported", "disconnected");
        break;
      default:
        console.log("Unknown camera error:", error);
        if (isMobile) {
          this.showMobileCameraPermissionGuide();
        } else {
          this.showToast("Camera access failed. Please check permissions and try again.", "error");
          this.updateStatus("Camera error", "disconnected");
        }
    }
  }

  showMobileCameraPermissionGuide() {
    // Create a modal specifically for mobile camera permission issues
    const modal = document.createElement('div');
    modal.className = 'modal active';
    modal.id = 'mobileCameraModal';
    
    const isIOS = /iPad|iPhone|iPod/.test(navigator.userAgent);
    const isAndroid = /Android/.test(navigator.userAgent);
    const isChrome = /Chrome/.test(navigator.userAgent) && /Google Inc/.test(navigator.vendor);
    
    modal.innerHTML = `
      <div class="modal-content">
        <div class="modal-header">
          <h2>📱 Camera Access Required</h2>
          <button class="close-btn" id="closeMobileCameraModal">&times;</button>
        </div>
        <div class="modal-body">
          <div class="permission-steps">
            <div class="step">
              <div class="step-icon">1</div>
              <div class="step-content">
                <h3>Look for the permission popup</h3>
                <p>Your browser should show a popup asking for camera access. <strong>Tap "Allow" or "Allow camera access"</strong>.</p>
              </div>
            </div>
            <div class="step">
              <div class="step-icon">2</div>
              <div class="step-content">
                <h3>Check the address bar</h3>
                <p>Look for a camera icon (📷) or lock icon (🔒) in your browser's address bar and tap it.</p>
              </div>
            </div>
            <div class="step">
              <div class="step-icon">3</div>
              <div class="step-content">
                <h3>Enable camera permission</h3>
                <p>In the settings that appear, make sure camera access is set to "Allow".</p>
              </div>
            </div>
            <div class="step">
              <div class="step-icon">4</div>
              <div class="step-content">
                <h3>Refresh and try again</h3>
                <p>Refresh this page and click "Start Analysis" again.</p>
              </div>
            </div>
          </div>
          
          <div class="mobile-tips">
            <h4>🔧 Troubleshooting:</h4>
            <ul>
              <li><strong>Make sure you're using the HTTPS URL</strong> from the tunnel (not HTTP)</li>
              <li>Try refreshing the page completely</li>
              <li>Close other apps that might be using the camera</li>
              <li>Try a different browser (Chrome, Safari, Firefox)</li>
              ${isIOS ? '<li>On iOS: Go to Settings > Safari > Camera and make sure it\'s set to "Allow"</li>' : ''}
              ${isAndroid ? '<li>On Android: Check your browser\'s site settings for camera permissions</li>' : ''}
              ${isChrome ? '<li><strong>Chrome users:</strong> Try tapping the lock icon in the address bar and enable camera access</li>' : ''}
            </ul>
          </div>
          
          ${isChrome ? `
            <div class="chrome-specific-tips" style="background: #e3f2fd; padding: 15px; border-radius: 5px; margin: 15px 0; border-left: 4px solid #2196f3;">
              <h4>🔵 Chrome-Specific Tips:</h4>
              <ul style="margin: 10px 0; padding-left: 20px;">
                <li>Chrome requires HTTPS for camera access - make sure you're using the secure tunnel URL</li>
                <li>Try tapping the lock icon (🔒) in the address bar and enable camera permissions</li>
                <li>If the permission popup doesn't appear, try refreshing the page</li>
                <li>Check Chrome's site settings: Tap the three dots menu → Site settings → Camera</li>
                <li>Make sure Chrome is updated to the latest version</li>
              </ul>
            </div>
          ` : ''}
          
          <div class="debug-info" style="background: #f0f0f0; padding: 10px; border-radius: 5px; margin-top: 15px; font-size: 0.8em;">
            <strong>Debug Info:</strong><br>
            Browser: ${isChrome ? 'Chrome' : 'Other'}<br>
            Platform: ${isIOS ? 'iOS' : isAndroid ? 'Android' : 'Other'}<br>
            User Agent: ${navigator.userAgent}<br>
            HTTPS: ${location.protocol === 'https:' ? 'Yes' : 'No'}<br>
            Secure Context: ${window.isSecureContext ? 'Yes' : 'No'}<br>
            MediaDevices: ${navigator.mediaDevices ? 'Available' : 'Not Available'}
          </div>
        </div>
        <div class="modal-footer">
          <button class="btn btn-secondary" id="closeMobileCameraModal">Close</button>
          <button class="btn btn-primary" id="retryMobileCamera">Try Again</button>
          <button class="btn btn-warning" id="refreshPage">Refresh Page</button>
        </div>
      </div>
    `;
    
    document.body.appendChild(modal);
    
    // Add event listeners
    document.getElementById('closeMobileCameraModal').addEventListener('click', () => {
      modal.remove();
    });
    
    document.getElementById('retryMobileCamera').addEventListener('click', () => {
      modal.remove();
      this.startAnalysis();
    });
    
    document.getElementById('refreshPage').addEventListener('click', () => {
      window.location.reload();
    });
  }

  async tryBasicCameraSettings() {
    console.log("Trying basic camera settings...");
    
    const isChrome = /Chrome/.test(navigator.userAgent) && /Google Inc/.test(navigator.vendor);
    const isMobile = /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent);
    
    // Try different constraint combinations, with Chrome-specific ones first
    const constraintSets = isChrome && isMobile ? [
      { video: { facingMode: "environment", width: { min: 320, max: 640 }, height: { min: 240, max: 480 }, aspectRatio: 4/3 }, audio: false },
      { video: { facingMode: "user", width: { min: 320, max: 640 }, height: { min: 240, max: 480 }, aspectRatio: 4/3 }, audio: false },
      { video: { width: 640, height: 480, aspectRatio: 4/3 }, audio: false },
      { video: { width: 320, height: 240, aspectRatio: 4/3 }, audio: false },
      { video: true, audio: false }
    ] : [
      { video: { facingMode: "environment", aspectRatio: 4/3 }, audio: false },
      { video: { facingMode: "user", aspectRatio: 4/3 }, audio: false },
      { video: true, audio: false },
      { video: { width: 640, height: 480, aspectRatio: 4/3 }, audio: false },
      { video: { width: 320, height: 240, aspectRatio: 4/3 }, audio: false }
    ];
    
    for (let i = 0; i < constraintSets.length; i++) {
      try {
        console.log(`Trying constraint set ${i + 1}:`, constraintSets[i]);
        this.showToast(`Trying camera settings ${i + 1}/${constraintSets.length}...`, "info");
        
        this.mediaStream = await navigator.mediaDevices.getUserMedia(constraintSets[i]);
        console.log("Camera access granted with basic settings!");
        
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
        this.showToast("Analysis started with basic camera settings", "success");
        return; // Success, exit the function
        
      } catch (error) {
        console.log(`Constraint set ${i + 1} failed:`, error);
        if (i === constraintSets.length - 1) {
          // All constraint sets failed
          console.error("All camera constraint sets failed:", error);
          this.handleCameraError(error);
        }
      }
    }
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
