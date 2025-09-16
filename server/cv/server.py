#!/usr/bin/env python3

import os
import io
import base64
import time
import logging
from typing import Dict, List, Any, Optional, Tuple
import traceback

import torch
import cv2
import numpy as np
from PIL import Image
import flask
from flask import Flask, request, jsonify
from flask_cors import CORS


logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s",
    handlers=[logging.FileHandler("ml_service.log"), logging.StreamHandler()],
)

logger = logging.getLogger(__name__)


class MLService:
    def __init__(self):
        self.device = self._get_optimal_device()
        logger.info(f"Using device: {self.device}")

        self.object_detector = ObjectDetector(device=self.device)
        self.pose_estimator = PoseEstimator(device=self.device)
        self.scene_analyzer = SceneAnalyzer(device=self.device)
        self.riding_analyzer = RidingAnalyzer()

        self.stats = {
            "total_requests": 0,
            "successfull_analyses": 0,
            "failed_analyses": 0,
            "average_processing_time": 0.0,
            "model_versions": self._get_model_versions(),
        }

        logger.info("ML service initialized successfully")

    def _get_optimal_device(self) -> str:
        if torch.cuda.is_available():
            device = "cuda"
            logger.info(f"CUDA available: {torch.cuda.get_device_name(0)}")
        elif hasattr(torch.backends, "mps") and torch.backends.mps.is_available():
            device = "mps"  # apple
            logger.info(f"Using apple metal performance shaders")
        else:
            device = "cpu"
            logger.info("Using CPU for inference")
        return device

    def _get_model_versions(self) -> Dict[str, str]:
        return {
            "object_detector": "YOLOv8n-1.0",
            "pose_estimator": "MediaPipe-0.10.3",
            "scene_analyzer": "Custom-1.0",
            "torch_version": torch.__version__,
            "service_version": "1.0.0",
        }

    def analyze_frame(
        self, image_data: bytes, config: Dict[str, Any] = None  # type: ignore
    ) -> Dict[str, Any]:
        start_time = time.time()
        self.stats["total_requests"] += 1

        try:
            image = self._decode_image(image_data)
            if image is None:
                raise ValueError("failed to decode image data")

            detections = self.object_detector.detect_objects(image)
            pose_keypoints = self.pose_estimator.estimate_pose(image)
            scene_analysis = self.scene_analyzer.analyze_scene(image)

            analysis_results = self.riding_analyzer.analyze_riding_technique(
                image, detections, pose_keypoints, scene_analysis
            )

            processing_time = (time.time() - start_time) * 1000
            self._update_performance_stats(processing_time)

            response = {
                "overall_score": analysis_results["overall_score"],
                "posture_score": analysis_results["posture_score"],
                "lane_score": analysis_results["lane_score"],
                "speed_score": analysis_results["speed_score"],
                "detections": self._format_detections(detections),
                "pose_keypoints": self._frame_pose_keypoints(pose_keypoints),
                "scene_analysis": scene_analysis,
                "processing_time": processing_time,
                "model_version": self.stats["model_versions"]["service_version"],
            }

            self.stats["successful_analyses"] += 1
            logger.debug(f"Frame analyzed successfully in {processing_time:.2f}ms")
            return response

        except Exception as e:
            self.stats["failed_analyses"] += 1
            logger.error(f"Frame analysis failed: {str(e)}")
            logger.error(traceback.format_exc())
            raise

    def _decode_image(self, image_data: bytes) -> Optional[np.ndarray]:
        try:
            if isinstance(image_data, str):
                image_data = base64.b64decode(image_data)

            nparr = np.frombuffer(image_data, np.uint8)
            image = cv2.imdecode(nparr, cv2.IMREAD_COLOR)

            if image is None:
                pil_image = Image.open(io.BytesIO(image_data))
                image = cv2.cvtColor(np.array(pil_image), cv2.COLOR_RGB2BGR)

            return image

        except Exception as e:
            logger.error(f"Image decoding failed: {e}")
            return None

    def _format_detections(self, detections: List[Dict]) -> List[Dict]:  # type: ignore
        formatted = []
        for det in detections:
            formatted.append(
                {
                    "class": det["class_name"],
                    "confidence": float(det["confidence"]),
                    "bounding_box": {
                        "x": float(det["bbox"][0]),
                        "y": float(det["bbox"][1]),
                        "width": float(det["bbox"][2] - det["bbox"][0]),
                        "height": float(det["bbox"][3] - det["bbox"][1]),
                    },
                    "track_id": det.get("track_id", -1),
                }
            )
            return formatted

    def _frame_pose_keypoints(self, pose_results: Dict) -> List[Dict]:  # type: ignore
        keypoints = []

        if pose_results and "landmarks" in pose_results:
            for landmark_name, landmark_data in pose_results["landmarks"].items():
                keypoints.append(
                    {
                        "name": landmark_name,
                        "x": float(landmark_data["x"]),
                        "y": float(landmark_data["y"]),
                        "confidence": float(landmark_data.get("confidence", 0.0)),
                        "visible": landmark_data.get("visible", True),
                    }
                )

        return keypoints

    def _update_performance_stats(self, processing_time: float):
        if self.stats["average_processing_time"] == 0:
            self.stats["average_processing_time"] = processing_time
        else:
            alpha = 0.1
            self.stats["average_processing_time"] = (
                alpha * processing_time
                + (1 - alpha) * self.stats["average_processing_time"]
            )

    def get_health_status(self) -> Dict[str, Any]:
        return {
            "status": "healthy",
            "device": self.device,
            "models_loaded": True,
            "stats": self.stats,
            "memory_usage": self._get_memory_usage(),
            "gpu_usage": self._get_gpu_usage() if self.device == "cuda" else None,
        }

    def _get_memory_usage(self) -> Dict[str, float]:
        import psutil

        process = psutil.Process()
        memory_info = process.memory_info()

        return {
            "rss_mb": memory_info.rss / 1024 / 1024,
            "vms_mb": memory_info.rss / 1024 / 1024,
            "percent": process.memory_percent(),
        }

    def _get_gpu_usage(self) -> Optional[Dict[str, float]]:
        if not torch.cuda.is_available():
            return None
        try:
            gpu_memory = torch.cuda.memory_stats()
            return {
                "allocated_mb": gpu_memory.get("allocated_bytes.all.current", 0)
                / 1024
                / 1024,
                "reserved_mb": gpu_memory.get("reserved_bytes.all.current", 0)
                / 1024
                / 1024,
                "max_allocated_mb": gpu_memory.get("allocated_bytes.all.peak", 0)
                / 1024
                / 1024,
            }
        except Exception as e:
            logger.warning(f"Failed to get GPU stats: {e}")
            return None


