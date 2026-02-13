# Setup Instructions

This guide provides detailed instructions for installing, configuring, and deploying the IRC-Reference-Tools system.

## Requirements

### System Requirements

- **Operating System**: Linux (Ubuntu 20.04+, CentOS 8+, RHEL 8+) or Windows (Docker Desktop)
- **Network**: Access to IRC devices and rack management interfaces

### Software Requirements

- **Docker**: Version 29.2.1 or later
- **Docker Compose**: Version 5.0.2 or later
- **Git**: For cloning the repository

### Network Requirements

- **Port 443**: Web UI access (HTTPS)
- **Port 8161**: ActiveMQ web console (optional)
- **Port 3306**: MySQL database (internal)
- **Port 61613**: ActiveMQ STOMP protocol (internal)
- **IRC Device Access**: HTTPS/HTTP to IRC management interfaces
- **Rack Device Access**: HTTPS/HTTP to rack device management interfaces

## Prerequisites

### 1. Docker Installation

Install Docker and Docker Compose following the official documentation:

- **Docker Desktop (Windows/macOS)**: https://docs.docker.com/desktop/install/
- **Docker Engine (Linux)**: https://docs.docker.com/engine/install/
- **Docker Compose**: Included with Docker Desktop or install separately: https://docs.docker.com/compose/install/

### 2. Verify Installation
```bash
docker --version
docker compose version
```

### 3. Git Installation

If Git is not already installed, follow the official documentation: https://git-scm.com/book/en/v2/Getting-Started-Installing-Git

## Installation

### 1. Clone Repository
```bash
git clone https://github.com/dell/IRC-Reference-Tools
cd IRC-Reference-Tools
```

### 2. Start the System

The compose script automatically handles environment variables, certificates, and container startup:

```bash
# Start the system (with build if code was changed)
bash docker-compose-files/compose.sh up --build

# Or start without rebuild (if already built)
bash docker-compose-files/compose.sh up
```

**What the script handles automatically:**
- Generates required environment variables (MySQL passwords, JWT secret)
- Creates self-signed SSL certificates if they don't exist
- Sets proper certificate permissions
- Starts all Docker containers with correct configuration

## Post-Installation Verification

### 1. Check Container Status
```bash
bash docker-compose-files/compose.sh ps
```

Expected output should show all containers as "Up":
- activemq
- mysqldb  
- configui
- dbdiscauth
- redfishread
- action

### 2. Verify Web UI Access
```bash
# Test web UI (accept self-signed certificate)
curl -k https://localhost
```

Or open in a web browser: `https://localhost`

## Configuration

### 1. Initial System Setup

1. **Access Web UI**: Open `https://localhost` in your browser
2. **Login**: Use default credentials (admin/admin) or configure authentication
3. **Add IRC Devices**:
   - Click "Add IRC"
   - Enter IRC IP address
   - Provide username and password
   - Select connection type
4. **Add Rack Devices**:
   - Click "Add Device"
   - Enter device IP address
   - Provide credentials
   - Select system type (iDRAC, NVLink, etc.)
   - Associate with IRC device

### 2. System Type Configuration

The system includes built-in configurations for common device types. To add custom device types:

1. Create YAML file in `configs/systemtypes/custom/`
2. Follow the configuration format in [Custom System Types Documentation](configs/systemtypes/custom/README.md)
3. Restart containers to load new configurations

### Container Logs

#### Important Log Locations
- **configui**: Web UI and authentication issues
- **redfishread**: IRC device connectivity and event processing
- **action**: Shutdown action execution
- **dbdiscauth**: Database operations and device management
- **mysqldb**: Database errors and initialization

#### Log Levels
Configure logging level via environment variable:
```bash
# Set in .env file
LOG_LEVEL=debug

# Or pass to docker compose
LOG_LEVEL=debug bash docker-compose-files/compose.sh up -d
```

## Maintenance

### Backup and Recovery

#### Database Backup
```bash
# Create backup
bash docker-compose-files/compose.sh exec mysqldb mysqldump -u root -p --all-databases --single-transaction --set-gtid-purged=OFF > consistent_backup.sql


# Restore backup
bash docker-compose-files/compose.sh exec -T mysqldb mysql -u root -p<MYSQL_ROOT_PASSWORD> < consistent_backup.sql
```

**Note:** Replace `<MYSQL_ROOT_PASSWORD>` with the actual password from your `.env` file.

#### Update Configuration
```bash
# After changing configs, restart affected services
bash docker-compose-files/compose.sh restart redfishread action
```

### Security Maintenance

#### Certificate Renewal
```bash
# Regenerate self-signed certificates
bash docker-compose-files/compose.sh --profile certgen up certgen
bash docker-compose-files/compose.sh restart configui
```

## Support

For additional support:
- Review the [troubleshooting guide](TROUBLESHOOTING.md)
- Check container logs for detailed error messages
- Review the [leak detection workflow](LEAK-DETECTION-WORKFLOW.md)
- Consult the [custom system type configuration documentation](configs/systemtypes/custom/README.md)
