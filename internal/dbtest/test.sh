#!/bin/bash
set -e

# Get the directory where this script is located
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
cd "$SCRIPT_DIR"

# Parse command-line arguments
CLEANUP=0
TEST_ARGS=""

for arg in "$@"; do
  case $arg in
    --cleanup)
      CLEANUP=1
      shift
      ;;
    *)
      TEST_ARGS="$TEST_ARGS $arg"
      ;;
  esac
done

# Detect platform architecture
ARCH=$(uname -m)
echo "Detected architecture: $ARCH"

# Stop and remove any existing containers
echo "Stopping and removing existing containers..."
docker-compose down -v || true
docker ps | grep 14339 | awk '{print $1}' | xargs -r docker rm -f || true

# Create a temporary compose file with platform-specific settings
TEMP_COMPOSE="docker-compose.override.yaml"

# Configure containers based on architecture
if [[ "$ARCH" == "arm64" || "$ARCH" == "aarch64" ]]; then
  echo "Setting up for ARM64 (Apple Silicon) architecture..."
  cat > "$TEMP_COMPOSE" << EOF
version: '3.9'
services:
  mssql:
    platform: linux/arm64
    image: mcr.microsoft.com/azure-sql-edge:latest
    user: root
    environment:
      - ACCEPT_EULA=Y
      - SA_PASSWORD=passWORD1
      - MSSQL_PID=Developer
    command: /opt/mssql/bin/sqlservr
    shm_size: 2gb
    deploy:
      resources:
        limits:
          memory: 6G
EOF
elif [[ "$ARCH" == "x86_64" || "$ARCH" == "amd64" ]]; then
  echo "Setting up for AMD64 (Intel/AMD) architecture..."
  cat > "$TEMP_COMPOSE" << EOF
version: '3.9'
services:
  mssql:
    image: mcr.microsoft.com/mssql/server:2019-latest
    shm_size: 2gb
    environment:
      - MSSQL_MEMORY_LIMIT_MB=4096
EOF
else
  echo "Unsupported architecture: $ARCH"
  exit 1
fi

# Start the containers
echo "Starting database containers..."
docker-compose up -d

# Wait for MSSQL container to be healthy
echo "Waiting for MSSQL container to be healthy..."
sleep 10
attempt=1
max_attempts=30

while [[ $attempt -le $max_attempts ]]; do
    echo "Checking MSSQL container health (attempt $attempt/$max_attempts)..."
    
    # Check if container is running
    if ! docker ps | grep dbtest-mssql-1 > /dev/null; then
        echo "MSSQL container is not running. Something went wrong."
        docker ps
        docker-compose logs mssql
        # Try alternate configuration if on ARM64
        if [[ "$ARCH" == "arm64" || "$ARCH" == "aarch64" ]]; then
            echo "Trying alternate SQL Server configuration for ARM64..."
            docker-compose down -v
            cat > "$TEMP_COMPOSE" << EOF
version: '3.9'
services:
  mssql:
    platform: linux/amd64
    image: mcr.microsoft.com/azure-sql-edge:latest
    environment:
      - ACCEPT_EULA=Y
      - SA_PASSWORD=passWORD1
      - MSSQL_PID=Developer
    shm_size: 2gb
EOF
            docker-compose up -d
            sleep 15
            continue
        else
            exit 1
        fi
    fi
    
    # Check if container is healthy
    if docker ps | grep dbtest-mssql-1 | grep -E "(unhealthy|starting)" > /dev/null; then
        echo "MSSQL container is not healthy yet. Waiting..."
        sleep 5
        ((attempt++))
    else
        echo "MSSQL container is healthy!"
        break
    fi
    
    if [[ $attempt -gt $max_attempts ]]; then
        echo "MSSQL container did not become healthy within the timeout"
        docker ps
        docker-compose logs mssql
        exit 1
    fi
done

# Create test database if it doesn't exist
echo "Creating test database if needed..."
docker exec dbtest-mssql-1 /opt/mssql-tools/bin/sqlcmd -S localhost -U sa -P 'passWORD1' -Q "IF NOT EXISTS (SELECT name FROM sys.databases WHERE name = 'test') CREATE DATABASE test"

echo "All database containers are up and running!"

# Run tests with UTC timezone to avoid timestamp issues
echo "Running tests with: TZ=UTC go test $TEST_ARGS"
TZ=UTC go test $TEST_ARGS

TEST_EXIT_CODE=$?

# Clean up if requested
if [[ $CLEANUP -eq 1 ]]; then
  echo "Cleaning up containers..."
  docker-compose down -v
fi

# Exit with the test exit code
exit $TEST_EXIT_CODE
