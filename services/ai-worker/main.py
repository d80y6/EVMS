import os
import asyncio
import json
import logging
import cv2
import numpy as np
import time
from nats.aio.client import Client as NATS
from ultralytics import YOLO

logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(name)s - %(levelname)s - %(message)s')
logger = logging.getLogger("AIWorker")

class AIWorker:
    def __init__(self, camera_id, nats_url, sampling_rate=5):
        self.camera_id = camera_id
        self.nats_url = nats_url
        self.sampling_rate = sampling_rate # Process every Nth frame
        self.frame_count = 0
        self.nc = NATS()
        logger.info(f"Loading YOLOv8 model for {camera_id}... (Sampling: 1/{sampling_rate})")
        # Load model with task='detect' and use half precision if possible
        self.model = YOLO('yolov8n.pt')

    async def connect(self):
        await self.nc.connect(self.nats_url)
        logger.info(f"Connected to NATS at {self.nats_url}")

    async def run(self):
        await self.connect()

        subject = f"camera.{self.camera_id}.frames"
        await self.nc.subscribe(subject, cb=self.on_frame)
        logger.info(f"Subscribed to {subject}")

        # Keep the event loop running
        while True:
            await asyncio.sleep(1)

    async def on_frame(self, msg):
        self.frame_count += 1
        if self.frame_count % self.sampling_rate != 0:
            return

        try:
            start_time = time.time()
            nparr = np.frombuffer(msg.data, np.uint8)
            frame = cv2.imdecode(nparr, cv2.IMREAD_COLOR)

            if frame is None:
                return

            # Perform inference
            results = self.model(frame, verbose=False)
            inference_time = time.time() - start_time

            events = []
            for r in results:
                for box in r.boxes:
                    cls = int(box.cls[0])
                    label = self.model.names[cls]
                    conf = float(box.conf[0])

                    if conf > 0.5:
                        b = box.xyxy[0].tolist()
                        events.append({
                            "label": label,
                            "confidence": conf,
                            "bbox": b,
                            "inference_ms": inference_time * 1000
                        })

            if events:
                result_subject = f"camera.{self.camera_id}.events"
                await self.nc.publish(result_subject, json.dumps(events).encode())

            # Optional: Log performance periodically
            if self.frame_count % (self.sampling_rate * 100) == 0:
                logger.info(f"Performance: {inference_time*1000:.2f}ms per frame, Camera: {self.camera_id}")

        except Exception as e:
            logger.error(f"Error processing frame: {e}")

if __name__ == "__main__":
    camera_id = os.getenv("CAMERA_ID", "demo_cam")
    nats_url = os.getenv("NATS_URL", "nats://nats:4222")
    sampling_rate = int(os.getenv("AI_SAMPLING_RATE", "5"))

    worker = AIWorker(camera_id, nats_url, sampling_rate)
    try:
        asyncio.run(worker.run())
    except KeyboardInterrupt:
        logger.info("AI Worker shutting down...")
