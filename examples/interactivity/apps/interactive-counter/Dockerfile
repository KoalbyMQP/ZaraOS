# Build from examples/interactivity after building the base image:
#   docker build -f apps/interactive-counter/Dockerfile -t interactive-counter:dev .
ARG BASE=zara-commands-base:dev
FROM ${BASE}
COPY apps/interactive-counter/main.py /app/main.py
CMD ["python3", "-u", "/app/main.py"]
