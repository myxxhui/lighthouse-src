# Cost Module

This module handles the dual-cost model calculations for Lighthouse:

- Billable Cost (based on K8s Request)
- Usage Value (based on actual usage P95)
- Waste/Efficiency calculations
- Four-level aggregation (Namespace → Node → Workload → Pod)
- Zombie asset detection

This is a placeholder file. Actual implementation will be added in Phase 2.