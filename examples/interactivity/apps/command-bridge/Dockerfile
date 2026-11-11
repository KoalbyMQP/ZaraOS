# Build from examples/interactivity after building the base image:
#   docker build -f apps/command-bridge/Dockerfile -t command-bridge:dev .
ARG BASE=zara-commands-base:dev
FROM ${BASE}
COPY apps/command-bridge/bridge.py /app/bridge.py
CMD ["python3", "-u", "/app/bridge.py"]
