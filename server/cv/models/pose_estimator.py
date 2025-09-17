#!/usr/bin/env python3

import logging
from typing import Dict, List, Any, Optional, Tuple
import numpy as np
import cv2
import mediapipe as mp

logger = logging.getLogger(__name__)


class PoseEstimator:

    def __init__(self, device: str = "cpu"):
        self.device = device
        self.mp_pose = mp.solutions.pose
        self.mp_drawing = mp.solutions.drawing_utils

        self.pose = self.mp_pose.Pose(
            static_image_mode=False,
            model_complexity=1,
            enable_segmentation=False,
            min_detection_confidence=0.7,
            min_tracking_confidence=0.5,
        )

        self.keypoint_mapping = {
            "nose": 0,
            "left_eye_inner": 1,
            "left_eye": 2,
            "left_eye_outer": 3,
            "right_eye_inner": 4,
            "right_eye": 5,
            "right_eye_outer": 6,
            "left_ear": 7,
            "right_ear": 8,
            "mouth_left": 9,
            "mouth_right": 10,
            "left_shoulder": 11,
            "right_shoulder": 12,
            "left_elbow": 13,
            "right_elbow": 14,
            "left_wrist": 15,
            "right_wrist": 16,
            "left_pinky": 17,
            "right_pinky": 18,
            "left_index": 19,
            "right_index": 20,
            "left_thumb": 21,
            "right_thumb": 22,
            "left_hip": 23,
            "right_hip": 24,
            "left_knee": 25,
            "right_knee": 26,
            "left_ankle": 27,
            "right_ankle": 28,
            "left_heel": 29,
            "right_heel": 30,
            "left_foot_index": 31,
            "right_foot_index": 32,
        }

        self.riding_keypoints = [
            "nose",
            "left_shoulder",
            "right_shoulder",
            "left_elbow",
            "right_elbow",
            "left_wrist",
            "right_wrist",
            "left_hip",
            "right_hip",
            "left_knee",
            "right_knee",
            "left_ankle",
            "right_ankle",
        ]

        logger.info("PoseEstimator initialized with MediaPipe")

    def estimate_pose(self, image: np.ndarray) -> Dict[str, Any]:
        if image is None:
            logger.error("Input image is None")
            return {}

        try:
            rgb_image = cv2.cvtColor(image, cv2.COLOR_BGR2RGB)

            results = self.pose.process(rgb_image)

            if results.pose_landmarks is None:
                logger.debug("No pose landmarks detected")
                return {"landmarks": {}, "confidence": 0.0}

            landmarks = self._extract_landmarks(results.pose_landmarks, image.shape)

            pose_quality = self._assess_pose_quality(landmarks)

            riding_landmarks = {
                k: v for k, v in landmarks.items() if k in self.riding_keypoints
            }

            return {
                "landmarks": riding_landmarks,
                "all_landmarks": landmarks,
                "confidence": pose_quality["overall_confidence"],
                "visibility_score": pose_quality["visibility_score"],
                "pose_quality": pose_quality,
                "detected_keypoints": len(
                    [k for k, v in riding_landmarks.items() if v["visible"]]
                ),
            }

        except Exception as e:
            logger.error(f"Pose estimation failed: {e}")
            return {"landmarks": {}, "confidence": 0.0}

    def _extract_landmarks(
        self, pose_landmarks, image_shape: Tuple[int, int, int]
    ) -> Dict[str, Dict]:
        height, width = image_shape[:2]
        landmarks = {}

        for name, idx in self.keypoint_mapping.items():
            if idx < len(pose_landmarks.landmark):
                landmark = pose_landmarks.landmark[idx]

                x_pixel = int(landmark.x * width)
                y_pixel = int(landmark.y * height)

                landmarks[name] = {
                    "x": landmark.x,
                    "y": landmark.y,
                    "z": landmark.z,
                    "x_pixel": x_pixel,
                    "y_pixel": y_pixel,
                    "visibility": landmark.visibility,
                    "confidence": landmark.visibility,
                    "visible": landmark.visibility > 0.5,
                }

        return landmarks

    def _assess_pose_quality(self, landmarks: Dict[str, Dict]) -> Dict[str, float]:
        if not landmarks:
            return {
                "overall_confidence": 0.0,
                "visibility_score": 0.0,
                "completeness_score": 0.0,
                "stability_score": 0.0,
            }

        visible_landmarks = [lm for lm in landmarks.values() if lm["visible"]]
        visibility_score = len(visible_landmarks) / len(landmarks) if landmarks else 0.0

        confidences = [lm["confidence"] for lm in landmarks.values() if lm["visible"]]
        avg_confidence = sum(confidences) / len(confidences) if confidences else 0.0

        riding_visible = sum(
            1
            for name in self.riding_keypoints
            if name in landmarks and landmarks[name]["visible"]
        )
        completeness_score = riding_visible / len(self.riding_keypoints)

        overall_confidence = (
            0.4 * avg_confidence + 0.3 * visibility_score + 0.3 * completeness_score
        )

        return {
            "overall_confidence": overall_confidence,
            "visibility_score": visibility_score,
            "completeness_score": completeness_score,
            "stability_score": avg_confidence,
            "detected_keypoints": len(visible_landmarks),
            "total_keypoints": len(landmarks),
        }

    def analyze_riding_posture(self, landmarks: Dict[str, Dict]) -> Dict[str, Any]:
        if not landmarks:
            return {"posture_score": 50.0, "issues": []}

        analysis_results = {"posture_score": 100.0, "issues": [], "measurements": {}}

        try:
            back_analysis = self._analyze_back_posture(landmarks)
            analysis_results["measurements"]["back_angle"] = back_analysis["angle"]
            if back_analysis["score"] < 70:
                analysis_results["issues"].append("Straighten your back")
                analysis_results["posture_score"] -= 15

            shoulder_analysis = self._analyze_shoulder_position(landmarks)
            analysis_results["measurements"]["shoulder_level"] = shoulder_analysis[
                "level_difference"
            ]
            if shoulder_analysis["score"] < 70:
                analysis_results["issues"].append("Level your shoulders")
                analysis_results["posture_score"] -= 10

            arm_analysis = self._analyze_arm_position(landmarks)
            analysis_results["measurements"]["elbow_angles"] = arm_analysis["angles"]
            if arm_analysis["score"] < 70:
                analysis_results["issues"].append("Relax your arms and elbows")
                analysis_results["posture_score"] -= 15

            knee_analysis = self._analyze_knee_position(landmarks)
            analysis_results["measurements"]["knee_angles"] = knee_analysis["angles"]
            if knee_analysis["score"] < 70:
                analysis_results["issues"].append("Adjust knee position on tank")
                analysis_results["posture_score"] -= 10

        except Exception as e:
            logger.error(f"Posture analysis error: {e}")
            analysis_results["posture_score"] = 60.0
            analysis_results["issues"] = ["Unable to fully analyze posture"]

        analysis_results["posture_score"] = max(
            0.0, min(100.0, analysis_results["posture_score"])
        )
        return analysis_results

    def _analyze_back_posture(self, landmarks: Dict) -> Dict[str, float]:
        try:
            if not all(
                k in landmarks
                for k in ["left_shoulder", "right_shoulder", "left_hip", "right_hip"]
            ):
                return {"angle": 0.0, "score": 60.0}

            shoulder_center_y = (
                landmarks["left_shoulder"]["y"] + landmarks["right_shoulder"]["y"]
            ) / 2
            hip_center_y = (
                landmarks["left_hip"]["y"] + landmarks["right_hip"]["y"]
            ) / 2
            shoulder_center_x = (
                landmarks["left_shoulder"]["x"] + landmarks["right_shoulder"]["x"]
            ) / 2
            hip_center_x = (
                landmarks["left_hip"]["x"] + landmarks["right_hip"]["x"]
            ) / 2

            dx = shoulder_center_x - hip_center_x
            dy = shoulder_center_y - hip_center_y

            if dy == 0:
                angle = 90.0
            else:
                angle = abs(np.degrees(np.arctan(dx / dy)))

            if angle <= 15:
                score = 100.0
            elif angle <= 30:
                score = 80.0 - (angle - 15) * 2.0
            else:
                score = max(40.0, 80.0 - (angle - 15) * 3.0)

            return {"angle": angle, "score": score}

        except Exception as e:
            logger.error(f"Back posture analysis error: {e}")
            return {"angle": 0.0, "score": 60.0}

    def _analyze_shoulder_position(self, landmarks: Dict) -> Dict[str, Any]:
        try:
            if not all(k in landmarks for k in ["left_shoulder", "right_shoulder"]):
                return {"level_difference": 0.0, "score": 60.0}

            left_y = landmarks["left_shoulder"]["y"]
            right_y = landmarks["right_shoulder"]["y"]

            level_difference = abs(left_y - right_y)

            if level_difference <= 0.02:
                score = 100.0
            elif level_difference <= 0.05:
                score = 80.0 - (level_difference - 0.02) * 1000
            else:
                score = max(40.0, 80.0 - (level_difference - 0.02) * 800)

            return {"level_difference": level_difference, "score": score}

        except Exception as e:
            logger.error(f"Shoulder analysis error: {e}")
            return {"level_difference": 0.0, "score": 60.0}

    def _analyze_arm_position(self, landmarks: Dict) -> Dict[str, Any]:
        arm_data = {"angles": {}, "score": 100.0}

        try:
            if all(
                k in landmarks for k in ["left_shoulder", "left_elbow", "left_wrist"]
            ):
                left_angle = self._calculate_elbow_angle(
                    landmarks["left_shoulder"],
                    landmarks["left_elbow"],
                    landmarks["left_wrist"],
                )
                arm_data["angles"]["left_elbow"] = left_angle

                if 90 <= left_angle <= 120:
                    left_score = 100.0
                elif 80 <= left_angle < 90 or 120 < left_angle <= 130:
                    left_score = 80.0
                else:
                    left_score = max(40.0, 80.0 - abs(left_angle - 105) * 2)

                arm_data["score"] *= 0.5 + 0.5 * (left_score / 100.0)

            if all(
                k in landmarks for k in ["right_shoulder", "right_elbow", "right_wrist"]
            ):
                right_angle = self._calculate_elbow_angle(
                    landmarks["right_shoulder"],
                    landmarks["right_elbow"],
                    landmarks["right_wrist"],
                )
                arm_data["angles"]["right_elbow"] = right_angle

                if 90 <= right_angle <= 120:
                    right_score = 100.0
                elif 80 <= right_angle < 90 or 120 < right_angle <= 130:
                    right_score = 80.0
                else:
                    right_score = max(40.0, 80.0 - abs(right_angle - 105) * 2)

                arm_data["score"] *= 0.5 + 0.5 * (right_score / 100.0)

        except Exception as e:
            logger.error(f"Arm analysis error: {e}")
            arm_data["score"] = 60.0

        return arm_data

    def _analyze_knee_position(self, landmarks: Dict) -> Dict[str, Any]:
        knee_data = {"angles": {}, "score": 100.0}

        try:
            if all(k in landmarks for k in ["left_hip", "left_knee", "left_ankle"]):
                left_angle = self._calculate_knee_angle(
                    landmarks["left_hip"],
                    landmarks["left_knee"],
                    landmarks["left_ankle"],
                )
                knee_data["angles"]["left_knee"] = left_angle

                if 110 <= left_angle <= 140:
                    left_score = 100.0
                elif 100 <= left_angle < 110 or 140 < left_angle <= 150:
                    left_score = 80.0
                else:
                    left_score = max(40.0, 80.0 - abs(left_angle - 125) * 1.5)

                knee_data["score"] *= 0.5 + 0.5 * (left_score / 100.0)

            if all(k in landmarks for k in ["right_hip", "right_knee", "right_ankle"]):
                right_angle = self._calculate_knee_angle(
                    landmarks["right_hip"],
                    landmarks["right_knee"],
                    landmarks["right_ankle"],
                )
                knee_data["angles"]["right_knee"] = right_angle

                if 110 <= right_angle <= 140:
                    right_score = 100.0
                elif 100 <= right_angle < 110 or 140 < right_angle <= 150:
                    right_score = 80.0
                else:
                    right_score = max(40.0, 80.0 - abs(right_angle - 125) * 1.5)

                knee_data["score"] *= 0.5 + 0.5 * (right_score / 100.0)

        except Exception as e:
            logger.error(f"Knee analysis error: {e}")
            knee_data["score"] = 60.0

        return knee_data

    def _calculate_elbow_angle(self, shoulder: Dict, elbow: Dict, wrist: Dict) -> float:
        try:
            v1 = np.array([shoulder["x"] - elbow["x"], shoulder["y"] - elbow["y"]])
            v2 = np.array([wrist["x"] - elbow["x"], wrist["y"] - elbow["y"]])

            cos_angle = np.dot(v1, v2) / (np.linalg.norm(v1) * np.linalg.norm(v2))
            cos_angle = np.clip(cos_angle, -1.0, 1.0)
            angle = np.degrees(np.arccos(cos_angle))

            return angle

        except Exception as e:
            logger.error(f"Elbow angle calculation error: {e}")
            return 105.0

    def _calculate_knee_angle(self, hip: Dict, knee: Dict, ankle: Dict) -> float:
        try:
            v1 = np.array([hip["x"] - knee["x"], hip["y"] - knee["y"]])
            v2 = np.array([ankle["x"] - knee["x"], ankle["y"] - knee["y"]])

            cos_angle = np.dot(v1, v2) / (np.linalg.norm(v1) * np.linalg.norm(v2))
            cos_angle = np.clip(cos_angle, -1.0, 1.0)
            angle = np.degrees(np.arccos(cos_angle))

            return angle

        except Exception as e:
            logger.error(f"Knee angle calculation error: {e}")
            return 125.0

    def get_pose_overlay(
        self, image: np.ndarray, landmarks: Dict[str, Dict]
    ) -> np.ndarray:
        if not landmarks:
            return image.copy()

        overlay_image = image.copy()
        height, width = image.shape[:2]

        for name, landmark in landmarks.items():
            if landmark["visible"]:
                x = int(landmark["x"] * width)
                y = int(landmark["y"] * height)

                if "shoulder" in name or "hip" in name:
                    color = (0, 255, 0)
                elif "elbow" in name or "knee" in name:
                    color = (255, 255, 0)
                elif "wrist" in name or "ankle" in name:
                    color = (255, 0, 0)
                else:
                    color = (0, 0, 255)

                cv2.circle(overlay_image, (x, y), 4, color, -1)
                cv2.putText(
                    overlay_image,
                    name[:3],
                    (x - 10, y - 10),
                    cv2.FONT_HERSHEY_SIMPLEX,
                    0.3,
                    color,
                    1,
                )

        connections = [
            ("left_shoulder", "right_shoulder"),
            ("left_shoulder", "left_elbow"),
            ("left_elbow", "left_wrist"),
            ("right_shoulder", "right_elbow"),
            ("right_elbow", "right_wrist"),
            ("left_shoulder", "left_hip"),
            ("right_shoulder", "right_hip"),
            ("left_hip", "right_hip"),
            ("left_hip", "left_knee"),
            ("left_knee", "left_ankle"),
            ("right_hip", "right_knee"),
            ("right_knee", "right_ankle"),
        ]

        for start_point, end_point in connections:
            if (
                start_point in landmarks
                and end_point in landmarks
                and landmarks[start_point]["visible"]
                and landmarks[end_point]["visible"]
            ):

                start_x = int(landmarks[start_point]["x"] * width)
                start_y = int(landmarks[start_point]["y"] * height)
                end_x = int(landmarks[end_point]["x"] * width)
                end_y = int(landmarks[end_point]["y"] * height)

                cv2.line(
                    overlay_image,
                    (start_x, start_y),
                    (end_x, end_y),
                    (255, 255, 255),
                    2,
                )

        return overlay_image

    def cleanup(self):
        """Clean up MediaPipe resources"""
        if hasattr(self, "pose"):
            self.pose.close()
        logger.info("PoseEstimator resources cleaned up")
