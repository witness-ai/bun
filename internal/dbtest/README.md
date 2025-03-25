# Database Tests

This directory contains integration tests that run against real database instances.

## Platform-Agnostic Database Setup

The tests support both AMD64 (Intel/AMD) and ARM64 (Apple Silicon) architectures using a single script that automatically detects your platform.

### Quick Start

Run the test script:

```bash
./test.sh
```

This script will:
- Detect your CPU architecture automatically
- Configure and start the appropriate database containers for your platform
- Wait for all containers to be healthy
- Run the tests with correct timezone settings

### Running Specific Tests

You can pass any arguments to the test script, and they will be forwarded to `go test`:

```bash
# Run a specific test
./test.sh -run TestMssqlMerge

# Run with verbose output
./test.sh -v
```

### Cleanup

By default, the script leaves containers running for faster subsequent test runs. 
To automatically clean up containers after testing, use:

```bash
./test.sh --cleanup
```

## Supported Databases

The test suite includes support for:
- MySQL 5.7
- MySQL 8.0
- MariaDB 10.6
- PostgreSQL 15
- Microsoft SQL Server

## Notes on SQL Server

The script automatically handles SQL Server configuration based on your platform:

- For ARM64 platforms (Apple Silicon):
  - Uses Azure SQL Edge image with emulation
  - Adds platform: linux/amd64 to enable x86_64 emulation

- For AMD64 platforms (Intel/AMD):
  - Uses regular SQL Server 2019 image

Both configurations use the same connection string: `sqlserver://sa:passWORD1@localhost:14339?database=test`

## Troubleshooting

The script automatically sets `TZ=UTC` when running tests to avoid timezone-related failures.

If you need to run tests manually:

```bash
TZ=UTC go test
```

This ensures that all timestamps in the test snapshots match the expected UTC format. 