#!/usr/bin/env python3

import logging
from typing import Dict, List, Any, Optional, Tuple
import numpy as np
import cv2
from sklearn.cluster import KMeans
import torch
import torch.nn.functional as F

logger = logging.getLogger(__name__)


class SceneAnalyzer:

    def __init__(self, device: str = "cpu"):
        self.device = device

        # Color ranges for different weather/road conditions (HSV)
        self.condition_colors = {
            "wet_road": [(0, 0, 0), (180, 50, 100)],  # Dark, low saturation
            "dry_road": [(0, 0, 100), (180, 30, 255)],  # Brighter surfaces
            "snow": [(0, 0, 200), (30, 30, 255)],  # White/light colors
            "fog": [(0, 0, 180), (30, 50, 230)],  # Gray/white hazy
        }

        # Time of day classification thresholds
        self.time_thresholds = {
            "night": (0, 80),  # Very dark
            "dawn_dusk": (80, 150),  # Low light
            "day": (150, 255),  # Bright
        }

        logger.info("SceneAnalyzer initialized")

    def analyze_scene(self, image: np.ndarray) -> Dict[str, Any]:
        """
        Perform comprehensive scene analysis

        Args:
            image: Input video frame (BGR format)

        Returns:
            Dictionary with scene analysis results
        """
        if image is None:
            logger.error("Input image is None")
            return self._get_default_scene_analysis()

        try:
            # Analyze different aspects of the scene
            weather_analysis = self._analyze_weather_conditions(image)
            road_analysis = self._analyze_road_conditions(image)
            traffic_analysis = self._analyze_traffic_context(image)
            time_analysis = self._analyze_time_of_day(image)
            visibility_analysis = self._analyze_visibility(image)

            # Combine all analyses
            scene_data = {
                "weather_condition": weather_analysis["condition"],
                "weather_confidence": weather_analysis["confidence"],
                "road_condition": road_analysis["condition"],
                "road_type": road_analysis["type"],
                "lane_count": road_analysis["estimated_lanes"],
                "time_of_day": time_analysis["period"],
                "brightness_level": time_analysis["brightness"],
                "traffic_density": traffic_analysis["density"],
                "visibility": visibility_analysis["score"],
                "visibility_category": visibility_analysis["category"],
                "safety_factors": self._assess_safety_factors(
                    weather_analysis,
                    road_analysis,
                    traffic_analysis,
                    time_analysis,
                    visibility_analysis,
                ),
                "recommended_adjustments": self._generate_recommendations(
                    weather_analysis, road_analysis, time_analysis, visibility_analysis
                ),
            }

            logger.debug(
                f"Scene analysis completed: {weather_analysis['condition']}, "
                f"{road_analysis['condition']}, {time_analysis['period']}"
            )

            return scene_data

        except Exception as e:
            logger.error(f"Scene analysis failed: {e}")
            return self._get_default_scene_analysis()

    def _analyze_weather_conditions(self, image: np.ndarray) -> Dict[str, Any]:
        """Analyze weather conditions from image characteristics"""
        try:
            # Convert to HSV for better color analysis
            hsv = cv2.cvtColor(image, cv2.COLOR_BGR2HSV)

            # Analyze overall brightness and contrast
            gray = cv2.cvtColor(image, cv2.COLOR_BGR2GRAY)
            mean_brightness = np.mean(gray)
            contrast = np.std(gray)

            # Analyze color distribution
            hist_h = cv2.calcHist([hsv], [0], None, [180], [0, 180])
            hist_s = cv2.calcHist([hsv], [1], None, [256], [0, 256])
            hist_v = cv2.calcHist([hsv], [2], None, [256], [0, 256])

            # Check for weather indicators
            weather_scores = {
                "clear": 0.0,
                "cloudy": 0.0,
                "rain": 0.0,
                "fog": 0.0,
                "snow": 0.0,
            }

            # Clear weather: high contrast, good brightness
            if contrast > 40 and mean_brightness > 100:
                weather_scores["clear"] = min(
                    100.0, contrast + (mean_brightness - 100) * 0.5
                )

            # Cloudy: reduced contrast, medium brightness
            if 20 < contrast < 50 and 80 < mean_brightness < 150:
                weather_scores["cloudy"] = 70.0 + (50 - contrast) * 0.6

            # Rain indicators: dark areas, low contrast
            if contrast < 30 and mean_brightness < 120:
                dark_ratio = np.sum(gray < 80) / gray.size
                weather_scores["rain"] = min(90.0, dark_ratio * 100 + (30 - contrast))

            # Fog indicators: low contrast, washed out colors
            low_sat_ratio = np.sum(hsv[:, :, 1] < 50) / hsv.size
            if low_sat_ratio > 0.6 and contrast < 25:
                weather_scores["fog"] = min(
                    95.0, low_sat_ratio * 80 + (25 - contrast) * 2
                )

            # Snow indicators: high brightness, low saturation
            if mean_brightness > 180 and low_sat_ratio > 0.7:
                weather_scores["snow"] = min(
                    90.0, (mean_brightness - 100) * 0.8 + low_sat_ratio * 50
                )

            # Determine most likely condition
            best_condition = max(weather_scores.items(), key=lambda x: x[1])

            return {
                "condition": best_condition[0],
                "confidence": best_condition[1] / 100.0,
                "scores": weather_scores,
                "brightness": mean_brightness,
                "contrast": contrast,
            }

        except Exception as e:
            logger.error(f"Weather analysis error: {e}")
            return {
                "condition": "clear",
                "confidence": 0.5,
                "scores": {},
                "brightness": 128,
                "contrast": 30,
            }

    def _analyze_road_conditions(self, image: np.ndarray) -> Dict[str, Any]:
        """Analyze road surface and infrastructure conditions"""
        try:
            # Focus on lower portion of image where road typically is
            height = image.shape[0]
            road_region = image[int(height * 0.6) :, :]

            # Convert to different color spaces for analysis
            gray_road = cv2.cvtColor(road_region, cv2.COLOR_BGR2GRAY)
            hsv_road = cv2.cvtColor(road_region, cv2.COLOR_BGR2HSV)

            # Analyze road surface texture
            texture_score = self._analyze_road_texture(gray_road)

            # Analyze road color characteristics
            mean_color = np.mean(road_region, axis=(0, 1))
            color_std = np.std(road_region, axis=(0, 1))

            # Detect lane markings to estimate lane count
            lane_count = self._estimate_lane_count(gray_road)

            # Determine road condition
            road_scores = {"dry": 0.0, "wet": 0.0, "icy": 0.0, "construction": 0.0}

            # Dry road characteristics: good texture, normal colors
            if texture_score > 0.6 and np.mean(color_std) > 20:
                road_scores["dry"] = 80.0 + texture_score * 20

            # Wet road characteristics: reflective, darker, smoother
            brightness = np.mean(gray_road)
            if brightness < 100 and texture_score < 0.5:
                reflective_areas = self._detect_reflective_surfaces(road_region)
                road_scores["wet"] = min(
                    90.0, (100 - brightness) * 0.5 + reflective_areas * 50
                )

            # Icy conditions: very bright or very dark, minimal texture
            if (brightness > 200 or brightness < 50) and texture_score < 0.3:
                road_scores["icy"] = min(
                    85.0, abs(brightness - 125) * 0.4 + (0.3 - texture_score) * 100
                )

            # Construction: orange/yellow colors, irregular patterns
            orange_mask = cv2.inRange(hsv_road, (10, 100, 100), (25, 255, 255))
            construction_ratio = np.sum(orange_mask > 0) / orange_mask.size
            if construction_ratio > 0.05:
                road_scores["construction"] = min(80.0, construction_ratio * 400)

            best_condition = max(road_scores.items(), key=lambda x: x[1])

            # Determine road type based on characteristics
            road_type = self._classify_road_type(image, lane_count, texture_score)

            return {
                "condition": best_condition[0],
                "confidence": best_condition[1] / 100.0,
                "type": road_type,
                "estimated_lanes": lane_count,
                "texture_score": texture_score,
                "surface_brightness": brightness,
            }

        except Exception as e:
            logger.error(f"Road analysis error: {e}")
            return {
                "condition": "dry",
                "confidence": 0.6,
                "type": "highway",
                "estimated_lanes": 2,
                "texture_score": 0.6,
                "surface_brightness": 128,
            }

    def _analyze_traffic_context(self, image: np.ndarray) -> Dict[str, Any]:
        """Analyze traffic density and flow patterns"""
        try:
            # Simple traffic analysis based on image characteristics
            # In production, this would integrate with object detection results

            gray = cv2.cvtColor(image, cv2.COLOR_BGR2GRAY)

            # Detect potential vehicle shapes using edge detection
            edges = cv2.Canny(gray, 50, 150)

            # Look for rectangular shapes (vehicles) in upper portion of image
            height = image.shape[0]
            traffic_region = edges[0 : int(height * 0.7), :]

            # Count edge density as proxy for vehicle presence
            edge_density = np.sum(traffic_region > 0) / traffic_region.size

            # Estimate traffic density
            if edge_density < 0.05:
                density = "no_traffic"
                density_score = 0.0
            elif edge_density < 0.1:
                density = "light"
                density_score = 0.3
            elif edge_density < 0.2:
                density = "moderate"
                density_score = 0.6
            elif edge_density < 0.35:
                density = "heavy"
                density_score = 0.8
            else:
                density = "congested"
                density_score = 1.0

            return {
                "density": density,
                "density_score": density_score,
                "edge_density": edge_density,
                "estimated_vehicles": int(edge_density * 20),  # Rough estimate
            }

        except Exception as e:
            logger.error(f"Traffic analysis error: {e}")
            return {
                "density": "light",
                "density_score": 0.3,
                "edge_density": 0.05,
                "estimated_vehicles": 1,
            }

    def _analyze_time_of_day(self, image: np.ndarray) -> Dict[str, Any]:
        """Determine time of day based on lighting conditions"""
        try:
            # Convert to different color spaces
            gray = cv2.cvtColor(image, cv2.COLOR_BGR2GRAY)
            hsv = cv2.cvtColor(image, cv2.COLOR_BGR2HSV)

            # Analyze brightness characteristics
            mean_brightness = np.mean(gray)
            brightness_std = np.std(gray)

            # Analyze color temperature (warmth vs coolness)
            mean_hue = np.mean(hsv[:, :, 0])
            saturation_mean = np.mean(hsv[:, :, 1])

            # Classify time period
            if mean_brightness < self.time_thresholds["night"][1]:
                if mean_brightness < self.time_thresholds["night"][0]:
                    period = "night"
                    confidence = 0.9
                else:
                    period = "dawn_dusk"
                    confidence = 0.7
            else:
                period = "day"
                confidence = 0.8

            # Adjust based on color characteristics
            if period == "dawn_dusk":
                # Dawn/dusk often has warmer colors (orange/red hues)
                if 10 < mean_hue < 30 or mean_hue > 150:  # Orange/red or purple hues
                    confidence += 0.1

            return {
                "period": period,
                "confidence": min(1.0, confidence),
                "brightness": mean_brightness,
                "color_temperature": mean_hue,
            }

        except Exception as e:
            logger.error(f"Time analysis error: {e}")
            return {
                "period": "day",
                "confidence": 0.5,
                "brightness": 128,
                "color_temperature": 90,
            }

    def _analyze_visibility(self, image: np.ndarray) -> Dict[str, Any]:
        """Assess overall visibility conditions"""
        try:
            gray = cv2.cvtColor(image, cv2.COLOR_BGR2GRAY)

            # Calculate visibility metrics
            contrast = np.std(gray)
            clarity = self._calculate_image_clarity(gray)
            brightness = np.mean(gray)

            # Combine metrics for overall visibility score
            visibility_score = (
                0.4 * min(100, contrast * 2)  # Contrast component
                + 0.4 * clarity  # Clarity component
                + 0.2 * min(100, brightness * 0.8)  # Brightness component
            )

            # Categorize visibility
            if visibility_score >= 80:
                category = "excellent"
            elif visibility_score >= 65:
                category = "good"
            elif visibility_score >= 45:
                category = "fair"
            elif visibility_score >= 25:
                category = "poor"
            else:
                category = "very_poor"

            return {
                "score": visibility_score,
                "category": category,
                "contrast": contrast,
                "clarity": clarity,
                "brightness": brightness,
            }

        except Exception as e:
            logger.error(f"Visibility analysis error: {e}")
            return {
                "score": 70.0,
                "category": "good",
                "contrast": 30,
                "clarity": 70,
                "brightness": 128,
            }

    # Helper methods
    def _analyze_road_texture(self, gray_road: np.ndarray) -> float:
        """Analyze road surface texture using gradient analysis"""
        try:
            # Calculate gradients
            grad_x = cv2.Sobel(gray_road, cv2.CV_64F, 1, 0, ksize=3)
            grad_y = cv2.Sobel(gray_road, cv2.CV_64F, 0, 1, ksize=3)
            gradient_magnitude = np.sqrt(grad_x**2 + grad_y**2)

            # Normalize texture score
            texture_score = np.mean(gradient_magnitude) / 255.0
            return min(1.0, texture_score)

        except Exception:
            return 0.5

    def _estimate_lane_count(self, gray_road: np.ndarray) -> int:
        """Estimate number of lanes by detecting lane markings"""
        try:
            # Apply edge detection to find lane markings
            edges = cv2.Canny(gray_road, 50, 150)

            # Use Hough transform to detect lines
            lines = cv2.HoughLinesP(
                edges, 1, np.pi / 180, threshold=50, minLineLength=50, maxLineGap=10
            )

            if lines is None:
                return 2  # Default assumption

            # Filter for roughly horizontal lines (lane markings)
            lane_lines = []
            for line in lines:
                x1, y1, x2, y2 = line[0]
                angle = np.degrees(np.arctan2(y2 - y1, x2 - x1))
                if abs(angle) < 30:  # Roughly horizontal
                    lane_lines.append(line[0])

            # Estimate lanes based on detected markings
            if len(lane_lines) == 0:
                return 2
            elif len(lane_lines) <= 2:
                return 2
            elif len(lane_lines) <= 4:
                return 3
            else:
                return 4

        except Exception:
            return 2

    def _detect_reflective_surfaces(self, road_region: np.ndarray) -> float:
        """Detect reflective surfaces that indicate wet conditions"""
        try:
            gray = cv2.cvtColor(road_region, cv2.COLOR_BGR2GRAY)

            # Look for bright spots that could be reflections
            bright_threshold = np.mean(gray) + np.std(gray) * 2
            bright_mask = gray > bright_threshold

            reflective_ratio = np.sum(bright_mask) / bright_mask.size
            return min(1.0, reflective_ratio * 10)  # Scale appropriately

        except Exception:
            return 0.0

    def _classify_road_type(
        self, image: np.ndarray, lane_count: int, texture_score: float
    ) -> str:
        """Classify the type of road based on characteristics"""
        try:
            # Simple classification based on available features
            if lane_count >= 4:
                return "highway"
            elif lane_count >= 3:
                return "major_road"
            elif texture_score > 0.7:
                return "city_street"
            else:
                return "residential"

        except Exception:
            return "unknown"

    def _calculate_image_clarity(self, gray: np.ndarray) -> float:
        """Calculate image clarity using Laplacian variance"""
        try:
            laplacian_var = cv2.Laplacian(gray, cv2.CV_64F).var()
            # Normalize to 0-100 scale
            return min(100.0, laplacian_var / 10.0)

        except Exception:
            return 70.0

    def _assess_safety_factors(
        self, weather: Dict, road: Dict, traffic: Dict, time: Dict, visibility: Dict
    ) -> List[str]:
        """Assess overall safety factors from all analyses"""
        factors = []

        # Weather-related factors
        if weather["condition"] in ["rain", "snow", "fog"]:
            factors.append(f"reduced_grip_{weather['condition']}")

        # Road condition factors
        if road["condition"] in ["wet", "icy"]:
            factors.append(f"slippery_surface")

        # Visibility factors
        if visibility["category"] in ["poor", "very_poor"]:
            factors.append("poor_visibility")

        # Time factors
        if time["period"] in ["night", "dawn_dusk"]:
            factors.append("low_light_conditions")

        # Traffic factors
        if traffic["density"] in ["heavy", "congested"]:
            factors.append("heavy_traffic")

        return factors

    def _generate_recommendations(
        self, weather: Dict, road: Dict, time: Dict, visibility: Dict
    ) -> List[str]:
        """Generate riding recommendations based on conditions"""
        recommendations = []

        # Weather-based recommendations
        if weather["condition"] == "rain":
            recommendations.extend(
                [
                    "reduce_speed",
                    "increase_following_distance",
                    "avoid_sudden_movements",
                ]
            )
        elif weather["condition"] == "fog":
            recommendations.extend(
                ["use_fog_lights", "reduce_speed_significantly", "stay_alert"]
            )
        elif weather["condition"] == "snow":
            recommendations.extend(["avoid_riding", "extreme_caution", "winter_tires"])

        # Road condition recommendations
        if road["condition"] == "wet":
            recommendations.extend(["gentle_braking", "avoid_painted_lines"])
        elif road["condition"] == "icy":
            recommendations.extend(["avoid_riding", "emergency_only"])

        # Visibility recommendations
        if visibility["category"] in ["poor", "very_poor"]:
            recommendations.extend(["use_headlights", "high_visibility_gear"])

        # Time-based recommendations
        if time["period"] == "night":
            recommendations.extend(["reflective_gear", "extra_vigilance"])

        return list(set(recommendations))  # Remove duplicates

    def _get_default_scene_analysis(self) -> Dict[str, Any]:
        """Return default scene analysis when processing fails"""
        return {
            "weather_condition": "clear",
            "weather_confidence": 0.5,
            "road_condition": "dry",
            "road_type": "highway",
            "lane_count": 2,
            "time_of_day": "day",
            "brightness_level": 128,
            "traffic_density": "light",
            "visibility": 70.0,
            "visibility_category": "good",
            "safety_factors": [],
            "recommended_adjustments": [],
        }
