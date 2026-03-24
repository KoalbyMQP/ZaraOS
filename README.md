This is where the ZaraOS project will live ~ stay tuned for updates!
And more!

## Dev Docker Stack

The supported local test pattern lives in `examples/`.

It uses a prebuilt dev API image running in Docker-in-Docker mode so Cortex can deploy ROS test containers through the same API path the product uses.

Start here:

```sh
export ZARAOS_DEV_IMAGE=ghcr.io/your-org/zaraos-dev:latest
docker compose -f examples/docker-compose.yml up -d
```

Full docs:

- `examples/README.md`
