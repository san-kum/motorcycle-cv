#!/usr/bin/env python3

import logging
from typing import Dict, List, Any, Optional, Tuple
import numpy as np
import cv2
import torch
from ultralytics import YOLO

logger = logging.getLogger(__name__)


class ObjectDetector:

    def __init__(
        self,
        model_path: str = "yolov8n.pt",
        device: str = "cpu",
        confidence_threshold: float = 0.5,
    ):
        self.device = device
        self.confidence_threshold = confidence_threshold
        self.model = None
        self.class_mapping = self._get_class_mapping()

        self.target_classes = {
            "motorcycle",
            "bicycle",
            "car",
            "truck",
            "bus",
            "person",
            "traffic light",
            "stop sign",
            "speed limit",
            "crosswalk",
        }

        self._load_model(model_path)
        logger.info(f"ObjectDetector initialized with {model_path} on {device}")

    def _load_model(self, model_path: str):
        try:
            self.model = YOLO(model_path)
            if torch.cuda.is_available() and self.device == "cuda":
                self.model.to(self.device)

            dummy_input = np.zeros((640, 640, 3), dtype=np.uint8)
            _ = self.model.predict(dummy_input, verbose=False)

            logger.info("YOLOv8 model loaded and warmed up successfully")

        except Exception as e:
            logger.error(f"Failed to load YOLOv8 model: {e}")
            raise

    def _get_class_mapping(self) -> Dict[int, str]:
        return {
            0: "person",
            1: "bicycle",
            2: "car",
            3: "motorcycle",
            4: "airplane",
            5: "bus",
            6: "train",
            7: "truck",
            8: "boat",
            9: "traffic light",
            10: "fire hydrant",
            11: "stop sign",
            12: "parking meter",
            13: "bench",
            14: "bird",
            15: "cat",
            16: "dog",
            17: "horse",
            18: "sheep",
            19: "cow",
            20: "elephant",
            21: "bear",
            22: "zebra",
            23: "giraffe",
            24: "backpack",
            25: "umbrella",
            26: "handbag",
            27: "tie",
            28: "suitcase",
            29: "frisbee",
        }

    def detect_objects(self, image: np.ndarray) -> List[Dict[str, Any]]:
        if self.model is None:
            logger.error("Model not loaded")
            return []

        try:
            results = self.model.predict(
                image, conf=self.confidence_threshold, verbose=False, device=self.device
            )

            detections = []
            if results and len(results) > 0:
                result = results[0]

                if result.boxes is not None:
                    boxes = result.boxes.cpu().numpy()

                    for i, box in enumerate(boxes.data):
                        x1, y1, x2, y2, conf, class_id = box
                        class_id = int(class_id)
                        class_name = self.class_mapping.get(
                            class_id, f"class_{class_id}"
                        )

                        if class_name in self.target_classes or any(
                            target in class_name.lower()
                            for target in self.target_classes
                        ):
                            detection = {
                                "bbox": [float(x1), float(y1), float(x2), float(y2)],
                                "confidence": float(conf),
                                "class_id": class_id,
                                "class_name": class_name,
                                "area": float((x2 - x1) * (y2 - y1)),
                                "center": [float((x1 + x2) / 2), float((y1 + y2) / 2)],
                            }
                            detections.append(detection)

            detections.sort(key=lambda x: x["confidence"], reverse=True)

            logger.debug(f"Detected {len(detections)} relevant objects")
            return detections

        except Exception as e:
            logger.error(f"Object detection failed: {e}")
            return []

    def detect_motorcycle_and_rider(
        self, image: np.ndarray
    ) -> Tuple[Optional[Dict], Optional[Dict]]:
        all_detections = self.detect_objects(image)

        motorcycle_detection = None
        rider_detection = None

        for detection in all_detections:
            if detection["class_name"] == "motorcycle" and motorcycle_detection is None:
                motorcycle_detection = detection
            elif detection["class_name"] == "person" and rider_detection is None:
                rider_detection = detection

            if motorcycle_detection and rider_detection:
                break

        return motorcycle_detection, rider_detection

    def analyze_traffic_density(self, detections: List[Dict]) -> Dict[str, Any]:
        vehicle_classes = {"car", "truck", "bus", "motorcycle", "bicycle"}
        vehicles = [d for d in detections if d["class_name"] in vehicle_classes]

        total_vehicles = len(vehicles)
        vehicle_distribution = {}

        for vehicle in vehicles:
            class_name = vehicle["class_name"]
            vehicle_distribution[class_name] = (
                vehicle_distribution.get(class_name, 0) + 1
            )

        image_area = 640 * 640
        vehicle_areas = sum(d["area"] for d in vehicles)
        density_ratio = vehicle_areas / image_area if image_area > 0 else 0

        if total_vehicles == 0:
            density_level = "no_traffic"
        elif total_vehicles <= 2 and density_ratio < 0.1:
            density_level = "light"
        elif total_vehicles <= 5 and density_ratio < 0.25:
            density_level = "moderate"
        elif total_vehicles <= 10 and density_ratio < 0.4:
            density_level = "heavy"
        else:
            density_level = "congested"

        return {
            "total_vehicles": total_vehicles,
            "vehicle_distribution": vehicle_distribution,
            "density_level": density_level,
            "density_ratio": density_ratio,
            "nearest_vehicle_distance": self._estimate_nearest_vehicle_distance(
                vehicles
            ),
        }

    def _estimate_nearest_vehicle_distance(self, vehicles: List[Dict]) -> float:
        if not vehicles:
            return float("inf")

        largest_area = max(vehicle["area"] for vehicle in vehicles)

        estimated_distance = max(
            10.0, 100.0 / (largest_area / 10000.0)
        )  # Rough approximation

        return min(estimated_distance, 200.0)  # Cap at 200 meters

    def detect_road_infrastructure(self, image: np.ndarray) -> Dict[str, List[Dict]]:
        detections = self.detect_objects(image)

        infrastructure = {
            "traffic_lights": [],
            "road_signs": [],
            "crosswalks": [],
            "other": [],
        }

        for detection in detections:
            class_name = detection["class_name"]

            if "traffic light" in class_name:
                infrastructure["traffic_lights"].append(detection)
            elif any(
                sign_type in class_name.lower()
                for sign_type in ["stop sign", "speed limit", "yield"]
            ):
                infrastructure["road_signs"].append(detection)
            elif "crosswalk" in class_name:
                infrastructure["crosswalks"].append(detection)
            elif class_name not in {
                "person",
                "car",
                "truck",
                "bus",
                "motorcycle",
                "bicycle",
            }:
                infrastructure["other"].append(detection)

        return infrastructure

    def update_confidence_threshold(self, new_threshold: float):
        if 0.0 <= new_threshold <= 1.0:
            self.confidence_threshold = new_threshold
            logger.info(f"Updated confidence threshold to {new_threshold}")
        else:
            logger.warning(f"Invalid confidence threshold: {new_threshold}")

    def get_model_info(self) -> Dict[str, Any]:
        return {
            "model_type": "YOLOv8",
            "device": self.device,
            "confidence_threshold": self.confidence_threshold,
            "target_classes": list(self.target_classes),
            "total_classes": len(self.class_mapping),
        }
