# Multus Validation

The `multus validation` command validates whether the current Multus and system configurations will support Rook with Multus. This tool should be run **before** installing Rook to ensure network compatibility.

See [Validating Multus configuration](https://rook.github.io/docs/rook/latest/CRDs/Cluster/network-providers/?h=valid#validating-multus-configuration) for more details.

## Subcommands

1. `run`: Run Multus validation tests to verify network connectivity
2. `config`: Generate a sample validation test config file
3. `cleanup`: Clean up Multus validation test resources

## Validation Run

Run Multus network validation tests. This verifies that Multus network communication functions properly by starting a web server and multiple clients.

```bash
kubectl rook-ceph multus validation run --help
```

### Examples

Run validation with public and cluster networks(default namespace rook-ceph):

```bash
kubectl rook-ceph multus validation run \
  --namespace rook-ceph \
  --public-network [namespace]/public-net \
  --cluster-network [namespace]/cluster-net
```

Run validation using a config file:

```bash
kubectl rook-ceph multus validation run --config /path/to/config.yaml
```

## Validation Config

Generate a sample validation test configuration file for different deployment scenarios.

```bash
kubectl rook-ceph multus validation config --help
```

## Validation Cleanup

Clean up any resources left over from a previous validation run.

```bash
kubectl rook-ceph multus validation cleanup --help
```
