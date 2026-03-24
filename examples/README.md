# Dev API Example

- pull the prebuilt ZaraOS dev API image
- run it in Docker-in-Docker mode
- build your ROS test images inside that container
- ask Cortex to deploy them through `POST /instances`

That is the only pattern documented here.

## Files

- `docker-compose.yml`: starts the prebuilt Cortex dev host
- `apps/ros-talker/`: sample ROS talker image
- `apps/ros-listener/`: sample ROS listener image

## Start The Dev API Host

Set the published dev host image if you do not want the default:

```sh
export ZARAOS_DEV_IMAGE=ghcr.io/KoalbyMQP/zaraos-dev:latest
```

Start the host:

```sh
docker compose -f examples/docker-compose.yml up -d
docker compose -f examples/docker-compose.yml ps
```

Stop it:

```sh
docker compose -f examples/docker-compose.yml down
```

## Build ROS Test Images

Build inside `dev-api`, because that container owns the inner Docker daemon that Cortex deploys to:

```sh
docker compose -f examples/docker-compose.yml exec dev-api bash
cd /workspace/examples/apps/ros-talker
docker build -t ros-talker:dev .

cd /workspace/examples/apps/ros-listener
docker build -t ros-listener:dev .
```

You can swap those folders for your own project images.

## Deploy Through Cortex

The simplest local path is to call Cortex from inside `dev-api`, where `127.0.0.1` bypasses auth.

Start the talker:

```sh
curl -X POST http://127.0.0.1:8080/instances \
  -H 'Content-Type: application/json' \
  -d '{"app":"ros-talker","image":"ros-talker:dev"}'
```

Start the listener:

```sh
curl -X POST http://127.0.0.1:8080/instances \
  -H 'Content-Type: application/json' \
  -d '{"app":"ros-listener","image":"ros-listener:dev"}'
```

Check status:

```sh
curl http://127.0.0.1:8080/instances
```

Read logs:

```sh
curl http://127.0.0.1:8080/instances/<id>/logs
```

Stop an instance:

```sh
curl -X DELETE http://127.0.0.1:8080/instances/<id>
```

## If You Want To Test From A Web Client

The web client should call the same API route:

```http
POST /instances
Content-Type: application/json

{"app":"my-node","image":"my-node:dev"}
```

When the request comes from outside `dev-api`, you must use normal Cortex auth. The unauthenticated `127.0.0.1` shortcut only applies from inside the container.

## Adapt This To Your Project

1. Replace `apps/ros-talker/` and `apps/ros-listener/` with your own ROS node images.
2. Keep using `examples/docker-compose.yml` to start the dev API host.
3. Build your images inside `dev-api`.
4. Deploy them through Cortex with `POST /instances`.
