import os
import time
import logging
import json
import asyncio
from nats.aio.client import Client as NATS

logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(name)s - %(levelname)s - %(message)s')
logger = logging.getLogger("AIWorker")

class ObjectDetector:
    def __init__(self):
        logger.info("Initializing Object Detector...")
        self.model_loaded = True

    def detect(self, frame_data):
        # Mock detection
        return [
            {"label": "person", "confidence": 0.95, "bbox": [100, 100, 200, 200]}
        ]

async def main():
    camera_id = os.getenv("CAMERA_ID", "default_cam")
    nats_url = os.getenv("NATS_URL", "nats://localhost:4222")

    nc = NATS()
    await nc.connect(nats_url)
    logger.info(f"Connected to NATS at {nats_url}")

    detector = ObjectDetector()

    async def frame_handler(msg):
        # In reality, parse MJPEG frame from msg.data
        detections = detector.detect(msg.data)
        if detections:
            # Publish results back to NATS
            result_subject = f"camera.{camera_id}.events"
            await nc.publish(result_subject, json.dumps(detections).encode())

    subject = f"camera.{camera_id}.frames"
    await nc.subscribe(subject, cb=frame_handler)
    logger.info(f"Subscribed to {subject}")

    while True:
        await asyncio.sleep(1)

if __name__ == "__main__":
    asyncio.run(main())
