# motorcycle-cv

Real-time motorcycle riding analysis: browser client streams frames to a Go backend over WebSocket, which calls a Python ML service (YOLOv8 + MediaPipe + custom scene analysis) and returns scores, annotations, and feedback.

## Quick start

Prerequisites:

- Python 3.10+ with pip
- Go 1.22+
- (Optional) NVIDIA GPU with CUDA for faster inference

Install Python deps:

```bash
git clone https://github.com/san-kum/motorcycle-cv.git
cd motorcycle-cv
pip install -r requirements.txt
```

Start the ML service (port 5000):

```bash
python server/cv/server.py
```

Start the Go server (port 8080):

```bash
go run server/main.go
```

Open the client:

```
http://localhost:8080/
```

Click "Start Analysis" and allow camera access.

## Endpoints

Backend (Go):

- WebSocket: `/ws`
- REST: `/api/v1/health`, `/api/v1/analyze-frame`, `/api/v1/stats`

ML Service (Python):

- `GET /health`
- `POST /analyze`
- `GET /models/info`

## Notes

- First run will download `yolov8n.pt`; cold start can take ~30–60s.
- Static client assets are served from `/static`.
- To point the Go server at a different ML URL, edit `server/main.go` in `ml.NewClient("http://localhost:5000", ...)`.