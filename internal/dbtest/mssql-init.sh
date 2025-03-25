#!/bin/bash
set -e

# Start SQL Server
/opt/mssql/bin/sqlservr &

# Wait for SQL Server to start
for i in {1..60}; do
  if /opt/mssql-tools/bin/sqlcmd -S localhost -U sa -P "$SA_PASSWORD" -Q "SELECT 1" &> /dev/null; then
    echo "SQL Server started"
    break
  fi
  echo "Waiting for SQL Server to start..."
  sleep 1
done

# Create the test database if it doesn't exist
/opt/mssql-tools/bin/sqlcmd -S localhost -U sa -P "$SA_PASSWORD" -Q "IF NOT EXISTS (SELECT name FROM sys.databases WHERE name = 'test') CREATE DATABASE test"
echo "Test database created"

# Keep container running
exec tail -f /dev/null 