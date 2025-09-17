#!/usr/bin/env python3

import logging
import math
from typing import Dict, List, Any, Optional, Tuple
import numpy as np
import cv2

logger = logging.getLogger(__name__)


class RidingAnalyzer:
    def __init__(self):
        self.scoring_weights = {
            "posture": 0.3,
            "lane_position": 0.25,
            "speed": 0.25,
            "safety_margin": 0.2,
        }

        self.ideal_posture = {
            "back_angle": {"min": 70, "max": 90, "ideal": 80},
            "knee_angle": {"min": 110, "max": 140, "ideal": 125},
            "elbow_angle": {"min": 90, "max": 130, "ideal": 110},
            "head_position": {"min": 0.4, "max": 0.6, "ideal": 0.5},
        }

        self.lane_analysis = {
            "optimal_position": 0.5,
            "acceptable_range": 0.15,
            "warning_range": 0.25,
        }

        logger.info("RidingAnalyzer initialized with scoring parameters")

    def analyze_riding_technique(
        self,
        image: np.ndarray,
        detections: List[Dict],
        pose_keypoints: Dict,
        scene_analysis: Dict,
    ) -> Dict[str, Any]:
        try:
            motorcycle_det, rider_det = self._find_motorcycle_and_rider(detections)
            posture_score = self._analyze_posture(pose_keypoints, rider_det)
            lane_score = self._analyze_lane_position(
                image, motorcycle_det, scene_analysis
            )
            speed_score = self._analyze_speed_appropriateness(
                detections, scene_analysis
            )
            safety_score = self._analyze_safety_margins(detections, motorcycle_det)

            overall_score = self._calculate_overall_score(
                posture_score, lane_score, speed_score, safety_score
            )

            analysis_results = {
                "overall_score": int(overall_score),
                "posture_score": int(posture_score),
                "lane_score": int(lane_score),
                "speed_score": int(speed_score),
                "safety_score": int(safety_score),
                "detailed_feedback": self._generate_detailed_feedback(
                    posture_score,
                    lane_score,
                    speed_score,
                    safety_score,
                    motorcycle_det,
                    rider_det,
                    scene_analysis,
                ),
            }

            logger.debug(f"Riding analysis completed: Overall={overall_score:.1f}")
            return analysis_results

        except Exception as e:
            logger.error(f"Riding analysis failed: {e}")
            return {
                "overall_score": 50,
                "posture_score": 50,
                "lane_score": 50,
                "speed_score": 50,
                "safety_score": 50,
                "detailed_feedback": [],
            }

    def _find_motorcycle_and_rider(
        self, detections: List[Dict]
    ) -> Tuple[Optional[Dict], Optional[Dict]]:
        motorcycle_det = None
        rider_det = None

        sorted_detections = sorted(
            detections, key=lambda x: x["confidence"], reverse=True
        )
        for detection in sorted_detections:
            if detection["class_name"] == "motorcycle" and motorcycle_det is None:
                motorcycle_det = detection
            elif detection["class_name"] == "person" and rider_det is None:
                rider_det = detection

            if not motorcycle_det and rider_det:
                break
        return motorcycle_det, rider_det

    def _analyze_posture(
        self, pose_keypoints: Dict, rider_det: Optional[Dict]
    ) -> float:
        if not pose_keypoints or "landmarks" not in pose_keypoints:
            logger.warning("No pose keypoints available for posture analysis")
            return 60.0
        landmarks = pose_keypoints["landmarks"]
        score = 100.0

        try:
            if self._has_keypoints(
                landmarks, ["left_shoulder", "left_hip", "right_shoulder", "right_hip"]
            ):
                back_angle = self._calculate_back_angle(landmarks)
                back_score = self._score_angle(
                    back_angle, self.ideal_posture["back_angle"]
                )
                score *= 0.4 * (back_score / 100.0)

            if self._has_keypoints(landmarks, ["left_knee", "left_hip", "left_ankle"]):
                knee_angle = self._calculate_knee_angle(landmarks)
                knee_score = self._score_angle(
                    knee_angle, self.ideal_posture["knee_angle"]
                )
                score += 0.3 * knee_score

            if self._has_keypoints(
                landmarks, ["left_shoulder", "left_elbow", "left_wrist"]
            ):
                elbow_angle = self._calculate_elbow_angle(landmarks)
                elbow_score = self._score_angle(
                    elbow_angle, self.ideal_posture["elbow_angle"]
                )
                score += 0.2 * elbow_score

            if self._has_keypoints(
                landmarks, ["nose", "left_shoulder", "right_shoulder"]
            ):
                head_score = self._analyze_head_position(landmarks)
                score += 0.1 * head_score

        except Exception as e:
            logger.error(f"Posture analysis error: {e}")
            return 60.0

        return max(0.0, min(100.0, score))

    def _analyze_lane_position(
        self, image: np.ndarray, motorcycle_det: Optional[Dict], scene_analysis: Dict
    ) -> float:
        if not motorcycle_det:
            logger.warning("No motorcycle detected for lane analysis")
            return 70.0

        try:
            bbox = motorcycle_det["bbox"]
            bike_center_x = (bbox[0] + bbox[2]) / 2
            image_width = image.shape[1]

            normalized_position = bike_center_x / image_width

            lane_count = scene_analysis.get("lane_count", 2)
            current_lane = int(normalized_position * lane_count)
            lane_center = (current_lane + 0.5) / lane_count

            deviation = abs(normalized_position - lane_center)

            if deviation <= self.lane_analysis["acceptable_range"]:
                score = (
                    100.0 - (deviation / self.lane_analysis["acceptable_range"]) * 20.0
                )
            elif deviation <= self.lane_analysis["warning_range"]:
                score = (
                    80.0
                    - (
                        (deviation - self.lane_analysis["acceptable_range"])
                        / (
                            self.lane_analysis["warning_range"]
                            - self.lane_analysis["acceptable_range"]
                        )
                    )
                    * 30.0
                )
            else:
                score = 50.0 - min(
                    30.0, (deviation - self.lane_analysis["warning_range"]) * 100.0
                )
            return max(0.0, min(100.0, score))

        except Exception as e:
            logger.error(f"Lane position analysis error: {e}")
            return 70.0

    def _analyze_speed_appropriateness(
        self, detections: List[Dict], scene_analysis: Dict
    ) -> float:
        try:
            weather = scene_analysis.get("weather_condition", "clear")
            road_condtion = scene_analysis.get("road_condition", "dry")
            traffic_density = scene_analysis.get("traffic_density", "light")
            time_of_day = scene_analysis.get("time_of_day", "day")

            base_score = 80.0

            if weather in ["rain", "snow", "fog"]:
                base_score -= 10.0
            if road_condtion in ["wet", "icy"]:
                base_score -= 15.0

            if traffic_density == "heavy":
                base_score -= 10.0
            elif traffic_density == "congested":
                base_score -= 20.0

            if time_of_day in ["dawn", "dusk", "night"]:
                base_score -= 5.0

            vehicle_count = len(
                [d for d in detections if d["class_name"] in ["car", "truck", "bus"]]
            )
            if vehicle_count > 3:
                base_score -= min(15.0, vehicle_count * 2.0)

            return max(20.0, min(100.0, base_score))

        except Exception as e:
            logger.error(f"Speed analysis error: {e}")
            return 75.0

    def _analyze_safety_margins(
        self, detections: List[Dict], motorcycle_det: Optional[Dict]
    ) -> float:
        if not motorcycle_det:
            return 70.0

        try:
            bike_bbox = motorcycle_det["bbox"]
            bike_center = [
                (bike_bbox[0] + bike_bbox[2]) / 2,
                (bike_bbox[1] + bike_bbox[3]) / 2,
            ]
            bike_area = (bike_bbox[2] - bike_bbox[0]) * (bike_bbox[3] - bike_bbox[1])

            nearby_vehicles = []
            for detection in detections:
                if (
                    detection["class_name"] in ["car", "truck", "bus"]
                    and detection != motorcycle_det
                ):
                    distance = self._calculate_bbox_distance(
                        bike_bbox, detection["bbox"]
                    )
                    relative_size = (
                        detection["area"] / bike_area if bike_area > 0 else 0.0
                    )

                    nearby_vehicles.append(
                        {
                            "distance": distance,
                            "size_ratio": relative_size,
                            "detection": detection,
                        }
                    )

            nearby_vehicles.sort(key=lambda x: x["distance"])
            base_score = 100.0

            for i, vehicle in enumerate(nearby_vehicles[:3]):
                distance = vehicle["distance"]
                size_ratio = vehicle["size_ratio"]
                min_safe_distance = 50.0 + (size_ratio * 30.0)

                if distance < min_safe_distance:
                    penalty = (1.0 - distance / min_safe_distance) * 40.0
                    base_score -= penalty / (i + 1)

            if len(nearby_vehicles) == 0:
                base_score += 10.0
            elif len(nearby_vehicles) > 0 and nearby_vehicles[0]["distance"] > 100:
                base_score += 5.0

            return max(0.0, min(100.0, base_score))

        except Exception as e:
            logger.error(f"Safety margin analysis error: {e}")
            return 70.0

    def _calculate_overall_score(
        self, posture: float, lane: float, speed: float, safety: float
    ) -> float:
        overall = (
            self.scoring_weights["posture"] * posture
            + self.scoring_weights["lane_position"] * lane
            + self.scoring_weights["speed"] * speed
            + self.scoring_weights["safety_margin"] * safety
        )
        return max(0.0, min(100.0, overall))

    def _generate_detailed_feedback(
        self,
        posture_score: float,
        lane_score: float,
        speed_score: float,
        safety_score: float,
        motorcycle_det: Optional[Dict],
        rider_det: Optional[Dict],
        scene_analysis: Dict,
    ) -> List[Dict[str, Any]]:
        feedback = []

        if posture_score < 60:
            feedback.append(
                {
                    "category": "posture",
                    "type": "warning",
                    "message": "Adjust your riding posture - keep back straight and relax shoulders",
                    "score": posture_score,
                }
            )
        elif posture_score > 85:
            feedback.append(
                {
                    "category": "posture",
                    "type": "success",
                    "message": "Excellent riding posture",
                    "score": posture_score,
                }
            )

        if lane_score < 70:
            feedback.append(
                {
                    "category": "lane_position",
                    "type": "warning",
                    "message": "Maintain better lane position - center yourself in the lane",
                    "score": lane_score,
                }
            )

        if speed_score < 70:
            weather = scene_analysis.get("weather_condition", "clear")
            if weather in ["rain", "snow"]:
                feedback.append(
                    {
                        "category": "speed",
                        "type": "warning",
                        "message": f"Reduce speed for {weather} conditions",
                        "score": speed_score,
                    }
                )
            else:
                feedback.append(
                    {
                        "category": "speed",
                        "type": "info",
                        "message": "Adjust speed for current traffic conditions",
                        "score": speed_score,
                    }
                )

        if safety_score < 60:
            feedback.append(
                {
                    "category": "safety",
                    "type": "error",
                    "message": "Increase following distance - maintain safer margins",
                    "score": safety_score,
                }
            )

        return feedback

    def _has_keypoints(self, landmarks: Dict, required_points: List[str]) -> bool:
        for point in required_points:
            if point not in landmarks or not landmarks[point].get("visible", False):
                return False
        return True

    def _calculate_back_angle(self, landmarks: Dict) -> float:
        try:
            left_shoulder = landmarks["left_shoulder"]
            right_shoulder = landmarks["right_shoulder"]
            left_hip = landmarks["left_hip"]
            right_hip = landmarks["right_hip"]

            shoulder_center = [
                (left_shoulder["x"] + right_shoulder["x"]) / 2,
                (left_shoulder["y"] + right_shoulder["y"]) / 2,
            ]
            hip_center = [
                (left_hip["x"] + right_hip["x"]) / 2,
                (left_hip["y"] + right_hip["y"]) / 2,
            ]

            dx = hip_center[0] - shoulder_center[0]
            dy = hip_center[1] - shoulder_center[1]
            angle = math.degrees(math.atan2(dx, dy))

            return abs(90 - abs(angle))

        except Exception as e:
            logger.error(f"Back angle calculation error: {e}")
            return 80.0

    def _calculate_knee_angle(self, landmarks: Dict) -> float:
        try:
            hip = landmarks["left_hip"]
            knee = landmarks["left_knee"]
            ankle = landmarks["left_ankle"]

            v1 = [hip["x"] - knee["x"], hip["y"] - knee["y"]]
            v2 = [ankle["x"] - knee["x"], ankle["y"] - knee["y"]]

            dot_product = v1[0] * v2[0] + v1[1] * v2[1]
            mag_v1 = math.sqrt(v1[0] ** 2 + v1[1] ** 2)
            mag_v2 = math.sqrt(v2[0] ** 2 + v2[1] ** 2)

            if mag_v1 * mag_v2 == 0:
                return 125.0

            cos_angle = dot_product / (mag_v1 * mag_v2)
            cos_angle = max(-1.0, min(1.0, cos_angle))
            angle = math.degrees(math.acos(cos_angle))

            return angle

        except Exception as e:
            logger.error(f"Knee angle calculation error: {e}")
            return 125.0

    def _calculate_elbow_angle(self, landmarks: Dict) -> float:
        try:
            shoulder = landmarks["left_shoulder"]
            elbow = landmarks["left_elbow"]
            wrist = landmarks["left_wrist"]

            v1 = [shoulder["x"] - elbow["x"], shoulder["y"] - elbow["y"]]
            v2 = [wrist["x"] - elbow["x"], wrist["y"] - elbow["y"]]

            dot_product = v1[0] * v2[0] + v1[1] * v2[1]
            mag_v1 = math.sqrt(v1[0] ** 2 + v1[1] ** 2)
            mag_v2 = math.sqrt(v2[0] ** 2 + v2[1] ** 2)

            if mag_v1 * mag_v2 == 0:
                return 110.0

            cos_angle = dot_product / (mag_v1 * mag_v2)
            cos_angle = max(-1.0, min(1.0, cos_angle))
            angle = math.degrees(math.acos(cos_angle))

            return angle

        except Exception as e:
            logger.error(f"Elbow angle calculation error: {e}")
            return 110.0

    def _analyze_head_position(self, landmarks: Dict) -> float:
        try:
            nose = landmarks["nose"]
            left_shoulder = landmarks["left_shoulder"]
            right_shoulder = landmarks["right_shoulder"]

            shoulder_center_x = (left_shoulder["x"] + right_shoulder["x"]) / 2
            shoulder_center_y = (left_shoulder["y"] + right_shoulder["y"]) / 2

            horizontal_offset = abs(nose["x"] - shoulder_center_x)
            vertical_position = (
                nose["y"] / shoulder_center_y if shoulder_center_y > 0 else 1.0
            )

            alignment_score = max(0, 100 - horizontal_offset * 500)
            position_score = 100 if 0.7 <= vertical_position <= 0.9 else 80

            return (alignment_score + position_score) / 2

        except Exception as e:
            logger.error(f"Head position analysis error: {e}")
            return 80.0

    def _score_angle(
        self, actual_angle: float, ideal_params: Dict[str, float]
    ) -> float:
        ideal = ideal_params["ideal"]
        min_acceptable = ideal_params["min"]
        max_acceptable = ideal_params["max"]

        if min_acceptable <= actual_angle <= max_acceptable:
            deviation = abs(actual_angle - ideal)
            max_deviation = max(ideal - min_acceptable, max_acceptable - ideal)
            return 100.0 - (deviation / max_deviation) * 20.0
        else:
            if actual_angle < min_acceptable:
                penalty = (min_acceptable - actual_angle) / min_acceptable * 50.0
            else:
                penalty = (actual_angle - max_acceptable) / max_acceptable * 50.0

            return max(0.0, 80.0 - penalty)

    def _calculate_bbox_distance(self, bbox1: List[float], bbox2: List[float]) -> float:
        center1 = [(bbox1[0] + bbox1[2]) / 2, (bbox1[1] + bbox1[3]) / 2]
        center2 = [(bbox2[0] + bbox2[2]) / 2, (bbox2[1] + bbox2[3]) / 2]

        return math.sqrt(
            (center1[0] - center2[0]) ** 2 + (center1[1] - center2[1]) ** 2
        )
