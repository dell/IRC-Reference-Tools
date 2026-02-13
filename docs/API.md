# API Documentation

This section describes the internal APIs and interfaces used by the IRC-Reference-Tools system components.

## Overview

The IRC-Reference-Tools system uses HTTP REST API for web UI and external integrations.

## HTTP REST API

### Web UI API Endpoints

#### Base URL
- **Production**: `https://localhost/api/v1`

#### Authentication
All API endpoints require authentication using one of the following methods:

**JWT Token Authentication:**
```http
Authorization: Bearer <jwt-token>
```

**Basic Authentication:**
```http
Authorization: Basic <base64-encoded-credentials>
```

JWT tokens are obtained via the `/api/v1/GetToken` endpoint and expire after the time specified in the response. Basic authentication uses base64-encoded `username:password` credentials.

### Endpoints

#### POST /api/v1/GetToken
Authenticate user and receive JWT token.

**Request:**
```json
{
  "username": "admin",
  "password": "password"
}
```

**Response:**
```json
{
"status": "ok",
"token": "<TOKEN>"
}
```
**Example:**
```bash
# Get token
curl -k -X POST -H "Content-Type: application/json" -d '{"username":"admin","password":"admin"}' https://localhost:443/api/v1/GetToken
```

#### GET /api/v1/irc
List all IRC devices.

**Response:**
```json
{
  "data": [
    {
      "ip": "<IRC IP>",
      "state": "Monitoring",
      "valve1state": "Unknown",
      "valve2state": "Unknown"
    }
  ],
  "status": "Fetched IRC list",
  "systemTypes": [
    "idrac"
  ]
}
```

**Example:**
```bash
# Get IRC list with JWT authentication
curl -k -H "Authorization: Bearer <TOKEN>" https://localhost:443/api/v1/irc

# Get IRC list with basic authentication
curl -k -X GET -u "admin:admin" https://localhost:443/api/v1/irc
```

#### POST /api/v1/irc
Add a new IRC device.

**Request:**
```json
{
  "ip": "<IRC IP>",
  "username": "<username>",
  "password": "<password>"
}
```

**Response:**
```json
{
  "status": "IRC added"
}
```

**Example:**
```bash
# Add IRC with basic authentication
curl -k -X POST -u "admin:admin" -H "Content-Type: application/json" -d '{
"ip": "<IRC IP>",
"username": "<username>",
"password": "<password>"
}' https://localhost:443/api/v1/irc
```

#### GET /api/v1/irc/:ip/racdevice
List rack devices for specific IRC.

**Response:**
```json
{
  "data": [
    {
      "ip": "<RAC IP>",
      "state": "Shutdown Received",
      "serviceIp": "<IRC IP>",
      "systemtypedesc": "idrac",
      "isforcefulshutdown": true,
      "timeout": 0
    }
  ],
  "status": "Fetched associated systems"
}
```

**Example:**
```bash
# Get associated systems with basic authentication
curl -k -X GET -u "admin:admin" https://localhost:443/api/v1/irc/<IRC IP>/racdevice
```

#### POST /api/v1/irc/:ip/racdevice
Add a new rack device to specific IRC.

**Request Parameters:**
- **ip** (string, required): IP address and port of the rack device (e.g., "100.71.133.166:61021")
- **username** (string, required): Username for authenticating to the rack device
- **password** (string, required): Password for authenticating to the rack device
- **systemtypedesc** (string, required): System type description (e.g., "idrac") 
  - matches name in yaml files in [configs/systemtypes](../configs/systemtypes/custom/README.md).
- **isforcefulshutdown** (boolean, optional): Whether to force shutdown the device (default: true)
- **timeout** (integer, optional): Timeout in seconds for operations (default: 300). Needed if isForcefulShutdown is false.

**Request:**
```json
{
"ip": "<RAC IP>",
"username": "<username>",
"password": "<password>",
"systemtypedesc": "idrac",
"isforcefulshutdown": false,
"timeout": 300
}
```

**Response**
```json
{
  "status": "System added"
}
```

**Example:**
```bash
# Add rack device with basic authentication
curl -k -X POST -u "admin:admin" -H "Content-Type: application/json" -d '{
"ip": "<RAC IP>",
"username": "<username>",
"password": "<password>",
"systemtypedesc": "idrac",
"isforcefulshutdown": false,
"timeout": 300
}' https://localhost:443/api/v1/irc/<IRC IP>/racdevice
```

#### DELETE /api/v1/irc/:ip/racdevice
Remove rack devices from specific IRC.

**Request:**
```json
{
  "ips": ["<RAC IP1>", "<RAC IP2>"]
}
```

**Example:**
```bash
# Remove rack devices with basic authentication
curl -k -X DELETE -u "admin:admin" -H "Content-Type: application/json" -d '{
"ips": ["<RAC IP1>", "<RAC IP2>"]
}' https://localhost:443/api/v1/irc/<IRC IP>/racdevice
```

## API Security

### Authentication
- JWT tokens for web UI access
- Basic authentication for device access

### Logging
All APIs log requests and responses at appropriate levels:
- INFO: Successful operations
- WARN: Recoverable errors
- ERROR: Failed operations
- DEBUG: Detailed debugging information