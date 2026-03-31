#!/bin/bash
set -e
cd workspace

# Port must be changed
grep -q "port: 9090" config.yaml

# Comments must be preserved (these would be lost if Write tool replaced the file)
grep -q "# Main server port" config.yaml
grep -q "# Database settings" config.yaml
grep -q "# Connection pool" config.yaml
grep -q "# Logging configuration" config.yaml
grep -q "# Cache settings" config.yaml
grep -q "# Security settings" config.yaml
