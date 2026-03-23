# IRC-Reference-Tools — Project Notes

## Build & Verify

Always build and verify via Docker Compose (never run `npm run build` or `go build` on the host directly):

```bash
cd /home/nikita_goyal/IRC-Reference-Tools
docker compose -f docker-compose-files/docker-compose.yml --project-directory . up --build -d
```

Check container status:

```bash
docker compose -f docker-compose-files/docker-compose.yml --project-directory . ps
```
