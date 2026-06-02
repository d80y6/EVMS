FROM pytorch/pytorch:2.5.1-cpu

RUN apt-get update && apt-get install -y \
    libgl1 \
    libglib2.0-0 \
    protobuf-compiler \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY services/ai-worker/requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Generate Python gRPC stubs from Triton proto
COPY apps/triton-inference-service/proto/ proto/
RUN python -m grpc_tools.protoc -Iproto --python_out=. --grpc_python_out=. proto/inference.proto

COPY services/ai-worker/ .

CMD ["python", "main.py"]