app = Flask(__name__)
CORS(app)

ml_service = None


def initialize_ml_service():
    global ml_service

    try:
        ml_service = MLService()
        logger.info("ML service ready for requests")
    except Exception as e:
        logger.error(f"failed to initialize ML service: {e}")
        logger.error(traceback.format_exc())
        raise


@app.route("/health", methods=["GET"])
def health_check():
    try:
        if ml_service is None:
            return (
                jsonify({"status": "error", "message": "ML service not initialized"}),
                503,
            )
        health_data = ml_service.get_health_status()
        return jsonify(health_check), 200
    except Exception as e:
        logger.error(f"Health check failed: {e}")
        return jsonify({"status": "error", "message": str(e)}), 500


@app.route("/analyze", methods=["GET"])
def analyze_frame():
    try:
        if ml_service is None:
            return jsonify({"error": "ML service not intialized"}), 500

        data = request.get_json()
        if not data or "image_data" not in data:
            return jsonify({"error": "Missing image_data in request"}), 400
        image_data = data["image_data"]
        config = data.get("config", {})
        result = ml_service.analyze_frame(image_data, config)
        return jsonify(result), 200
    except ValueError as e:
        logger.error(f"Validation error: {e}")
        return jsonify({"error": f"Validation error: {str(e)}"}), 400
    except Exception as e:
        logger.error(f"Analysis failed: {e}")
        logger.error(traceback.format_exc())
        return jsonify({"error": f"Analysis failed: {str(e)}"}), 500


@app.route("/models/info", methods=["GET"])
def get_models_info():
    try:
        if ml_service is None:
            return jsonify({"error": "ML service not initialized"}), 503

        return (
            jsonify(
                {
                    "models": ml_service.stats["model_versions"],
                    "device": ml_service.device,
                    "capabilities": {
                        "object_detection": True,
                        "pose_estimation": True,
                        "scene_analysis": True,
                        "riding_analysis": True,
                    },
                }
            ),
            200,
        )
    except Exception as e:
        logger.error(f"Failed to get model info: {e}")
        return jsonify({"error": str(e)}), 500


@app.route("/config", methods=["PUT"])
def update_config():
    try:
        if ml_service is None:
            return jsonify({"error": "ML service not initialzied"}), 503
        config = request.get_json()
        if not config:
            return jsonify({"error": "No configuration provided"}), 400

        logger.info(f"Configuration update requested: {config}")
        return jsonify({"status": "success", "message": "Configuration updated"}), 200

    except Exception as e:

        logger.error(f"Configuration update failed: {e}")
        return jsonify({"error": str(e)}), 500


@app.errorhandler(404)
def not_found(error):
    return jsonify({"error": "Endpoint not found."}), 404


@app.errorhandler(500)
def internal_error(error):
    logger.error(f"Internal server error: {error}")
    return jsonify({"error": "Internal server error"}), 500


if __name__ == "__main__":
    initialize_ml_service()

    port = int(os.environ.get("PORT", 5000))
    debug = os.environ.get("DEBUG", "False").lower() == "true"
    logger.info(f"Starting ML service on port {port}")
    app.run(host="0.0.0.0", port=port, debug=debug, threaded=True)
