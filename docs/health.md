# Health

The `health` command checks the health of a Ceph cluster and reports common configuration issues. It currently validates the following (let us know if you would like to add other validations):

1. **Mon Distribution** — At least three mon pods should be running on different nodes, and mon quorum is intact
2. **Ceph Cluster Health** — Overall Ceph health status (HEALTH_OK, HEALTH_WARN, HEALTH_ERR)
3. **OSD Distribution** — At least three OSD pods should be running on different nodes
4. **All Pods Status** — All pods are in Running/Succeeded state
5. **PG Status** — Placement groups are in a healthy state
6. **MGR Status** — At least one mgr pod is running

Results are organized by category (Storage, K8s Resources) and each check reports one of:

- **OK** — The check passed.
- **Warning** — There is a potential issue that should be addressed.
- **Critical** — Immediate attention is required.

### PG States

The following PG states are considered healthy:

- `active+clean` — Normal healthy state
- `active+clean+scrubbing` — Routine data integrity check (runs daily)
- `active+clean+scrubbing+deep` — Deep data integrity check (runs approximately weekly)
- `active+clean+snaptrim` — Routine snapshot trimming
- `active+clean+snaptrim_wait` — Waiting to start snapshot trimming

Any other PG state is flagged as a warning or critical issue.

## Usage

```bash
kubectl rook-ceph health
kubectl rook-ceph health --verbose
kubectl rook-ceph health -o json
kubectl rook-ceph health -o yaml
```

Use `--verbose` to include individual resource details (e.g., pod names, nodes) for each check.

Use `-o`/`--output` to change the output format. Supported values: `text` (default), `json`, `yaml`. JSON and YAML output is printed to stdout so it can be captured separately from progress logs (which go to stderr):

```bash
kubectl rook-ceph health -o json > report.json
kubectl rook-ceph health -o json 2>/dev/null | jq .
```

## Example Output

```
Checking Ceph status ...
Checking Mon Distribution...
Checking Ceph Cluster Health...
Checking OSD Distribution...
Checking Pods Status...
Checking PG Status...
Checking MGR Status...

========================================================================
CLUSTER HEALTH REPORT
========================================================================
Generated: 2026-07-22 04:31:13 UTC
Namespace: rook-ceph

========================================================================
Storage
========================================================================

[OK] Ceph Cluster Health [OK]
	Status: HEALTH_OK

[OK] PG Status [OK]
	Status: 185 PGs in healthy state
	Details:
		- PgState: active+clean, PgCount: 185

========================================================================
K8s Resources
========================================================================

[OK] Mon Distribution [OK]
	Status: 3 mon pods running on 3 different nodes
	Details:
		- 3/3 mons in quorum

[OK] OSD Distribution [OK]
	Status: 3 osd pods running on 3 different nodes

[OK] Pods Status [OK]
	Status: All 58 pods are Running/Succeeded
	Details:
		- 58 Running/Succeeded across checked namespaces

[OK] MGR Status [OK]
	Status: 2 mgr pod(s) running

========================================================================
SUMMARY
========================================================================
Total Checks: 6
OK:           6
```
