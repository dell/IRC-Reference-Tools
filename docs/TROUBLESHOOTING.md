# Troubleshooting Guide

Common issues and solutions for the IRC-Reference-Tools Docker deployment.

## Quick Diagnostics

```bash
bash docker-compose-files/compose.sh ps              # Container status
bash docker-compose-files/compose.sh logs --tail=50  # Recent logs
docker stats                   # Resource usage
curl -k https://localhost      # Test web UI
```

## Container Issues

### Containers Fail to Start

```bash
# Check status and logs for the failing container
bash docker-compose-files/compose.sh ps
bash docker-compose-files/compose.sh logs <container-name>

# Check for port conflicts
netstat -tulpn | grep -E ':(443|8161|3306|61613)'

# Check disk space
df -h
```

### Web UI Not Accessible

```bash
# Check configui logs
bash docker-compose-files/compose.sh logs configui

# Verify certificates exist
ls -la .certs/

# Regenerate certificates if needed
bash docker-compose-files/compose.sh --profile certgen up certgen
bash docker-compose-files/compose.sh restart configui
```

### Database Issues

```bash
# Check MySQL logs
bash docker-compose-files/compose.sh logs mysqldb

# Test database connection
bash docker-compose-files/compose.sh exec mysqldb mysql -u reftools -p -e "SHOW DATABASES;"

# Full database reset (destroys data)
bash docker-compose-files/compose.sh down
docker volume rm irc-reference-tools_mysqldb-volume
bash docker-compose-files/compose.sh up -d
```

### Configuration Not Loading

If custom system types or environment changes aren't taking effect:

```bash
# Verify config files are mounted
bash docker-compose-files/compose.sh exec redfishread ls -la /configs/systemtypes/

# Restart services to reload configs
bash docker-compose-files/compose.sh restart redfishread action

# Recreate containers to pick up .env changes
bash docker-compose-files/compose.sh down && bash docker-compose-files/compose.sh up -d
```

## Network Issues

### Cannot Reach IRC or Rack Devices

```bash
# Test connectivity from the host
curl -k -u username:password https://<device-ip>/redfish/v1

# Test connectivity from inside a container
bash docker-compose-files/compose.sh exec redfishread curl -k -u username:password https://<device-ip>/redfish/v1
```

If the host can reach the device but the container cannot, check the Docker network configuration:

```bash
docker network inspect irc-reference-tools_host-bridge-net
```

## Collecting Logs for Support

```bash
bash docker-compose-files/compose.sh -p irc-reference-tools logs > container-logs.txt
docker version > docker-version.txt
```
