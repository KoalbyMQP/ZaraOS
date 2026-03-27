This is where the ZaraOS project will live ~ stay tuned for updates!

## DevPod quick start

1. **Install DevPod** — Download and install [DevPod](https://devpod.sh/) for your platform.

2. **Install Docker** — Download and install [Docker Desktop](https://www.docker.com/products/docker-desktop/) (or your preferred Docker engine on Linux).

3. **Start Docker** — Run Docker so images can be pulled and builds can run (Docker must be running before you open the workspace).

4. **Create a workspace from Git** — In DevPod, create a new workspace from this repository, using:

   `https://github.com/KoalbyMQP/ZaraOS.git@develop`

5. **Choose your IDE** — When prompted, pick the editor or IDE you want DevPod to use.

6. **If setup looks stuck or shows an error** — Open the DevPod **Workspaces** list, find this workspace, and click **Open** next to it.

7. **Run the stack** — After the workspace opens, open the integrated terminal and run:

   ```bash
   start.sh
   ```

   That starts Cortex and serves it on **port 8080**, which enables Apps access.
