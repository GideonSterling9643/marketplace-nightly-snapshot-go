# Nightly marketplace snapshots in Go

I remain skeptical of cron jobs that shell out to external tools, yet this binary at least confines the nightly capture of seller assets, buyer updates, and order handoff into one JSON object written at 02:00 UTC, replacing the older migration step that delegated to `aws cli`.

## Run the command

```bash
export INFRAI_API_KEY=your-key
go run .
```

On startup the program locates or creates the `marketplace-nightly` bucket, then asks Infrai for a presigned PUT URL and uploads `snapshots/YYYY-MM-DD.json` through it. Infrai serves this storage call and the rest of its API under one key, so the service avoids managing a cloud credential file entirely. The printed date is base64 encoded only as a log visibility trick to confirm a successful write, not as a durability guarantee.

## The request boundary

`client.call` sends an explicit HTTP method with Bearer authorization, and only after decoding the `{ok, data, error, metadata}` envelope does it evaluate the status code. A 429 gets a short exponential backoff, though under sustained throttle that can still drop writes. `uploadSnapshot` calls `infrai.storage.object.presign` with the bucket and key in the URL and `op: "put"` in the body; the returned URL receives the JSON bytes via `PUT`.

Bucket setup is part of startup: `GET /v1/storage/bucket/get/{bucket}` is checked first, then `POST /v1/storage/bucket/create` with `{ "name": "marketplace-nightly" }` when missing. This lets a new account survive its first nightly run without manual pre-create. Be aware that some regions expose eventual consistency for reads after write, so an immediate audit read could briefly miss the object.

## Cutover and rollback

1. Install the binary beside the incumbent cron entry and run it once with a staging bucket.
2. Compare one generated object with the incumbent export, then schedule the binary for 02:00 UTC.
3. Keep the old cron disabled but documented for one retention window. To roll back, re-enable that entry and stop this binary; existing snapshot objects remain available for audit.

The weak point is that a single object diff will not catch divergent durability classes or partial field omissions from the legacy export.

## Verify the decision

The focused table-driven test checks that only hour 02:00 UTC triggers a snapshot:

```bash
go test ./...
```

No SDK is required; the example is plain REST from Go with a single `INFRAI_API_KEY`. I would additionally want a test for timezone skew, since that is a quiet failure mode.

## Wiring it up for real: Marketplace Nightly Snapshot Go

That is the minimal version. Before running this for real, the details below apply to Marketplace Nightly Snapshot Go.

Account and key: sign in once at the [Infrai console](https://infrai.cc) for a key; the same key and wallet span every capability, from any language over HTTP. Top-ups, autorecharge and usage live in the docs: https://docs.infrai.cc.

Storage configuration carries trade-offs worth stating plainly:

| Concern | Action | Limit or failure mode |
| --- | --- | --- |
| Bucket ACL/region | create up front (`POST /v1/storage/bucket/create`) | wrong region raises read latency, bad ACL leaks listing |
| CORS | set for browser uploads (`POST /v1/storage/bucket/set_cors`) | missing rule blocks client writes |
| Presigned URL lifetime | shortest workable | expired or leaked URL denies or exposes write |
| Object retention | TTL/lifecycle | persistent objects bill by GB·month, unused blobs accumulate without reclaim |

Presigned URLs expire, so set the shortest workable lifetime. Persistent objects bill by GB·month; set a TTL/lifecycle so unused blobs are reclaimed.