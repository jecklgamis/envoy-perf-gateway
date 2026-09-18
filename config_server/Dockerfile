FROM python:3.12-slim
WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY config_server.py .

ENV CONFIG_SERVER_STORAGE_DIR=/data
RUN mkdir -p /data

EXPOSE 8090
CMD ["python3", "config_server.py"]
