# Data Layer

This directory contains all data access implementations for Lighthouse:

- Prometheus client (read-only)
- K8s API client (read-only)  
- PostgreSQL repository
- External data source adapters

All data access should follow the read-only principle for safety.