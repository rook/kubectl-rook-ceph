# radosgw-admin

This runs any radosgw-admin CLI command with arbitrary args.

## Examples

```bash
kubectl rook-ceph radosgw-admin user create --display-name="my user" --uid=myuser

# Info: running 'radosgw-admin' command with args: [user create --display-name=my-user --uid=myuser]
# {
#     "user_id": "myuser",
#     "display_name": "my user",
#    ...
#    ...
#    ...
# }
```
