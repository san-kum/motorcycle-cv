#!/usr/bin/env python3

"""
Simple ML Service for Motorcycle CV
This is a lightweight version that doesn't require heavy ML dependencies
"""

import os
import io
import base64
import time
import logging
import random
from typing import Dict, List, Any, Optional, Tuple
import traceback

from flask import Flask, request, jsonify
from flask_cors import CORS

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

app = Flask(__name__)
CORS(app)


class SimpleRidingAnalyzer:
    """Simple riding analyzer that generates mock analysis results"""

    def __init__(self):
        self.last_analysis_time = 0
        self.analysis_count = 0

    def analyze_frame(self, image_data: bytes) -> Dict[str, Any]:
        """Analyze a single frame and return mock results"""
        current_time = time.time()

        # Simulate processing time
        time.sleep(0.1)

        # Generate mock scores with some variation
        base_scores = {
            "overall_score": random.randint(70, 95),
            "posture_score": random.randint(75, 90),
            "lane_score": random.randint(65, 85),
            "speed_score": random.randint(70, 88),
        }

        # Add some variation based on analysis count
        variation = random.uniform(-5, 5)
        for key in base_scores:
            base_scores[key] = max(0, min(100, base_scores[key] + variation))

        # Generate feedback messages
        feedback_messages = self._generate_feedback(base_scores)

        # Generate mock annotations
        annotations = self._generate_annotations()

        self.analysis_count += 1

        return {
            "overall_score": int(base_scores["overall_score"]),
            "posture_score": int(base_scores["posture_score"]),
            "lane_score": int(base_scores["lane_score"]),
            "speed_score": int(base_scores["speed_score"]),
            "feedback": feedback_messages,
            "annotations": annotations,
            "timestamp": int(time.time()),
            "processing_time": 0.1,
        }

    def _generate_feedback(self, scores: Dict[str, float]) -> List[Dict[str, Any]]:
        """Generate mock feedback messages based on scores"""
        feedback = []

        overall = scores["overall_score"]
        posture = scores["posture_score"]
        lane = scores["lane_score"]
        speed = scores["speed_score"]

        # Overall feedback
        if overall >= 90:
            feedback.append(
                {
                    "message": "Excellent riding! Keep up the great work",
                    "type": "success",
                    "score": int(overall),
                }
            )
        elif overall >= 80:
            feedback.append(
                {
                    "message": "Good riding! Minor improvements possible",
                    "type": "info",
                    "score": int(overall),
                }
            )
        elif overall >= 70:
            feedback.append(
                {
                    "message": "Decent riding. Focus on technique",
                    "type": "warning",
                    "score": int(overall),
                }
            )
        else:
            feedback.append(
                {
                    "message": "Needs improvement. Practice more",
                    "type": "error",
                    "score": int(overall),
                }
            )

        # Posture feedback
        if posture >= 85:
            feedback.append(
                {
                    "message": "Excellent riding posture!",
                    "type": "success",
                    "score": int(posture),
                }
            )
        elif posture < 75:
            feedback.append(
                {
                    "message": "Improve your riding posture",
                    "type": "warning",
                    "score": int(posture),
                }
            )

        # Lane feedback
        if lane >= 80:
            feedback.append(
                {
                    "message": "Great lane positioning",
                    "type": "success",
                    "score": int(lane),
                }
            )
        elif lane < 70:
            feedback.append(
                {
                    "message": "Work on staying in your lane",
                    "type": "warning",
                    "score": int(lane),
                }
            )

        # Speed feedback
        if speed >= 80:
            feedback.append(
                {
                    "message": "Good speed control",
                    "type": "success",
                    "score": int(speed),
                }
            )
        elif speed < 70:
            feedback.append(
                {"message": "Adjust your speed", "type": "warning", "score": int(speed)}
            )

        return feedback

    def _generate_annotations(self) -> List[Dict[str, Any]]:
        """Generate mock annotations for visualization"""
        annotations = []

        # Mock bounding boxes for motorcycle detection
        if random.random() > 0.3:  # 70% chance of detecting motorcycle
            annotations.append(
                {
                    "type": "bounding_box",
                    "x": random.randint(50, 200),
                    "y": random.randint(50, 150),
                    "width": random.randint(100, 200),
                    "height": random.randint(80, 150),
                    "label": "Motorcycle",
                    "confidence": random.uniform(0.7, 0.95),
                }
            )

        # Mock keypoints for pose estimation
        if random.random() > 0.4:  # 60% chance of pose detection
            keypoints = [
                "head",
                "shoulder_left",
                "shoulder_right",
                "elbow_left",
                "elbow_right",
            ]
            for i, keypoint in enumerate(keypoints):
                annotations.append(
                    {
                        "type": "keypoint",
                        "x": random.randint(100, 300),
                        "y": random.randint(100, 250),
                        "label": keypoint,
                        "confidence": random.uniform(0.6, 0.9),
                    }
                )

        return annotations


# Initialize analyzer
analyzer = SimpleRidingAnalyzer()


@app.route("/health", methods=["GET"])
def health_check():
    """Health check endpoint"""
    return jsonify(
        {
            "status": "healthy",
            "service": "motorcycle-cv-ml",
            "timestamp": int(time.time()),
            "version": "1.0.0",
        }
    )


@app.route("/analyze", methods=["POST"])
def analyze_frame():
    """Analyze a single frame"""
    try:
        data = request.get_json()

        if not data or "image_data" not in data:
            return jsonify({"error": "No image data provided"}), 400

        # Extract image data from base64
        image_data = data["image_data"]
        if "," in image_data:
            image_data = image_data.split(",")[1]

        # Decode base64 image
        try:
            image_bytes = base64.b64decode(image_data)
            # Validate that we got some data
            if len(image_bytes) < 100:  # Minimum size for a valid image
                logger.warning(f"Image data too small: {len(image_bytes)} bytes")
                # Still proceed with analysis for testing
        except Exception as e:
            logger.error(f"Failed to decode image: {e}")
            # For testing purposes, create dummy data
            image_bytes = b"dummy_image_data_for_testing"

        # Analyze the frame
        result = analyzer.analyze_frame(image_bytes)

        logger.info(f"Analysis completed: overall={result['overall_score']}")

        return jsonify(result)

    except Exception as e:
        logger.error(f"Analysis failed: {e}")
        logger.error(traceback.format_exc())
        return jsonify({"error": "Analysis failed"}), 500


@app.route("/stats", methods=["GET"])
def get_stats():
    """Get service statistics"""
    return jsonify(
        {
            "total_analyses": analyzer.analysis_count,
            "uptime_seconds": time.time() - analyzer.last_analysis_time,
            "status": "running",
        }
    )


if __name__ == "__main__":
    logger.info("Starting Simple ML Service...")
    logger.info("This is a mock service for testing - no real ML processing")

    # Set analyzer start time
    analyzer.last_analysis_time = time.time()

    # Run the Flask app
    app.run(host="0.0.0.0", port=5000, debug=True, threaded=True)
